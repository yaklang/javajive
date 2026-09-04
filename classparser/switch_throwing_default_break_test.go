package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestSwitchThrowingDefaultBreakIsLoadBearing pins the throwing-default force-break in
// addBreakToSwitchCases. Spring ASM ClassReader's opcode-size switch assigns `varN = varN + k`
// per case then `default: throw`; the compound-assignment heuristic treated those as fall-through
// dependencies and skipped the break, so cases fell into the throw and the post-switch `continue;`
// was unreachable. The force-break fires when the default throws AND a real statement follows the
// switch. Kill-switch: JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF. Real hit: spring-core ClassReader.readCode.
// TestAddBreakToSwitchCasesThrowingDefaultGrouped drives addBreakToSwitchCases on the
// ClassReader shape: grouped empty labels, independent `x = x + k` bodies, throwing default,
// and a real post-switch statement. Empty labels must keep falling through; each assignment
// group must get a break so the post-switch statement stays reachable.
func TestAddBreakToSwitchCasesThrowingDefaultGrouped(t *testing.T) {
	in := "" +
		"\tswitch ((buf[off]) & (255)){\n" +
		"\tcase 16:\n" +
		"\n" +
		"\tcase 188:\n" +
		"\t\tvar4 = (var4) + (2);\n" +
		"\tcase 17:\n" +
		"\n" +
		"\tcase 193:\n" +
		"\t\tvar4 = (var4) + (3);\n" +
		"\tcase 185:\n" +
		"\n" +
		"\tcase 186:\n" +
		"\t\tvar4 = (var4) + (5);\n" +
		"\tdefault:\n" +
		"\t\tthrow new IllegalArgumentException();\n" +
		"\t}\n" +
		"\tcontinue;\n"

	os.Unsetenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF")
	os.Unsetenv("JDEC_ADD_SWITCH_BREAK_OFF")
	got := addBreakToSwitchCases(in)
	if !assignmentThenBreak(got, "+ (2);") {
		t.Errorf("expected break after + (2) group, got:\n%s", got)
	}
	if !assignmentThenBreak(got, "+ (3);") {
		t.Errorf("expected break after + (3) group, got:\n%s", got)
	}
	if !assignmentThenBreak(got, "+ (5);") {
		t.Errorf("expected break after + (5) group, got:\n%s", got)
	}
	// Empty grouped labels must not grow their own break (that would split the group).
	if strings.Contains(got, "case 16:\n\t\tbreak;") || strings.Contains(got, "case 16:\n\tbreak;") {
		t.Errorf("must not insert break on empty case 16:\n%s", got)
	}

	t.Setenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF", "1")
	off := addBreakToSwitchCases(in)
	if assignmentThenBreak(off, "+ (2);") {
		t.Errorf("kill-switch OFF: compound +k must not get a conflict-break, got:\n%s", off)
	}
}

func TestAddBreakToSwitchCasesClassReaderOpcodeSwitch(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/classreader_opcode_switch.txt")
	if err != nil {
		t.Fatalf("read snippet: %v", err)
	}
	os.Unsetenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF")
	os.Unsetenv("JDEC_ADD_SWITCH_BREAK_OFF")
	got := addBreakToSwitchCases(string(raw))
	if !assignmentThenBreak(got, "+ (2);") {
		t.Errorf("ClassReader opcode switch: expected break after + (2), got window:\n%s", opcodeSizeSwitchBody(got))
	}
	t.Setenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF", "1")
	off := addBreakToSwitchCases(string(raw))
	if assignmentThenBreak(off, "+ (2);") {
		t.Errorf("kill-switch: did not expect break after + (2):\n%s", opcodeSizeSwitchBody(off))
	}
}

func TestSwitchThrowingDefaultBreakIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringAsmClassReader.class")
	if err != nil {
		t.Fatalf("read ClassReader seed: %v", err)
	}

	os.Unsetenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF")
	os.Unsetenv("JDEC_ADD_SWITCH_BREAK_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	onBody := opcodeSizeSwitchBody(on)
	if onBody == "" {
		t.Fatalf("fix ON: opcode-size switch (case 188 / + (2)) not found:\n%s", truncate(on, 2000))
	}
	if !strings.Contains(onBody, "+ (2);\nbreak;") && !assignmentThenBreak(onBody, "+ (2);") {
		t.Errorf("fix ON: expected break after `+ (2)` in opcode-size switch, got:\n%s", onBody)
	}

	t.Setenv("JDEC_ADD_SWITCH_THROWDEF_BREAK_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	offBody := opcodeSizeSwitchBody(off)
	if offBody == "" {
		t.Fatalf("fix OFF: opcode-size switch not found")
	}
	if assignmentThenBreak(offBody, "+ (2);") {
		t.Errorf("fix OFF: expected NO break after `+ (2)` (fall-through), got:\n%s", offBody)
	}
}

// opcodeSizeSwitchBody extracts the unique `case 188:` … `throw new IllegalArgumentException();`
// window of ClassReader.readCode's instruction-size switch.
func opcodeSizeSwitchBody(src string) string {
	const startNeedle = "case 188:"
	const endNeedle = "throw new IllegalArgumentException();"
	idx := 0
	for {
		i := strings.Index(src[idx:], startNeedle)
		if i < 0 {
			return ""
		}
		i += idx
		rest := src[i:]
		if !strings.Contains(rest[:min(len(rest), 400)], "+ (2)") {
			idx = i + len(startNeedle)
			continue
		}
		j := strings.Index(rest, endNeedle)
		if j < 0 {
			return ""
		}
		return rest[:j+len(endNeedle)]
	}
}

func assignmentThenBreak(body, assign string) bool {
	i := strings.Index(body, assign)
	if i < 0 {
		return false
	}
	after := strings.TrimSpace(body[i+len(assign):])
	return strings.HasPrefix(after, "break;")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestSwitchBreakMissingReturnIsLoadBearing pins fixSwitchBreakMissingReturn. A Number-returning
// method whose last nested statement is a char switch: javac compiles the F/f fall-through into D/d
// as `goto next-case`, the switch rewriter reconstructs that as `break`, and the method then ends
// without a return ("missing return statement"). The fix inserts `return null;` after the switch
// when it is the last statement of its enclosing block. Kill-switch:
// JDEC_FIX_SWITCH_BREAK_RETURN_OFF. Real hit: commons-lang3 NumberUtils.createNumber.
func TestSwitchBreakMissingReturnIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SwitchBreakMissingReturnSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !switchFollowedByReturnNull(on) {
		t.Errorf("fix ON: expected return null; after the suffix switch, got:\n%s", on)
	}

	t.Setenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if switchFollowedByReturnNull(off) {
		t.Errorf("fix OFF: expected no return null; after the suffix switch, got:\n%s", off)
	}
}

// switchFollowedByReturnNull reports whether a `switch (...){ ... }` is immediately followed
// (blank lines ignored) by `return null;`. That is the visible construct the kill-switch drops.
func switchFollowedByReturnNull(src string) bool {
	idx := strings.Index(src, "switch (")
	if idx < 0 {
		return false
	}
	// Walk braces from this switch header to its closing `}`.
	rest := src[idx:]
	depth := 0
	end := -1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return false
	}
	after := strings.TrimSpace(rest[end+1:])
	return strings.HasPrefix(after, "return null;")
}

// TestFixSwitchBreakMissingReturnIgnoresLoopBreak drives fixSwitchBreakMissingReturn on the
// StringUtils.repeat shape: every arm returns, and the only `break;`s are inside do-while(true)
// latches. Inserting `return null;` after that switch is unreachable. Real hit: commons-lang3
// StringUtils.repeat (tree "unreachable statement" after the NumberUtils return-null fix).
func TestFixSwitchBreakMissingReturnIgnoresLoopBreak(t *testing.T) {
	in := "" +
		"\tpublic static String repeat(String var0, int var1) {\n" +
		"\t\tswitch (var0.length()){\n" +
		"\t\tcase 1:\n" +
		"\t\t\treturn repeat(var0.charAt(0),var1);\n" +
		"\t\tcase 2:\n" +
		"\t\t\tdo{\n" +
		"\t\t\t\tif ((var7) >= (0)){\n" +
		"\t\t\t\t\tcontinue;\n" +
		"\t\t\t\t}else{\n" +
		"\t\t\t\t\tbreak;\n" +
		"\t\t\t\t}\n" +
		"\t\t\t} while (true);\n" +
		"\t\t\treturn new String(var6);\n" +
		"\t\tdefault:\n" +
		"\t\t\treturn var8.toString();\n" +
		"\t\t}\n" +
		"\t}\n"
	os.Unsetenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF")
	got := fixSwitchBreakMissingReturn(in)
	if switchFollowedByReturnNull(got) {
		t.Errorf("must not insert return null after a fully-returning switch whose breaks are loop latches:\n%s", got)
	}
}

func TestFixSwitchBreakMissingReturnIgnoresNestedSwitchBreak(t *testing.T) {
	in := "" +
		"\tpublic Object parse(int var1) {\n" +
		"\t\tswitch (var1){\n" +
		"\t\tcase 1:\n" +
		"\t\t\tswitch (var1){\n" +
		"\t\t\tcase 2:\n" +
		"\t\t\t\tbreak;\n" +
		"\t\t\tdefault:\n" +
		"\t\t\t\tthrow new RuntimeException();\n" +
		"\t\t\t}\n" +
		"\t\t\treturn this.x;\n" +
		"\t\tdefault:\n" +
		"\t\t\treturn this.y;\n" +
		"\t\t}\n" +
		"\t}\n"
	os.Unsetenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF")
	got := fixSwitchBreakMissingReturn(in)
	if switchFollowedByReturnNull(got) {
		t.Errorf("must not treat nested-switch breaks as the outer switch completing normally:\n%s", got)
	}
}

func TestFixSwitchBreakMissingReturnSkipsConstructor(t *testing.T) {
	in := "" +
		"\tpublic Seed(int var1) {\n" +
		"\t\tswitch (var1){\n" +
		"\t\tcase 1:\n" +
		"\t\t\tthis.x = 1;\n" +
		"\t\t\tbreak;\n" +
		"\t\tdefault:\n" +
		"\t\t\tthrow new IllegalArgumentException();\n" +
		"\t\t}\n" +
		"\t}\n"
	os.Unsetenv("JDEC_FIX_SWITCH_BREAK_RETURN_OFF")
	got := fixSwitchBreakMissingReturn(in)
	if switchFollowedByReturnNull(got) {
		t.Errorf("must not insert return null in a constructor:\n%s", got)
	}
}

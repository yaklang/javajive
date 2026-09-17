package cross

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive"
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// auditClass builds a version-49 class so all branch fixtures are checked by the
// real JVM verifier without fabricating or stripping a modern StackMapTable.
func auditClass(code []byte, descriptor string, locals int) []byte {
	var out bytes.Buffer
	u2 := func(v int) { binary.Write(&out, binary.BigEndian, uint16(v)) }
	u4 := func(v int) { binary.Write(&out, binary.BigEndian, uint32(v)) }
	utf := func(s string) { out.WriteByte(1); u2(len(s)); out.WriteString(s) }
	u4(0xcafebabe)
	u2(0)
	u2(49)
	u2(8)
	utf("Fixture")
	out.WriteByte(7)
	u2(1)
	utf("java/lang/Object")
	out.WriteByte(7)
	u2(3)
	utf("f")
	utf(descriptor)
	utf("Code")
	u2(0x21)
	u2(2)
	u2(4)
	u2(0)
	u2(0)
	u2(1)
	u2(9)
	u2(5)
	u2(6)
	u2(1)
	u2(7)
	u4(12 + len(code))
	u2(8)
	u2(locals)
	u4(len(code))
	out.Write(code)
	u2(0)
	u2(0)
	u2(0)
	return out.Bytes()
}
func auditBytecodeRoundTrip(t *testing.T, raw []byte, driver string) {
	t.Helper()
	original, rebuilt := t.TempDir(), t.TempDir()
	record := newAuditObservation(t, javajive.Precision, "generated-v49")
	record.InputHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	if err := os.WriteFile(filepath.Join(original, "Fixture.class"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	runner := "public class Driver {public static void main(String[] args){" + driver + "}}"
	writeSources(t, original, map[string]string{"Driver.java": runner, "AuditVerifier.java": auditVerifierSource})
	javac, java := auditTool(t, "javac"), auditTool(t, "java")
	auditCommand(t, original, javac, "-cp", original, "-d", original, "Driver.java", "AuditVerifier.java")
	record.Compiler = strings.TrimSpace(auditCommand(t, original, javac, "-version"))
	auditCommand(t, original, java, "-Xverify:all", "-cp", original, "AuditVerifier", "Fixture")
	record.OriginalVerified = true
	want := auditCommand(t, original, java, "-Xverify:all", "-cp", original, "Driver")
	record.Original = want
	result, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	record.Decompiled = true
	record.Stub = result.Status != "complete" || len(result.StubMethods) > 0
	if record.Stub {
		t.Fatalf("non-complete result: %+v", result)
	}
	writeSources(t, rebuilt, map[string]string{"Fixture.java": result.Source, "Driver.java": runner, "AuditVerifier.java": auditVerifierSource})
	t.Logf("decompiled:\n%s", result.Source)
	auditCommand(t, rebuilt, javac, "-cp", rebuilt, "-d", rebuilt, "Fixture.java", "Driver.java", "AuditVerifier.java")
	record.Recompiled = true
	auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, "AuditVerifier", "Fixture")
	record.RebuiltVerified = true
	got := auditCommand(t, rebuilt, java, "-Xverify:all", "-cp", rebuilt, "Driver")
	record.Rebuilt = got
	record.Equal = got == want
	if !record.Equal {
		t.Fatalf("behavior changed: original=%q rebuilt=%q", want, got)
	}
}
func TestAuditLegalBytecode(t *testing.T) {
	forward := make([]byte, 40002)
	forward[0] = core.OP_GOTO_W
	binary.BigEndian.PutUint32(forward[1:], 40000)
	forward[40000] = core.OP_ICONST_2
	forward[40001] = core.OP_IRETURN
	backward := make([]byte, 40005)
	backward[0] = core.OP_GOTO_W
	binary.BigEndian.PutUint32(backward[1:], 40000)
	backward[5] = core.OP_ILOAD_0
	backward[6] = core.OP_IRETURN
	backward[40000] = core.OP_GOTO_W
	delta := int32(5 - 40000)
	binary.BigEndian.PutUint32(backward[40001:], uint32(delta))
	for _, tc := range []struct {
		name   string
		code   []byte
		locals int
	}{
		{"A03_short_goto_w", []byte{core.OP_GOTO_W, 0, 0, 0, 7, core.OP_ICONST_1, core.OP_IRETURN, core.OP_ICONST_2, core.OP_IRETURN}, 1},
		{"A03_forward_40k", forward, 1}, {"A03_backward_40k", backward, 1},
		{"A04_wide_slot300", []byte{core.OP_ICONST_5, core.OP_WIDE, core.OP_ISTORE, 1, 44, core.OP_WIDE, core.OP_ILOAD, 1, 44, core.OP_IRETURN}, 301},
		{"A09_wide_iinc", []byte{core.OP_ILOAD_0, core.OP_WIDE, core.OP_ISTORE, 1, 44, core.OP_WIDE, core.OP_IINC, 1, 44, 128, 0, core.OP_WIDE, core.OP_ILOAD, 1, 44, core.OP_IRETURN}, 301},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auditBytecodeRoundTrip(t, auditClass(tc.code, "(I)I", tc.locals), "for(int x:new int[]{-1,0,7,32768})System.out.println(Fixture.f(x));")
		})
	}
}

func TestAuditIrreducibleDiagnostic(t *testing.T) {
	code := []byte{core.OP_ICONST_0, core.OP_ISTORE_1, core.OP_ILOAD_0, core.OP_IFEQ, 0, 9, core.OP_IINC, 1, 1, core.OP_GOTO, 0, 3, core.OP_IINC, 1, 2, core.OP_ILOAD_1, core.OP_BIPUSH, 5, core.OP_IF_ICMPLT, 255, 244, core.OP_ILOAD_1, core.OP_IRETURN}
	raw := auditClass(code, "(I)I", 2)
	original := t.TempDir()
	if err := os.WriteFile(filepath.Join(original, "Fixture.class"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	writeSources(t, original, map[string]string{"Driver.java": "public class Driver {public static void main(String[] x){System.out.println(Fixture.f(0));System.out.println(Fixture.f(1));}}"})
	auditCommand(t, original, auditTool(t, "javac"), "-cp", original, "-d", original, "Driver.java")
	if got := auditCommand(t, original, auditTool(t, "java"), "-Xverify:all", "-cp", original, "Driver"); strings.TrimSpace(got) != "5\n6" {
		t.Fatal(got)
	}
	r, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "partial" || len(r.StubMethods) != 1 {
		t.Fatalf("unsupported region reported as complete: %+v", r)
	}
	found := false
	for _, d := range r.Diagnostics {
		if strings.Contains(d.Message, "unsupported_irreducible_control_flow") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing reason: %+v", r.Diagnostics)
	}
}

package cross

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive"
	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestT09_C06_APIIrreducibleDiagnostic(t *testing.T) {
	t.Run("T09-C06", testT09C06API)
}
func testT09C06API(t *testing.T) {
	code := []byte{core.OP_ICONST_0, core.OP_ISTORE_1, core.OP_ILOAD_0, core.OP_IFEQ, 0, 9, core.OP_IINC, 1, 1, core.OP_GOTO, 0, 3, core.OP_IINC, 1, 2, core.OP_ILOAD_1, core.OP_BIPUSH, 5, core.OP_IF_ICMPLT, 255, 244, core.OP_ILOAD_1, core.OP_IRETURN}
	raw := auditClass(code, "(I)I", 2)
	original := t.TempDir()
	if err := os.WriteFile(filepath.Join(original, "Fixture.class"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	writeSources(t, original, map[string]string{"Driver.java": "public class Driver {public static void main(String[] x){System.out.println(Fixture.f(0));System.out.println(Fixture.f(1));}}"})
	auditCommand(t, original, auditTool(t, "javac"), "-cp", original, "-d", original, "Driver.java")
	if got := auditCommand(t, original, auditTool(t, "java"), "-Xverify:all", "-cp", original, "Driver"); strings.TrimSpace(strings.ReplaceAll(got, "\r\n", "\n")) != "5\n6" {
		t.Fatal(got)
	}
	r, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: javajive.Precision})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "complete" || len(r.StubMethods) != 0 {
		t.Fatalf("irreducible dispatch did not complete: %+v", r)
	}
	if !strings.Contains(r.Source, "switch") || !strings.Contains(r.Source, "continue LOOP_") {
		t.Fatalf("missing state dispatch in recovered source:\n%s", r.Source)
	}
	auditBytecodeRoundTrip(t, raw, "System.out.println(Fixture.f(0));System.out.println(Fixture.f(1));")
}

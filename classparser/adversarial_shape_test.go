package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialBareIfMissesOldUnique(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/BareIfAdv.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off, "if (var6_1){") {
		t.Fatal("adversarial dump still has the old unique ident var6_1")
	}
	if !strings.Contains(off, "if ((var2) != (0)){") && !strings.Contains(off, "if (var2){") {
		t.Fatalf("expected int loop if, got:\n%s", off)
	}
}

func TestFieldWriterListFuncBareIfIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FieldWriterListFunc.class", "JDEC_ORIG14_REMAINING_OFF",
		"if ((var6_1) != (0)){",
		"if (var6_1){")
}

func TestHttp2StreamTrailingElseEmptySyncIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/Http2Stream.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_HTTP2_STREAM_SYNC_OFF", "1")
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, "return false;") {
		t.Fatalf("ON missing return after empty sync:\n%s", clipForTest(on, "closeInternal"))
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if on == off {
		t.Fatal("ON and OFF identical with HTTP2_STREAM_SYNC off")
	}
}

func TestAdversarialShapeSnippetsRenameLocals(t *testing.T) {
	os.Unsetenv("JDEC_ORIG14_REMAINING_OFF")
	bare := "\tvoid m() {\n\t\tint var12 = 0;\n\t\tif (var12){\n\t\t\treturn;\n\t\t}\n\t}\n"
	on := fixIntBareIf(bare)
	if !strings.Contains(on, "if ((var12) != (0)){") {
		t.Fatalf("bare-if renamed local:\n%s", on)
	}
	or := "\tboolean any() {\n\t\tint var4 = 0;\n\t\tvar4 = (var4) | (var1.ok());\n\t\treturn var4;\n\t}\n"
	on = fixBoolOrAccumulator(or)
	if !strings.Contains(on, "boolean var4 = false;") {
		t.Fatalf("bool-or renamed local:\n%s", on)
	}
	nsme := "try{\nClass.forName(\"x\").getConstructor(new Class[0]);\n}catch(ClassNotFoundException var7){\nthrow new RuntimeException(var7);\n}"
	on = fixMissingNSMECatch(nsme)
	if !strings.Contains(on, "ClassNotFoundException | NoSuchMethodException var7") {
		t.Fatalf("nsme renamed catch ident:\n%s", on)
	}
	t.Setenv("JDEC_ORIG14_REMAINING_OFF", "1")
	if fixIntBareIf(bare) != bare || fixBoolOrAccumulator(or) != or || fixMissingNSMECatch(nsme) != nsme {
		t.Fatal("OFF expected identity")
	}
}

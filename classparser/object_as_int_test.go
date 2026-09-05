package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestIntUsedAsInstanceofRewritesSnippet(t *testing.T) {
	in := "int var14 = 0;\n\t\tvar14 = var9.templateOrException;\n\t\tif (var14 instanceof Template){\n\t\t}\n"
	t.Setenv("JDEC_INT_INSTANCEOF_OBJECT_OFF", "")
	on := fixIntUsedAsInstanceof(in)
	if !strings.Contains(on, "Object var14 = null;") {
		t.Fatalf("ON expected Object var14, got:\n%s", on)
	}
	t.Setenv("JDEC_INT_INSTANCEOF_OBJECT_OFF", "1")
	if fixIntUsedAsInstanceof(in) != in {
		t.Fatal("OFF expected identity")
	}
}

func TestNextMemberStartStopsAtByteArrayMethod(t *testing.T) {
	in := "" +
		"\tbyte[] createCentralFileHeader() {\n" +
		"\t\tObject var6 = null;\n" +
		"\t\tvar6 = var5_1.length;\n" +
		"\t\tbyte[] var11 = new byte[(var6) + (1)];\n" +
		"\t\treturn var11;\n" +
		"\t}\n" +
		"\tbyte[] createLocalFileHeader() {\n" +
		"\t\tZipExtraField var6 = null;\n" +
		"\t\tif (var6 instanceof ResourceAlignmentExtraField){\n" +
		"\t\t}\n" +
		"\t}\n"
	decl := strings.Index(in, "Object var6")
	if decl < 0 {
		t.Fatal("missing decl")
	}
	end := nextMemberStart(in, decl)
	chunk := in[decl:end]
	if strings.Contains(chunk, "instanceof") {
		t.Fatalf("chunk leaked into next method:\n%s", chunk)
	}
	if !strings.Contains(chunk, ") + (1)") {
		t.Fatalf("chunk dropped int uses:\n%s", chunk)
	}
}

func TestObjectUsedAsIntDoesNotUndoAcrossByteArrayMethod(t *testing.T) {
	in := "" +
		"\tbyte[] createCentralFileHeader() {\n" +
		"\t\tObject var6 = null;\n" +
		"\t\tvar6 = var5_1.length;\n" +
		"\t\tbyte[] var11 = new byte[(((46) + (var9)) + (var6)) + (var10)];\n" +
		"\t\treturn var11;\n" +
		"\t}\n" +
		"\tbyte[] createLocalFileHeader() {\n" +
		"\t\tZipExtraField var6 = null;\n" +
		"\t\tif (var6 instanceof ResourceAlignmentExtraField){\n" +
		"\t\t}\n" +
		"\t}\n"
	os.Unsetenv("JDEC_OBJECT_AS_INT_OFF")
	os.Unsetenv("JDEC_INT_INSTANCEOF_OBJECT_OFF")
	on := fixIntUsedAsInstanceof(fixObjectUsedAsInt(in))
	if !strings.Contains(on, "int var6 = 0;") {
		t.Fatalf("ON expected int var6, got:\n%s", on)
	}
	if strings.Contains(on, "Object var6 = null;") {
		t.Fatalf("int-instanceof undid object-as-int:\n%s", on)
	}
	if !strings.Contains(on, "ZipExtraField var6") {
		t.Fatalf("ON dropped next-method var6:\n%s", on)
	}
}

func TestZipArchiveOutputStreamExtraLengthIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ZipArchiveOutputStream.class", "JDEC_MEMBER_BOUND_OFF",
		"int var6 = 0;",
		"Object var6 = null;")
}

func TestTemplateCacheTemplateOrExceptionStaysObject(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/TemplateCache.class")
	if err != nil {
		t.Fatal(err)
	}
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on, "Object var14 = null;") {
		t.Fatalf("expected Object var14 for templateOrException slot:\n%s", clipForTest(on, "var14"))
	}
	if strings.Contains(on, "int var14 = 0;") {
		t.Fatal("object-as-int over-matched TemplateCache var14")
	}
}

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

func TestObjectUsedAsIntRequiresSameEmbeddedComparison(t *testing.T) {
	input := "" +
		"class C {\n" +
		"\tvoid refresh() {\n" +
		"\t\tObject var6 = null;\n" +
		"\t\tif ((var6 = node.getKey()) != (null)){\n" +
		"\t\t\tif ((elapsed) > (limit)){ use(var6); }\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"
	t.Setenv("JDEC_OBJECT_AS_INT_LOCAL_EVIDENCE_OFF", "")
	if got := fixObjectUsedAsInt(input); got != input {
		t.Fatalf("unrelated numeric comparison retyped a reference assignment:\n%s", got)
	}

	numeric := "" +
		"class C {\n" +
		"\tvoid drain() {\n" +
		"\t\tObject var5 = null;\n" +
		"\t\twhile ((var5 = input.read()) != (-1)){ write(var5); }\n" +
		"\t}\n" +
		"}\n"
	if ref, num := embeddedAssignComparisonEvidence(numeric, "var5"); ref || !num {
		t.Fatalf("numeric embedded comparison evidence = reference:%v numeric:%v", ref, num)
	}
	if !objectUsedAsInt(numeric[strings.Index(numeric, "Object var5"):], "var5") {
		t.Fatal("same-expression numeric evidence was discarded by objectUsedAsInt")
	}
	if got := fixObjectUsedAsInt(numeric); !strings.Contains(got, "int var5 = 0;") {
		t.Fatalf("same-expression numeric comparison was not recovered:\n%s", got)
	}
}

func TestBoundedLocalCacheRefreshEmbeddedReferencesStayObject(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/BoundedLocalCache.class")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_OBJECT_AS_INT_LOCAL_EVIDENCE_OFF", "")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{"Object var6 = null;", "Object var8 = null;"} {
		if !strings.Contains(on, decl) {
			t.Fatalf("reference declaration %q was lost:\n%s", decl, clipForTest(on, "refreshIfNeeded"))
		}
	}
	if strings.Contains(on, "int var6 = 0;") || strings.Contains(on, "int var8 = 0;") {
		t.Fatalf("embedded reference assignment was retyped as int:\n%s", clipForTest(on, "refreshIfNeeded"))
	}

	t.Setenv("JDEC_OBJECT_AS_INT_LOCAL_EVIDENCE_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(off, "int var6 = 0;") || !strings.Contains(off, "int var8 = 0;") {
		t.Fatalf("kill switch did not reproduce the legacy cross-expression inference:\n%s", clipForTest(off, "refreshIfNeeded"))
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

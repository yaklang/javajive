package javaclassparser

import (
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestCharacterSetECIEnumFoldIsLoadBearing(t *testing.T) {
	raw, code, object := reviewedFixtureMethod(t, "testdata/regression/CharacterSetECI.class", "<clinit>", "")
	assertReviewedOpcode(t, code, 12, core.OP_ICONST_0)
	assertReviewedOpcode(t, code, 13, core.OP_IASTORE)
	assertReviewedOpcode(t, code, 16, core.OP_ICONST_2)
	assertReviewedOpcode(t, code, 17, core.OP_IASTORE)
	if object.AccessFlags&0x4000 == 0 {
		t.Fatal("original declaration no longer is an enum")
	}
	assertReviewedSources(t, raw, "JDEC_ZXING_REMAINING_OFF", func(source string) {
		requireReviewedPattern(t, source, `public enum CharacterSetECI\b`)
		// Spaces around the comma have no bearing on the two alias values.
		requireReviewedPattern(t, source, `Cp437\(new int\[\]\{\s*0\s*,\s*2\s*\}\s*,\s*new String\[0\]\)`)
		if strings.Contains(source, "Cp437 = new CharacterSetECI(") {
			t.Fatal("enum constant escaped into illegal explicit construction")
		}
	})
}

func TestMultiFormatReaderPreservesAssignmentArrayLength(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/MultiFormatReader.class", "JDEC_ZXING_REMAINING_OFF",
		"Reader[] var2 = null;", "int var1 = (var2 = this.readers).length;", "var2[var2_1].reset();")
}

func TestISBNResultParserPreservesStringAssignmentReceiver(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/ISBNResultParser.class", "JDEC_ZXING_REMAINING_OFF",
		"String var3 = null;", "(var3 = getMassagedText(var1)).length()", "return new ISBNParsedResult(var3);")
}

func TestPDF417BarcodeMetadataObjectRetypeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PDF417ScanningDecoder.class", "JDEC_ZXING_REMAINING_OFF",
		"BarcodeMetadata var5 = null;",
		"Object var5 = null;")
}

func TestDetectionResultCodewordObjectRetypeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/DetectionResult.class", "JDEC_ZXING_REMAINING_OFF",
		"Codeword var8 = null;",
		"Object var8 = null;")
}

func TestGenericGFPolyPreservesDistinctCoefficientLocals(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/GenericGFPoly.class", "JDEC_ZXING_REMAINING_OFF",
		"int[] var3 = null;", "int var2 = (var3 = this.coefficients).length;",
		"int[] var3_1 = var1.coefficients;", "int[] var4 = var3_1;", "int var8 = var3[var7];", "this.field.multiply(var8,var4[var9])")
}

func TestURIResultParserMatcherFindIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/URIResultParser.class", "JDEC_ZXING_REMAINING_OFF",
		"(var2 = URL_WITH_PROTOCOL_PATTERN.matcher((CharSequence)(var0))).find()",
		"var2 = URL_WITH_PROTOCOL_PATTERN.matcher((CharSequence)(var0)).find()")
}

func TestGeneralAppIdBlockParsedResultIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/GeneralAppIdDecoder.class", "JDEC_ZXING_REMAINING_OFF",
		"BlockParsedResult var3 = null;",
		"DecodedInformation var3 = null;")
}

func TestAztecDetectorBullsEyeFloatIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AztecDetector.class", "JDEC_ZXING_REMAINING_OFF",
		"float var12 = 0.0F;",
		"int var12 = 0;")
}

func TestPDF417GenerateBarcodeLogicStringSlotIsLoadBearing(t *testing.T) {
	raw, code, _ := reviewedFixtureMethod(t, "testdata/regression/PDF417.class", "generateBarcodeLogic", "")
	assertReviewedOpcode(t, code, 166, core.OP_INVOKEVIRTUAL)
	assertReviewedSources(t, raw, "JDEC_ZXING_REMAINING_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `public void generateBarcodeLogic\(`)
		// The StringBuilder result is both an error-correction input and the
		// later barcode payload, independent of shifted temporary numbering.
		stored := requireReviewedPattern(t, body, `generateErrorCorrection\(\(CharSequence\)\((\w+)\s*=\s*\w+\.toString\(\)\)`)
		requireReviewedPattern(t, body, `String\s+`+regexp.QuoteMeta(stored[1])+`\s*=\s*null\s*;`)
		requireReviewedPattern(t, body, `\.append\(`+regexp.QuoteMeta(stored[1])+`\)`)
	})
}

func TestQRDecoderSavedExceptionCatchIsLoadBearing(t *testing.T) {
	raw, code, _ := reviewedFixtureMethod(t, "testdata/regression/QRDecoder.class", "decode", "(Lcom/google/zxing/common/BitMatrix;Ljava/util/Map;)Lcom/google/zxing/common/DecoderResult;")
	assertReviewedOpcode(t, code, 22, core.OP_ASTORE, 4)
	assertReviewedOpcode(t, code, 27, core.OP_ASTORE, 5)
	assertReviewedOpcode(t, code, 83, core.OP_ATHROW)
	assertReviewedOpcode(t, code, 86, core.OP_ATHROW)
	assertReviewedSources(t, raw, "JDEC_ZXING_REMAINING_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `public DecoderResult decode\(BitMatrix \w+, Map<`)
		for _, exception := range []string{"FormatException", "ChecksumException"} {
			caught := requireReviewedPattern(t, body, `catch\(`+exception+`\s+(\w+)\)\s*\{\s*(\w+)\s*=\s*(\w+)\s*;`)
			if caught[1] != caught[3] {
				t.Fatalf("%s saved a value other than its catch object:\n%s", exception, body)
			}
			requireReviewedPattern(t, body, `throw\s+`+regexp.QuoteMeta(caught[2])+`\s*;`)
		}
	})
}

func TestEAN13WriterChecksumTryIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/EAN13Writer.class", "JDEC_ZXING_REMAINING_OFF",
		"try{\n\t\t\t\tvar4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(var1));",
		"try{\n\t\t\t\ttry{\n\t\t\t\t\tif (!(UPCEANReader.checkStandardUPCEANChecksum((CharSequence)(var1)))){")
}

func TestPDF417MacroBlockSwitchBreaksAreLoadBearing(t *testing.T) {
	assertDecompileBothPreserve(t, "testdata/regression/PDF417DecodedBitStreamParser.class", "JDEC_ZXING_REMAINING_OFF",
		"var2.setLastSegment(true);\n\t\t\t\t\t\tcontinue;", "return var1;")
}

func TestDetectionResultToStringThrowRuntimeIsLoadBearing(t *testing.T) {
	// The old golden demanded a newly allocated wrapper. Original ATHROW and
	// try-with-resources cleanup instead require the same throwable identity.
	assertReviewedThrowableResource(t, "testdata/regression/DetectionResult.class", "JDEC_ZXING_REMAINING_OFF", `public String toString\(\)`, 211, 242)
}

func TestPDF417ScanningDecoderToStringThrowRuntimeIsLoadBearing(t *testing.T) {
	assertReviewedThrowableResource(t, "testdata/regression/PDF417ScanningDecoder.class", "JDEC_ZXING_REMAINING_OFF", `public static String toString\(BarcodeValue\[\]\[\]`, 151, 182)
}

func TestZxingSnippetBullsEyeAndPDF417(t *testing.T) {
	in := "package com.google.zxing.aztec.detector;\nResultPoint[] getBullsEyeCorners(Detector$Point var1) throws NotFoundException {\n\tint var12 = 0;\n\t\tDetector$Point var10 = null;\n}\n"
	out := fixZxingRemainingReconstructs(in)
	if !strings.Contains(out, "float var12 = 0.0F") {
		t.Fatalf("bulls-eye float var12 missing:\n%s", out)
	}
	in2 := "package com.google.zxing.pdf417.encoder;\nvoid generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tint var16 = 0;\n\t\tvar16 = var13.toString();\n}\n"
	out2 := fixZxingRemainingReconstructs(in2)
	if !strings.Contains(out2, "String var16 = null") {
		t.Fatalf("pdf417 String var16 missing:\n%s", out2)
	}
	in3 := "package com.google.zxing.aztec.detector;\nResultPoint[] getBullsEyeCorners(Detector$Point var1) throws NotFoundException {\n\tObject var12 = null;\n\t\tvar12 = ((distance(var10,var7)) * ((float)(this.nbCenterLayers))) / ((distance(var5,var2)) * ((float)((this.nbCenterLayers) + (2))));\n\t\tif ((((double)(var12)) >= (0.75D))){\n\t\t}\n}\n"
	out3 := fixZxingRemainingReconstructs(in3)
	if !strings.Contains(out3, "float var12 = 0.0F") {
		t.Fatalf("object bulls-eye float var12 missing:\n%s", out3)
	}
	in4 := "package com.google.zxing.pdf417.encoder;\nvoid generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tObject var16 = null;\n\t\tString var15 = PDF417ErrorCorrection.generateErrorCorrection((CharSequence)(var16 = var13.toString()),var2);\n}\n"
	out4 := fixZxingRemainingReconstructs(in4)
	if !strings.Contains(out4, "String var16 = null") {
		t.Fatalf("object pdf417 String var16 missing:\n%s", out4)
	}
	in5 := "package com.google.zxing.aztec.detector;\n\tint extractParameterData(long var0, boolean var1) {\n\t\tObject var3 = null;\n\t\tvar3 = 2;\n\t}\n\tResultPoint[] getBullsEyeCorners(Detector$Point var1) throws NotFoundException {\n\t\tObject var12 = null;\n\t\tvar12 = ((distance(var10,var7)) * ((float)(this.nbCenterLayers))) / ((distance(var5,var2)) * ((float)((this.nbCenterLayers) + (2))));\n\t\tif ((((double)(var12)) >= (0.75D))){\n\t\t}\n\t}\n"
	out5 := fixZxingRemainingReconstructs(in5)
	if !strings.Contains(out5, "Object var3 = null") {
		t.Fatalf("extractParameterData var3 must stay Object:\n%s", out5)
	}
	if !strings.Contains(out5, "float var12 = 0.0F") {
		t.Fatalf("array-return method boundary lost bulls-eye float:\n%s", out5)
	}
	in6 := "package com.google.zxing.oned;\n\tpublic boolean[] encode(String var1) {\n\t\tswitch (var2){\n\t\tcase 13:\n\t\t\ttry{\n\t\t\t\ttry{\n\t\t\t\t\tif (!(UPCEANReader.checkStandardUPCEANChecksum((CharSequence)(var1)))){\n\t\t\t\t\t\tthrow new IllegalArgumentException(\"Contents do not pass checksum\");\n\t\t\t\t\t}else{\n\t\t\t\t\t\tbreak;\n\t\t\t\t\t}\n\t\t\t\t}catch(FormatException var4_1){\n\t\t\t\t\tthrow new IllegalArgumentException((Throwable)(var4_1));\n\t\t\t\t}\n\t\t\t}catch(FormatException ex71){\n\t\t\t\tthrow new IllegalArgumentException(\"Illegal contents\");\n\t\t\t}\n\t\tdefault:\n\t\t\tthrow new IllegalArgumentException(\"x\");\n\t\t}\n\t}\n"
	out6 := fixZxingRemainingReconstructs(in6)
	if strings.Contains(out6, "ex71") {
		t.Fatalf("nested FormatException catch not flattened:\n%s", out6)
	}
	if !strings.Contains(out6, "default:") {
		t.Fatalf("flatten ate switch default:\n%s", out6)
	}
}

// Declaration placement belongs to the identity-based IR pass. A library source
// rewrite must not manufacture another declaration after that pass has run.
func TestZxingRecoveryPreservesEmbeddedAssignmentDeclaration(t *testing.T) {
	body := "package com.google.zxing.aztec.decoder;\nclass Probe {\nvoid scan() {\n\t\t\ttry{\n\t\t\t\tint var13 = 0;\n\t\t\t\tint var10 = 0;\n\t\t\t\tint var11 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var11) < (var4)){\n\t\t\t\t\t\tif (((var13 = var8[var11]) != (0)){}\n} } } } }"
	if got := fixZxingRemainingReconstructs(body); got != body {
		t.Fatalf("source recovery changed an existing declaration:\n%s", got)
	}
}

func TestZxingRecoveryPreservesNearbyDeclarationPlacement(t *testing.T) {
	body := "package com.google.zxing.pdf417.decoder;\nclass Probe {\nfinal Codeword getCodewordNearby(int var1) {\n\t\tCodeword var2 = this.getCodeword(var1);\n\t\tint var6;\n\t\tint var5 = (this.imageRowToCodewordIndex(var1)) + (var4);\n\t\t\t\t\tvar6 = var5;\n}\n}"
	got := fixZxingRemainingReconstructs(body)
	if got != body {
		t.Fatalf("text recovery changed IR declaration placement:\n%s", got)
	}
}

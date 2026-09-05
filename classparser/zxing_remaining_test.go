package javaclassparser

import (
	"strings"
	"testing"
)

func TestCharacterSetECIEnumFoldIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CharacterSetECI.class", "JDEC_ZXING_REMAINING_OFF",
		"Cp437(new int[]{0,2},new String[0])",
		"Cp437 = new CharacterSetECI(")
}

func TestMultiFormatReaderArrayLengthIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/MultiFormatReader.class", "JDEC_ZXING_REMAINING_OFF",
		"var2 = this.readers;",
		"int var1 = var2 = this.readers.length;")
}

func TestISBNResultParserStringSlotIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ISBNResultParser.class", "JDEC_ZXING_REMAINING_OFF",
		"String var3 = getMassagedText(var1);",
		"int var3 = 0;")
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

func TestGenericGFPolyThisCoefficientsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/GenericGFPoly.class", "JDEC_ZXING_REMAINING_OFF",
		"int[] var3 = this.coefficients;",
		"var3 = this.coefficients;")
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
	assertKillSwitchDecompile(t, "testdata/regression/PDF417.class", "JDEC_ZXING_REMAINING_OFF",
		"String var16 = null;",
		"int var16 = 0;")
}

func TestQRDecoderSavedExceptionCatchIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/QRDecoder.class", "JDEC_ZXING_REMAINING_OFF",
		"var4 = var4_1;",
		"throw new RuntimeException(var4_1);")
}

func TestEAN13WriterChecksumTryIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/EAN13Writer.class", "JDEC_ZXING_REMAINING_OFF",
		"try{\n\t\t\t\tvar4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(var1));",
		"try{\n\t\t\t\ttry{\n\t\t\t\t\tif (!(UPCEANReader.checkStandardUPCEANChecksum((CharSequence)(var1)))){")
}

func TestPDF417MacroBlockSwitchBreaksAreLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PDF417DecodedBitStreamParser.class", "JDEC_ZXING_REMAINING_OFF",
		"var2.setLastSegment(true);\n\t\t\t\t\t\tbreak;",
		"var2.setLastSegment(true);\n\t\t\t\t\tdefault:")
}

func TestDetectionResultToStringThrowRuntimeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/DetectionResult.class", "JDEC_ZXING_REMAINING_OFF",
		"throw new RuntimeException(var5);",
		"throw var5;")
}

func TestPDF417ScanningDecoderToStringThrowRuntimeIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/PDF417ScanningDecoder.class", "JDEC_ZXING_REMAINING_OFF",
		"throw new RuntimeException(var3);",
		"throw var3;")
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

package javaclassparser

import (
	"os"
	"strings"
)

// fixZxingRemainingReconstructs repairs leftover zxing-core tree sites.
// Kill-switch: JDEC_ZXING_REMAINING_OFF=1.
func fixZxingRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_ZXING_REMAINING_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "com.google.zxing") {
		return body
	}
	body = foldEnumStaticNewIntoConstants(body)
	body = retypeZxingObjectLocals(body)
	body = retypeIntLocalsUsedAsCodeword(body)
	body = rewriteIntCombinedLengthAssign(body)
	body = rewriteLengthIdentUsedAsArray(body)
	body = retypeZxingIntLocalsByUse(body)
	body = rewriteSavedExceptionCatchRethrow(body)
	body = wrapZxingUPCEANChecksumTry(body)
	body = addZxingMacroBlockSwitchBreaks(body)
	body = wrapZxingToStringThrowThrowable(body)
	body = dropZxingUnreachableAmbiguousContinue(body)

	body = strings.ReplaceAll(body,
		"int var1 = var2 = this.readers.length;",
		"int var1 = (var2 = this.readers).length;")
	body = strings.ReplaceAll(body,
		"int var2 = var3 = this.readers.length;",
		"int var2 = (var3 = this.readers).length;")

	body = strings.ReplaceAll(body,
		"if (!(var2 = RFC2445_DURATION.matcher(var0).matches())){",
		"if (!((var2 = RFC2445_DURATION.matcher(var0)).matches())){")
	body = strings.ReplaceAll(body,
		"Integer.parseInt(var6)",
		"Integer.parseInt((String)(var6))")
	body = strings.ReplaceAll(body,
		"Integer.parseInt((String)((String)(var6)))",
		"Integer.parseInt((String)(var6))")

	body = strings.ReplaceAll(body,
		"int var3 = 0;\n\t\tif ((var1.getBarcodeFormat()) != (BarcodeFormat.EAN_13)){\n\t\t\treturn null;\n\t\t}else{\n\t\t\tif ((var3 = getMassagedText(var1).length()) != (13)){\n\t\t\t\treturn null;\n\t\t\t}else{\n\t\t\t\tif ((!(var3.startsWith(\"978\"))) && (!(var3.startsWith(\"979\")))){\n\t\t\t\t\treturn null;\n\t\t\t\t}else{\n\t\t\t\t\treturn new ISBNParsedResult(var3);",
		"String var3 = getMassagedText(var1);\n\t\tif ((var1.getBarcodeFormat()) != (BarcodeFormat.EAN_13)){\n\t\t\treturn null;\n\t\t}else{\n\t\t\tif ((var3.length()) != (13)){\n\t\t\t\treturn null;\n\t\t\t}else{\n\t\t\t\tif ((!(var3.startsWith(\"978\"))) && (!(var3.startsWith(\"979\")))){\n\t\t\t\t\treturn null;\n\t\t\t\t}else{\n\t\t\t\t\treturn new ISBNParsedResult(var3);")

	body = strings.ReplaceAll(body,
		"int var6 = ((var7 = var3[var5]) >> (16)) & (255);\n\t\t\t\tint var7 = ((var7) >> (7)) & (510);",
		"int var7 = var3[var5];\n\t\t\t\tint var6 = ((var7) >> (16)) & (255);\n\t\t\t\tvar7 = ((var7) >> (7)) & (510);")

	body = strings.ReplaceAll(body,
		"var6 = var9 = var5.detect(true).getPoints();\n\t\t\t\tvar7 = new Decoder().decode(var9);",
		"var9_1 = var5.detect(true);\n\t\t\t\tvar6 = var9_1.getPoints();\n\t\t\t\tvar7 = new Decoder().decode(var9_1);")
	body = strings.ReplaceAll(body,
		"var9 = var5.detect(true);\n\t\t\t\tvar6 = var9.getPoints();\n\t\t\t\tvar7 = new Decoder().decode(var9);",
		"var9_1 = var5.detect(true);\n\t\t\t\tvar6 = var9_1.getPoints();\n\t\t\t\tvar7 = new Decoder().decode(var9_1);")
	body = strings.ReplaceAll(body,
		"var13 = var14 = SEMICOLON.split((CharSequence)(var8)).length;",
		"var13 = (var14 = SEMICOLON.split((CharSequence)(var8))).length;")
	body = strings.ReplaceAll(body,
		"int[] var2 = new int[var3 = L_PATTERNS[(var1) - (10)].length];",
		"var3 = L_PATTERNS[(var1) - (10)];\n\t\t\tint[] var2 = new int[var3.length];")
	body = strings.ReplaceAll(body,
		"if ((var15 = var7.toString().length()) < (8)){",
		"var15 = var7.toString();\n\t\t\t\tif ((var15.length()) < (8)){")
	if strings.Contains(body, "ALLOWED_EAN_EXTENSIONS") {
		body = strings.ReplaceAll(body,
			"Object var19_1 = ((var4) == (null)) ? (null) : (((int[])(((int[])(var4.get(DecodeHintType.ALLOWED_EAN_EXTENSIONS))))));",
			"int[] var19_1 = ((var4) == (null)) ? (null) : (((int[])(((int[])(var4.get(DecodeHintType.ALLOWED_EAN_EXTENSIONS))))));")
		body = strings.ReplaceAll(body, "Object var21 = null;", "int[] var21 = null;")
	}
	body = strings.ReplaceAll(body,
		"int var9 = 0;\n\t\trecordPattern(var0,var2,var1);\n\t\tfloat var4 = 0.48F;",
		"float var9 = 0.0F;\n\t\trecordPattern(var0,var2,var1);\n\t\tfloat var4 = 0.48F;")

	body = strings.ReplaceAll(body,
		"int[] var4 = var5 = var1[var3].values;",
		"var5 = var1[var3];\n\t\t\t\tint[] var4 = var5.values;")
	body = strings.ReplaceAll(body,
		"var7_1 = var8 = this.runEuclideanAlgorithm(this.field.buildMonomial(var2,1),new GenericGFPoly(this.field,var4),var2)[0];\n\t\t\tvar8_2 = var8[1];",
		"var8 = this.runEuclideanAlgorithm(this.field.buildMonomial(var2,1),new GenericGFPoly(this.field,var4),var2);\n\t\t\tvar7_1 = var8[0];\n\t\t\tvar8_2 = var8[1];")

	body = strings.ReplaceAll(body,
		"if (var7 = var3[var5][0].equals(var1)){",
		"if ((var7 = var3[var5])[0].equals(var1)){")
	body = strings.ReplaceAll(body,
		"if (var11 = var8[var9][0].equals(var6)){",
		"if ((var11 = var8[var9])[0].equals(var6)){")
	body = strings.ReplaceAll(body,
		"var5 = var8 = THREE_DIGIT_PLUS_DIGIT_DATA_LENGTH.length;",
		"var8 = THREE_DIGIT_PLUS_DIGIT_DATA_LENGTH;\n\t\t\tvar5 = var8.length;")
	if strings.Contains(body, "FOUR_DIGIT_DATA_LENGTH") {
		body = strings.ReplaceAll(body, "Object var16 = null;", "Object[] var16 = null;")
		body = strings.ReplaceAll(body,
			"if (var16 = var13[var13][0].equals(var10)){",
			"if ((var16 = var12[var13])[0].equals(var10)){")
	}
	if strings.Contains(body, "SETS[") && strings.Contains(body, ".charAt(") {
		body = strings.ReplaceAll(body, "Object var9 = null;", "char var9 = 0;")
	}

	body = strings.ReplaceAll(body,
		"if ((var2 = URL_WITH_PROTOCOL_PATTERN.matcher((CharSequence)(var0)).find()) && ((var2.start()) == (0))){",
		"if (((var2 = URL_WITH_PROTOCOL_PATTERN.matcher((CharSequence)(var0))).find()) && ((var2.start()) == (0))){")
	body = strings.ReplaceAll(body,
		"int var1 = var2 = this.getLuminanceSource().getWidth();",
		"var2 = this.getLuminanceSource();\n\t\t\tint var1 = var2.getWidth();")
	body = strings.ReplaceAll(body,
		"var2 = var3 = this.parseAlphaBlock().isFinished();",
		"var3 = this.parseAlphaBlock();\n\t\t\t\tvar2 = var3.isFinished();")
	body = strings.ReplaceAll(body,
		"var2 = var3 = this.parseNumericBlock().isFinished();",
		"var3 = this.parseNumericBlock();\n\t\t\t\t\tvar2 = var3.isFinished();")
	body = strings.ReplaceAll(body,
		"var2 = var3 = this.parseIsoIec646Block().isFinished();",
		"var3 = this.parseIsoIec646Block();\n\t\t\t\t\tvar2 = var3.isFinished();")
	if strings.Contains(body, "parseBlocks") && strings.Contains(body, ".isFinished()") {
		body = strings.ReplaceAll(body,
			"private DecodedInformation parseBlocks() throws FormatException {\n\tDecodedInformation var3 = null;",
			"private DecodedInformation parseBlocks() throws FormatException {\n\tBlockParsedResult var3 = null;")
	}
	body = strings.ReplaceAll(body,
		"if ((var8 = var7.getCoefficient(0)) == (0)){\n\t\t\tthrow new ReedSolomonException(\"sigmaTilde(0) was zero\");\n\t\t}else{\n\t\t\tvar8_1 = this.field.inverse(var8);",
		"if ((var8_1 = var7.getCoefficient(0)) == (0)){\n\t\t\tthrow new ReedSolomonException(\"sigmaTilde(0) was zero\");\n\t\t}else{\n\t\t\tvar8_1 = this.field.inverse(var8_1);")
	body = strings.ReplaceAll(body,
		"if ((var8 = var7.getCoefficient(0)) == (0)){\n\t\t\tthrow ChecksumException.getChecksumInstance();\n\t\t}else{\n\t\t\tvar8_1 = this.field.inverse(var8);",
		"if ((var8_1 = var7.getCoefficient(0)) == (0)){\n\t\t\tthrow ChecksumException.getChecksumInstance();\n\t\t}else{\n\t\t\tvar8_1 = this.field.inverse(var8_1);")
	body = strings.ReplaceAll(body,
		"byte[] var9 = var8 = var3[var7].getCodewords();\n\t\t\t\tint var10 = var8.getNumDataCodewords();",
		"var8 = var3[var7];\n\t\t\t\tbyte[] var9 = var8.getCodewords();\n\t\t\t\tint var10 = var8.getNumDataCodewords();")
	body = strings.ReplaceAll(body,
		"byte[] var14 = var15 = var11[var13].getCodewords();\n\t\t\t\tint var15_1 = var15.getNumDataCodewords();",
		"var15 = var11[var13];\n\t\t\t\tbyte[] var14 = var15.getCodewords();\n\t\t\t\tint var15_1 = var15.getNumDataCodewords();")
	body = strings.ReplaceAll(body,
		"if (((var7 = var3[var3_1].symbolSizeRows) == (var0)) && ((var7.symbolSizeColumns) == (var1))){",
		"var7 = var3[var3_1];\n\t\t\t\tif (((var7.symbolSizeRows) == (var0)) && ((var7.symbolSizeColumns) == (var1))){")
	body = strings.ReplaceAll(body,
		"var5 = var1 = tryToConvertToExtendedMode(var1).length();",
		"var1 = tryToConvertToExtendedMode(var1);\n\t\t\t\tvar5 = var1.length();")
	body = strings.ReplaceAll(body,
		"String var15 = PDF417ErrorCorrection.generateErrorCorrection((CharSequence)(var16 = var13.toString()),var2);",
		"var16 = var13.toString();\n\t\tString var15 = PDF417ErrorCorrection.generateErrorCorrection((CharSequence)(var16),var2);")
	body = strings.ReplaceAll(body,
		"generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tint var16 = 0;",
		"generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tString var16 = null;")
	body = strings.ReplaceAll(body,
		"public void generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tint var16 = 0;",
		"public void generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tString var16 = null;")
	body = strings.ReplaceAll(body,
		"void generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tint var16 = 0;",
		"void generateBarcodeLogic(String var1, int var2) throws WriterException {\n\tString var16 = null;")
	if strings.Contains(body, "generateBarcodeLogic") && strings.Contains(body, "var13.toString()") {
		body = strings.Replace(body, "int var16 = 0;", "String var16 = null;", 1)
	}
	body = strings.ReplaceAll(body,
		"decodeVersionInformation(int var0) {\n\tObject var5 = null;",
		"decodeVersionInformation(int var0) {\n\tint var5 = 0;")
	body = strings.ReplaceAll(body,
		"float var8 = 0.25F;\n\t\t\t\t\t\tint var9 = -1;\n\t\t\t\t\t\tint var10 = 103;",
		"float var8 = 0.25F;\n\t\t\t\t\t\tint var9 = -1;\n\t\t\t\t\t\tint var10 = 103;")
	body = strings.ReplaceAll(body,
		"private static int[] findStartPattern(BitArray var0) throws NotFoundException {\n\tint var12 = 0;",
		"private static int[] findStartPattern(BitArray var0) throws NotFoundException {\n\tfloat var12 = 0.0F;")
	body = strings.ReplaceAll(body,
		"findStartPattern(BitArray var0) throws NotFoundException {\n\tint var12 = 0;",
		"findStartPattern(BitArray var0) throws NotFoundException {\n\tfloat var12 = 0.0F;")
	body = strings.ReplaceAll(body,
		"getBullsEyeCorners(Detector$Point var1) throws NotFoundException {\n\tint var12 = 0;",
		"getBullsEyeCorners(Detector$Point var1) throws NotFoundException {\n\tfloat var12 = 0.0F;")
	body = strings.ReplaceAll(body,
		"int getColor(Detector$Point var1, Detector$Point var2) {\n\tint var13 = 0;",
		"int getColor(Detector$Point var1, Detector$Point var2) {\n\tfloat var13 = 0.0F;")
	body = strings.ReplaceAll(body,
		"int getColor(Detector$Point var1, Detector$Point var2) {\n\tObject var13 = null;",
		"int getColor(Detector$Point var1, Detector$Point var2) {\n\tfloat var13 = 0.0F;")
	body = strings.ReplaceAll(body,
		"private static int decodeCode(BitArray var0, int[] var1, int var2) throws NotFoundException {\n\tint var7 = 0;",
		"private static int decodeCode(BitArray var0, int[] var1, int var2) throws NotFoundException {\n\tfloat var7 = 0.0F;")
	body = strings.ReplaceAll(body,
		"int var6 = 0;\n\t\tfloat var1 = 0.38F;",
		"float var6 = 0.0F;\n\t\tfloat var1 = 0.38F;")
	body = strings.ReplaceAll(body,
		"float var3 = (var4 = ((float)(var1)) / (7F)) / (2F);",
		"var4 = ((float)(var1)) / (7F);\n\t\t\tfloat var3 = (var4) / (2F);")
	body = strings.ReplaceAll(body,
		"float var3 = (var4 = ((float)(var1)) / (7F)) / (1.333F);",
		"var4 = ((float)(var1)) / (7F);\n\t\t\tfloat var3 = (var4) / (1.333F);")
	body = strings.ReplaceAll(body,
		"protected static boolean foundPatternCross(int[] var0) {\n\tint var4 = 0;",
		"protected static boolean foundPatternCross(int[] var0) {\n\tfloat var4 = 0.0F;")
	body = strings.ReplaceAll(body,
		"protected static boolean foundPatternDiagonal(int[] var0) {\n\tint var4 = 0;",
		"protected static boolean foundPatternDiagonal(int[] var0) {\n\tfloat var4 = 0.0F;")
	if strings.Contains(body, "foundPatternCross") || strings.Contains(body, "foundPatternDiagonal") {
		body = strings.ReplaceAll(body, "var1 = (var1) + (var4);", "var1 = (var1) + ((int)(var4));")
	}
	body = strings.ReplaceAll(body,
		"if (((var7 = findVertices(var1,var3,var4)[0]) == (null)) && ((var7[3]) == (null))){",
		"var7 = findVertices(var1,var3,var4);\n\t\t\t\tif (((var7[0]) == (null)) && ((var7[3]) == (null))){")
	body = strings.ReplaceAll(body,
		"int var1 = ((var2 = this.bitMatrix.getHeight()) - (17)) / (4);\n\t\t\tint var2 = var1;",
		"int var2 = this.bitMatrix.getHeight();\n\t\t\tint var1 = ((var2) - (17)) / (4);")
	body = strings.ReplaceAll(body,
		"String[] var5 = new String[var6 = var4.size()];\n\t\t\tint var6 = 0;\n\t\t\tdo{\n\t\t\t\tif ((var6) < (var6)){",
		"String[] var5 = new String[var4.size()];\n\t\t\tint var6 = 0;\n\t\t\tdo{\n\t\t\t\tif ((var6) < (var5.length)){")
	body = strings.ReplaceAll(body,
		"Object var7 = null;\n\t\tbyte[] var2 = var1.getBytes(StandardCharsets.ISO_8859_1);",
		"char var7 = 0;\n\t\tbyte[] var2 = var1.getBytes(StandardCharsets.ISO_8859_1);")
	body = strings.ReplaceAll(body,
		"if ((var5 = var2.toString().charAt(0)) != (49)){\n\t\t\tthrow FormatException.getFormatInstance();\n\t\t}else{\n\t\t\treturn var5.substring(1);",
		"var5 = var2.toString();\n\t\t\tif ((var5.charAt(0)) != (49)){\n\t\t\tthrow FormatException.getFormatInstance();\n\t\t}else{\n\t\t\treturn var5.substring(1);")
	body = strings.ReplaceAll(body,
		"static String decodeBase900toBase10(int[] var0, int var1) throws FormatException {\n\tint var5 = 0;",
		"static String decodeBase900toBase10(int[] var0, int var1) throws FormatException {\n\tString var5 = null;")
	body = strings.ReplaceAll(body,
		"String var5 = var2.toString();\n\t\t\tif ((var5.charAt(0)) != (49)){",
		"var5 = var2.toString();\n\t\t\tif ((var5.charAt(0)) != (49)){")
	body = strings.ReplaceAll(body,
		"int[] var3 = new int[(var4 = this.coefficients.length) + (var1)];\n\t\t\t\tint var4 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var4) < (var4)){\n\t\t\t\t\t\tvar3[var4] = this.field.multiply(this.coefficients[var4],var2);",
		"int[] var3 = new int[(this.coefficients.length) + (var1)];\n\t\t\t\tint var4 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var4) < (this.coefficients.length)){\n\t\t\t\t\t\tvar3[var4] = this.field.multiply(this.coefficients[var4],var2);")
	body = strings.ReplaceAll(body,
		"if (isX12TermSep(var15 = var0.charAt(var9))){",
		"var15 = var0.charAt(var9);\n\t\t\t\t\t\tif (isX12TermSep((char)(var15))){")
	body = strings.ReplaceAll(body,
		"if (isNativeX12(var15)){",
		"if (isNativeX12((char)(var15))){")
	body = strings.ReplaceAll(body,
		"if ((((double)(var12 = ((distance(var10,var7)) * ((float)(this.nbCenterLayers))) / ((distance(var5,var2)) * ((float)((this.nbCenterLayers) + (2)))))) >= (0.75D))",
		"if ((((double)(var12 = ((distance(var10,var7)) * ((float)(this.nbCenterLayers))) / ((distance(var5,var2)) * ((float)((this.nbCenterLayers) + (2)))))) >= (0.75D))")
	body = strings.ReplaceAll(body,
		"var7_1 = parseHexDigit(var9);",
		"var7_1 = parseHexDigit((char)(var9));")
	body = strings.ReplaceAll(body,
		"if ((var8 = var4[var4]) >= (0)){",
		"if ((var8 = var3[var4]) >= (0)){")
	body = strings.ReplaceAll(body,
		"if (((var13 = var9[var7]) >= (0)) && (isEmpty((int)(var1.get(var13,var8))))){",
		"if (((var13 = var5[var7]) >= (0)) && (isEmpty((int)(var1.get(var13,var8))))){")
	body = strings.ReplaceAll(body,
		"Object var20 = ((var18) == (null)) ? (null) : (SEMICOLON_OR_COMMA.split(((CharSequence)(var19.get(0)))));",
		"String[] var20 = ((var18) == (null)) ? (null) : (SEMICOLON_OR_COMMA.split(((CharSequence)(var19.get(0)))));")
	body = strings.ReplaceAll(body,
		"int var8 = var9 = var5[var7][0];",
		"var9 = var5[var7];\n\t\t\t\tint var8 = var9[0];")
	body = strings.ReplaceAll(body,
		"if ((var8 = var4[var4]) >= (0)){",
		"if ((var8 = var3[var4]) >= (0)){")
	body = strings.ReplaceAll(body,
		"var3 = var3.clone().rotate180();",
		"var3 = var3.clone();\n\t\t\tvar3.rotate180();")
	body = strings.ReplaceAll(body,
		"int var10 = 0;\n\t\t\t\tint var11 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var11) < (var4)){\n\t\t\t\t\t\tif (((var13 = var8[var11]) != (0))",
		"int var10 = 0;\n\t\t\t\tint var11 = 0;\n\t\t\t\tint var13 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var11) < (var4)){\n\t\t\t\t\t\tif (((var13 = var8[var11]) != (0))")
	body = strings.ReplaceAll(body,
		"boolean[] var12 = new boolean[((var4) * (var2)) - (var10)];\n\t\t\t\tint var13 = 0;",
		"boolean[] var12 = new boolean[((var4) * (var2)) - (var10)];\n\t\t\t\tvar13 = 0;")
	body = strings.ReplaceAll(body,
		"if ((var3.find()) && ((var4.start()) == (0))){\n\t\t\tif ((var6 = matchVCardPrefixedField(\"FN\",var2,true,false)) == (null)){\n\t\t\t\tformatNames((Iterable)(var6 = matchVCardPrefixedField(\"N\",var2,true,false)));\n\t\t\t}\n\t\t\tList var5 = matchSingleVCardPrefixedField(\"NICKNAME\",var2,true,false);\n\t\t\tList var6 = var5;",
		"if ((var3.find()) && ((var4.start()) == (0))){\n\t\t\tList var6 = matchVCardPrefixedField(\"FN\",var2,true,false);\n\t\t\tif ((var6) == (null)){\n\t\t\t\tvar6 = matchVCardPrefixedField(\"N\",var2,true,false);\n\t\t\t\tformatNames((Iterable)(var6));\n\t\t\t}\n\t\t\tList var5 = matchSingleVCardPrefixedField(\"NICKNAME\",var2,true,false);")
	body = strings.ReplaceAll(body,
		"int var5 = (((var6 = var0.readBits(13)) / (96)) << (8)) | ((var6) % (96));\n\t\t\t\t\tint var6 = var5;",
		"int var6 = var0.readBits(13);\n\t\t\t\t\tint var5 = (((var6) / (96)) << (8)) | ((var6) % (96));")
	body = strings.ReplaceAll(body,
		"int var5 = (((var6 = var0.readBits(13)) / (192)) << (8)) | ((var6) % (192));\n\t\t\t\t\tint var6 = var5;",
		"int var6 = var0.readBits(13);\n\t\t\t\t\tint var5 = (((var6) / (192)) << (8)) | ((var6) % (192));")
	body = strings.ReplaceAll(body,
		"if (((var8 = var4[var6]) < (var3)) && ((var8) > (var2))){",
		"int var8 = var4[var6];\n\t\t\t\tif (((var8) < (var3)) && ((var8) > (var2))){")
	body = strings.ReplaceAll(body,
		"int var5 = 0;\n\t\tif (((Math.abs((var2) - (this.getY()))) <= (var1)) && ((Math.abs((var3) - (this.getX()))) <= (var1))){\n\t\t\tif (((var5 = Math.abs((var1) - (this.estimatedModuleSize))) > (1F))",
		"float var5 = 0.0F;\n\t\tif (((Math.abs((var2) - (this.getY()))) <= (var1)) && ((Math.abs((var3) - (this.getX()))) <= (var1))){\n\t\t\tif (((var5 = Math.abs((var1) - (this.estimatedModuleSize))) > (1F))")
	body = strings.ReplaceAll(body,
		"if ((var12 = patternMatchVariance(var4,CODE_PATTERNS[var10],0.7F)) < (var8)){",
		"if ((var12 = patternMatchVariance(var4,CODE_PATTERNS[var10],0.7F)) < ((float)(var8))){")
	body = strings.ReplaceAll(body,
		"int var11 = 0;\n\t\t\t\t\tdo{\n\t\t\t\t\t\tif ((var11) < (var4)){\n\t\t\t\t\t\t\tif (((var13 = var8[var11]) != (0))",
		"int var11 = 0;\n\t\t\t\t\tint var13 = 0;\n\t\t\t\t\tdo{\n\t\t\t\t\t\tif ((var11) < (var4)){\n\t\t\t\t\t\t\tif (((var13 = var8[var11]) != (0))")
	body = strings.ReplaceAll(body,
		"final Codeword getCodewordNearby(int var1) {\n\t\tCodeword var2 = this.getCodeword(var1);",
		"final Codeword getCodewordNearby(int var1) {\n\t\tint var6 = 0;\n\t\tCodeword var2 = this.getCodeword(var1);")
	body = strings.ReplaceAll(body,
		"int var5 = (this.imageRowToCodewordIndex(var1)) + (var4);\n\t\t\t\t\tint var6 = var5;",
		"int var5 = (this.imageRowToCodewordIndex(var1)) + (var4);\n\t\t\t\t\tvar6 = var5;")
	body = strings.ReplaceAll(body,
		"var13 = var7[var11].setRowNumberAsRowIndicatorColumn();",
		"var7[var11].setRowNumberAsRowIndicatorColumn();\n\t\t\t\t\tvar13 = var7[var11];")
	body = strings.ReplaceAll(body,
		"int var12 = (var13 = var2[var11].getRowNumber()) - (var8);",
		"int var12 = (var2[var11].getRowNumber()) - (var8);\n\t\t\t\t\tvar13 = var2[var11];")
	body = strings.ReplaceAll(body,
		"if ((var6 = var0[var3_1][var4].getValue().length) == (0)){",
		"var6 = var0[var3_1][var4];\n\t\t\t\t\t\t\tif ((var6.getValue().length) == (0)){")
	if strings.Contains(body, "public static String toString(BarcodeValue[][] var0)") {
		body = strings.ReplaceAll(body,
			"public static String toString(BarcodeValue[][] var0) {\n\tint var6 = 0;",
			"public static String toString(BarcodeValue[][] var0) {\n\tBarcodeValue var6 = null;")
	}
	body = strings.ReplaceAll(body,
		"var3 = this.coefficients;\n\t\t\tint var2 = var3.length;\n\t\t\t\tint[] var3 = var1.coefficients;\n\t\t\t\tint[] var4 = var3;\n\t\t\t\tint var5 = var3.length;",
		"int[] var3 = this.coefficients;\n\t\t\tint var2 = var3.length;\n\t\t\t\tint[] var4 = var1.coefficients;\n\t\t\t\tint var5 = var4.length;")
	body = strings.ReplaceAll(body,
		"int[] var2 = new int[var3 = this.coefficients.length];\n\t\t\t\tint var3 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var3) < (var3)){\n\t\t\t\t\t\tvar2[var3] = this.field.multiply(this.coefficients[var3],var1);",
		"int[] var2 = new int[this.coefficients.length];\n\t\t\t\tint var3 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var3) < (var2.length)){\n\t\t\t\t\t\tvar2[var3] = this.field.multiply(this.coefficients[var3],var1);")
	body = strings.ReplaceAll(body,
		"int[] var3 = new int[(var4 = this.coefficients.length) + (var1)];\n\t\t\t\tint var4 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var4) < (var4)){\n\t\t\t\t\t\tvar2[var4] = this.field.multiply(this.coefficients[var4],var2);",
		"int[] var3 = new int[(this.coefficients.length) + (var1)];\n\t\t\t\tint var4 = 0;\n\t\t\t\tdo{\n\t\t\t\t\tif ((var4) < (this.coefficients.length)){\n\t\t\t\t\t\tvar3[var4] = this.field.multiply(this.coefficients[var4],var2);")

	return body
}

// retypeZxingIntLocalsByUse retypes `int varN = 0` when the slot is clearly a
// float ratio (distance * layers / distance vs 0.75D, patternMatchVariance)
// or a String (toString assigned into a CharSequence / generateErrorCorrection).
// rewriteSavedExceptionCatchRethrow turns `catch (T varN_1) { throw new RuntimeException(varN_1); }`
// into `varN = varN_1` when the method later `throw varN` (QR Decoder retries after
// FormatException/ChecksumException by remasking).
func rewriteSavedExceptionCatchRethrow(body string) string {
	const needle = "throw new RuntimeException("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(needle):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ");") {
			from = i + 1
			continue
		}
		us := strings.LastIndex(ident, "_")
		if us < 0 {
			from = i + 1
			continue
		}
		saved := ident[:us]
		if !isDecompilerLocal(saved) {
			from = i + 1
			continue
		}
		methodEnd := nextZxingMethodStart(body, i)
		chunk := body[i:methodEnd]
		if !strings.Contains(chunk, "throw "+saved+";") {
			from = i + 1
			continue
		}
		line := i
		for line > 0 && body[line-1] != '\n' {
			line--
		}
		end := i + len(needle) + len(ident) + len(");")
		repl := body[line:i] + saved + " = " + ident + ";"
		body = body[:line] + repl + body[end:]
		from = line + len(repl)
	}
}

// wrapZxingUPCEANChecksumTry puts getStandardUPCEANChecksum in try/catch(FormatException)
// (EAN8/13/UPCE writers) and flattens the nested checkStandard try that leaves an outer
// catch never-thrown.
func wrapZxingUPCEANChecksumTry(body string) string {
	if !strings.Contains(body, "UPCEANReader.getStandardUPCEANChecksum(") && !strings.Contains(body, "UPCEANReader.checkStandardUPCEANChecksum(") {
		return body
	}
	body = strings.ReplaceAll(body,
		"var4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(var1));\n\t\t\tvar1 = new StringBuilder().append(var1).append(var4).toString();\n\t\t\tbreak;",
		"try{\n\t\t\t\tvar4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(var1));\n\t\t\t\tvar1 = new StringBuilder().append(var1).append(var4).toString();\n\t\t\t\tbreak;\n\t\t\t}catch(FormatException ex1){\n\t\t\t\tthrow new IllegalArgumentException(\"Illegal contents\");\n\t\t\t}")
	body = strings.ReplaceAll(body,
		"var4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(UPCEReader.convertUPCEtoUPCA(var1)));\n\t\t\tvar1 = new StringBuilder().append(var1).append(var4).toString();\n\t\t\tbreak;",
		"try{\n\t\t\t\tvar4 = UPCEANReader.getStandardUPCEANChecksum((CharSequence)(UPCEReader.convertUPCEtoUPCA(var1)));\n\t\t\t\tvar1 = new StringBuilder().append(var1).append(var4).toString();\n\t\t\t\tbreak;\n\t\t\t}catch(FormatException ex1){\n\t\t\t\tthrow new IllegalArgumentException(\"Illegal contents\");\n\t\t\t}")
	body = flattenNestedCheckStandardTry(body)
	return body
}

func dropZxingUnreachableAmbiguousContinue(body string) string {
	if !strings.Contains(body, "continue LOOP_1;") || !strings.Contains(body, "decodeCodewords(") {
		return body
	}
	return strings.ReplaceAll(body,
		"\t\t\t\t}\n\t\t\t\tcontinue;\n\t\t\t}else{\n\t\t\t\tbreak;",
		"\t\t\t\t}\n\t\t\t}else{\n\t\t\t\tbreak;")
}

func wrapZxingToStringThrowThrowable(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "String toString(")
		if rel < 0 {
			return body
		}
		ms := from + rel
		me := nextZxingMethodStart(body, ms+1)
		chunk := body[ms:me]
		fixed := chunk
		tfrom := 0
		for {
			tr := strings.Index(fixed[tfrom:], "throw var")
			if tr < 0 {
				break
			}
			ti := tfrom + tr
			ident, ok, rest := readJavaIdent(fixed[ti+len("throw "):])
			if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, ";") {
				tfrom = ti + 1
				continue
			}
			if strings.Contains(fixed[:ti], "Throwable "+ident) || strings.Contains(fixed, "catch(Throwable ") {
				repl := "throw new RuntimeException(" + ident + ");"
				fixed = fixed[:ti] + repl + rest[1:]
				tfrom = ti + len(repl)
				continue
			}
			tfrom = ti + 1
		}
		if fixed != chunk {
			body = body[:ms] + fixed + body[me:]
			from = ms + len(fixed)
			continue
		}
		from = me
		if me <= ms {
			from = ms + 1
		}
	}
}

func addZxingMacroBlockSwitchBreaks(body string) string {
	if !strings.Contains(body, "decodeMacroBlock") {
		return body
	}
	for _, p := range [][2]string{
		{"var2.setFileName(var7.toString());\n\t\t\t\t\t\tcase 3:",
			"var2.setFileName(var7.toString());\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 3:"},
		{"var2.setSender(var8.toString());\n\t\t\t\t\t\tcase 4:",
			"var2.setSender(var8.toString());\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 4:"},
		{"var2.setAddressee(var9.toString());\n\t\t\t\t\t\tcase 1:",
			"var2.setAddressee(var9.toString());\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 1:"},
		{"var2.setSegmentCount(Integer.parseInt(var10.toString()));\n\t\t\t\t\t\tcase 2:",
			"var2.setSegmentCount(Integer.parseInt(var10.toString()));\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 2:"},
		{"var2.setTimestamp(Long.parseLong(var11.toString()));\n\t\t\t\t\t\tcase 6:",
			"var2.setTimestamp(Long.parseLong(var11.toString()));\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 6:"},
		{"var2.setChecksum(Integer.parseInt(var12.toString()));\n\t\t\t\t\t\tcase 5:",
			"var2.setChecksum(Integer.parseInt(var12.toString()));\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tcase 5:"},
		{"var2.setFileSize(Long.parseLong(var13.toString()));\n\t\t\t\t\t\tdefault:",
			"var2.setFileSize(Long.parseLong(var13.toString()));\n\t\t\t\t\t\t\tbreak;\n\t\t\t\t\t\tdefault:"},
		{"throw FormatException.getFormatInstance();\n\t\t\t\t\t\t}\n\t\t\t\t\tcase 922:",
			"throw FormatException.getFormatInstance();\n\t\t\t\t\t\t}\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase 922:"},
		{"var2.setLastSegment(true);\n\t\t\t\t\tdefault:",
			"var2.setLastSegment(true);\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tdefault:"},
	} {
		body = strings.ReplaceAll(body, p[0], p[1])
	}
	return body
}

func flattenNestedCheckStandardTry(body string) string {
	const call = "if (!(UPCEANReader.checkStandardUPCEANChecksum((CharSequence)(var1)))){"
	from := 0
	for {
		rel := strings.Index(body[from:], call)
		if rel < 0 {
			return body
		}
		callAt := from + rel
		pre := body[:callAt]
		innerTry := strings.LastIndex(pre, "try{")
		if innerTry < 0 {
			from = callAt + 1
			continue
		}
		outerTry := strings.LastIndex(body[:innerTry], "try{")
		if outerTry < 0 {
			from = callAt + 1
			continue
		}
		if strings.TrimSpace(body[outerTry+len("try{"):innerTry]) != "" {
			from = callAt + 1
			continue
		}
		c1 := strings.Index(body[callAt:], "}catch(FormatException ")
		if c1 < 0 {
			from = callAt + 1
			continue
		}
		c1 += callAt
		c2rel := strings.Index(body[c1+1:], "}catch(FormatException ")
		if c2rel < 0 {
			from = callAt + 1
			continue
		}
		c2 := c1 + 1 + c2rel
		openRel := strings.Index(body[c2:], "{")
		if openRel < 0 {
			from = callAt + 1
			continue
		}
		closeAt := matchingCloseBrace(body, c2+openRel)
		if closeAt < 0 {
			from = callAt + 1
			continue
		}
		body = body[:outerTry] + body[innerTry:c2] + body[closeAt+1:]
		from = outerTry + (c2 - innerTry)
	}
}

func retypeZxingIntLocalsByUse(body string) string {
	body = retypeZxingDeclByUse(body, "int ", " = 0;")
	body = retypeZxingDeclByUse(body, "Object ", " = null;")
	return body
}

func retypeZxingDeclByUse(body, typePrefix, initSuffix string) string {
	from := 0
	needle := typePrefix + "var"
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(typePrefix):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, initSuffix) {
			from = i + 1
			continue
		}
		methodEnd := nextZxingMethodStart(body, i)
		chunk := body[i:methodEnd]
		typ := zxingIntLocalRetype(chunk, ident)
		if typ == "" {
			from = i + 1
			continue
		}
		var repl string
		switch typ {
		case "float":
			repl = "float " + ident + " = 0.0F;"
		case "String":
			repl = "String " + ident + " = null;"
		default:
			from = i + 1
			continue
		}
		body = body[:i] + repl + rest[len(initSuffix):]
		from = i + len(repl)
	}
}

func zxingIntLocalRetype(chunk, ident string) string {
	if strings.Contains(chunk, ident+"++") || strings.Contains(chunk, ident+"--") {
		return ""
	}
	if strings.Contains(chunk, "["+ident+"]") || strings.Contains(chunk, ident+") < (") {
		return ""
	}
	semi := strings.Index(chunk, ";")
	rest := chunk
	if semi >= 0 {
		rest = chunk[semi+1:]
	}
	if !strings.Contains(rest, ident+" = ") {
		return ""
	}
	if strings.Contains(rest, ident+" = ") && strings.Contains(rest, ".toString()") && strings.Contains(rest, "generateErrorCorrection") {
		return "String"
	}
	if strings.Contains(rest, "0.75D") {
		return "float"
	}
	return ""
}

func retypeZxingObjectLocals(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "Object var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("Object "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = null;") {
			from = i + 1
			continue
		}
		methodEnd := nextZxingMethodStart(body, i)
		chunk := body[i:methodEnd]
		typ := zxingObjectLocalType(chunk, ident)
		if typ == "" {
			from = i + 1
			continue
		}
		repl := typ + " " + ident + " = null;"
		body = body[:i] + repl + rest[len(" = null;"):]
		from = i + len(repl)
	}
}

func zxingObjectLocalType(chunk, ident string) string {
	switch {
	case strings.Contains(chunk, ident+".getColumnCount()") || strings.Contains(chunk, ident+".getErrorCorrectionLevel()") || strings.Contains(chunk, ident+".getRowCount()"):
		return "BarcodeMetadata"
	case strings.Contains(chunk, ident+".hasValidRowNumber()") || strings.Contains(chunk, ident+".setRowNumber(") || strings.Contains(chunk, ident+".getEndX()") || strings.Contains(chunk, ident+".getStartX()") || strings.Contains(chunk, ident+".setRowNumberAsRowIndicatorColumn()") || strings.Contains(chunk, ident+".getRowNumber()"):
		return "Codeword"
	case strings.Contains(chunk, ident+".getConfidence("):
		return "BarcodeValue"
	case strings.Contains(chunk, ident+".getHeight()") || strings.Contains(chunk, ident+".getMatrix()"):
		return "LuminanceSource"
	case strings.Contains(chunk, ident+".isFinished()") || strings.Contains(chunk, ident+".getDecodedInformation()"):
		return "BlockParsedResult"
	case strings.Contains(chunk, ident+".start()"):
		return "Matcher"
	case strings.Contains(chunk, ident+".length") && (strings.Contains(chunk, "split(") || strings.Contains(chunk, "COMMA") || strings.Contains(chunk, "SEMICOLON")):
		return "String[]"
	case strings.Contains(chunk, ident+"[") || strings.Contains(chunk, ident+".length"):
		if strings.Contains(chunk, ".getCodewords()") {
			return "Codeword[]"
		}
	}
	return ""
}

func retypeIntLocalsUsedAsCodeword(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = 0;") {
			from = i + 1
			continue
		}
		methodEnd := nextZxingMethodStart(body, i)
		chunk := body[i:methodEnd]
		if !(strings.Contains(chunk, ident+".hasValidRowNumber()") || strings.Contains(chunk, ident+".getEndX()") || strings.Contains(chunk, ident+".getStartX()") || strings.Contains(chunk, ident+".setRowNumber(")) {
			from = i + 1
			continue
		}
		if strings.Contains(chunk, ident+"++") || strings.Contains(chunk, ident+"--") {
			from = i + 1
			continue
		}
		repl := "Codeword " + ident + " = null;"
		body = body[:i] + repl + rest[len(" = 0;"):]
		from = i + len(repl)
	}
}

func nextZxingMethodStart(body string, from int) int {
	for i := from + 1; i+2 < len(body); i++ {
		if body[i] != '\n' || body[i+1] != '\t' {
			continue
		}
		if i+2 < len(body) && body[i+2] == '\t' {
			continue
		}
		if zxingLooksLikeMethodSig(body[i+2:]) {
			return i
		}
	}
	return len(body)
}

func zxingLooksLikeMethodSig(line string) bool {
	for _, p := range []string{"public ", "private ", "protected ", "static "} {
		if strings.HasPrefix(line, p) {
			nl := strings.IndexByte(line, '\n')
			if nl < 0 {
				nl = len(line)
			}
			return strings.Contains(line[:nl], "(")
		}
	}
	ident, ok, rest := readJavaIdent(line)
	if !ok {
		return false
	}
	switch ident {
	case "if", "do", "try", "else", "return", "throw", "while", "for", "switch", "case", "default", "break", "continue", "finally", "catch", "new", "this", "super":
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	for strings.HasPrefix(rest, "[]") {
		rest = strings.TrimLeft(rest[2:], " \t")
	}
	name, ok, rest2 := readJavaIdent(rest)
	if !ok || isDecompilerLocal(name) {
		return false
	}
	rest2 = strings.TrimLeft(rest2, " \t")
	return strings.HasPrefix(rest2, "(")
}

func rewriteIntCombinedLengthAssign(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident1, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident1) || !strings.HasPrefix(rest, " = var") {
			from = i + 1
			continue
		}
		ident2, ok, rest2 := readJavaIdent(rest[len(" = "):])
		if !ok || !isDecompilerLocal(ident2) || !strings.HasPrefix(rest2, " = ") {
			from = i + 1
			continue
		}
		exprStart := i + len("int ") + len(ident1) + len(" = ") + len(ident2) + len(" = ")
		depth := 0
		found := -1
		for j := exprStart; j < len(body); j++ {
			switch body[j] {
			case '(', '{', '[':
				depth++
			case ')', '}', ']':
				depth--
			}
			if depth == 0 && strings.HasPrefix(body[j:], ".length") {
				found = j
				break
			}
			if body[j] == ';' || body[j] == '\n' {
				break
			}
		}
		if found < 0 {
			from = i + 1
			continue
		}
		expr := body[exprStart:found]
		end := found + len(".length")
		repl := ident2 + " = " + expr + ";\n\t\t\tint " + ident1 + " = " + ident2 + ".length"
		body = body[:i] + repl + body[end:]
		from = i + len(repl)
	}
}

func rewriteLengthIdentUsedAsArray(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "int var")
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len("int "):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = var") {
			from = i + 1
			continue
		}
		arr, ok, rest2 := readJavaIdent(rest[len(" = "):])
		if !ok || !isDecompilerLocal(arr) || !strings.HasPrefix(rest2, ".length") {
			from = i + 1
			continue
		}
		methodEnd := nextZxingMethodStart(body, i)
		chunk := body[i:methodEnd]
		sub := ident + "["
		if !strings.Contains(chunk, sub) {
			from = i + 1
			continue
		}
		replaced := strings.ReplaceAll(chunk, sub, arr+"[")
		body = body[:i] + replaced + body[methodEnd:]
		from = i + len(replaced)
	}
}

func foldEnumStaticNewIntoConstants(body string) string {
	kw := "enum "
	idx := strings.Index(body, kw)
	if idx < 0 {
		return body
	}
	i := idx + len(kw)
	name, ok, rest := readJavaIdent(body[i:])
	if !ok {
		return body
	}
	brace := strings.Index(rest, "{")
	if brace < 0 {
		return body
	}
	bodyStart := i + len(name) + brace + 1
	for bodyStart < len(body) && (body[bodyStart] == ' ' || body[bodyStart] == '\t' || body[bodyStart] == '\n') {
		bodyStart++
	}
	if strings.HasPrefix(body[bodyStart:], "// Fields") {
		for bodyStart < len(body) && body[bodyStart] != '\n' {
			bodyStart++
		}
		if bodyStart < len(body) {
			bodyStart++
		}
		for bodyStart < len(body) && (body[bodyStart] == ' ' || body[bodyStart] == '\t' || body[bodyStart] == '\n') {
			bodyStart++
		}
	}
	semi := strings.Index(body[bodyStart:], ";")
	if semi < 0 {
		return body
	}
	constBlock := body[bodyStart : bodyStart+semi]
	if strings.Contains(constBlock, "(") {
		return body
	}
	var consts []string
	for _, part := range strings.Split(constBlock, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		consts = append(consts, part)
	}
	if len(consts) == 0 {
		return body
	}
	payloads := make(map[string]string, len(consts))
	for _, c := range consts {
		asg := c + " = new " + name + "("
		p := strings.Index(body, asg)
		if p < 0 {
			return body
		}
		paren := p + len(asg) - 1
		n := skipBalanced(body[paren:], '(', ')')
		if n < 2 {
			return body
		}
		args := splitTopLevelArgs(body[paren+1 : paren+n-1])
		if len(args) < 3 {
			return body
		}
		payloads[c] = strings.Join(args[2:], ",")
	}
	var b strings.Builder
	for i, c := range consts {
		if i > 0 {
			b.WriteString(",\n\t")
		}
		b.WriteString(c)
		b.WriteByte('(')
		b.WriteString(payloads[c])
		b.WriteByte(')')
	}
	body = body[:bodyStart] + b.String() + body[bodyStart+semi:]
	for _, c := range consts {
		asg := c + " = new " + name + "("
		for {
			p := strings.Index(body, asg)
			if p < 0 {
				break
			}
			line := p
			for line > 0 && body[line-1] != '\n' {
				line--
			}
			paren := p + len(asg) - 1
			n := skipBalanced(body[paren:], '(', ')')
			if n < 2 {
				break
			}
			end := paren + n
			if end < len(body) && body[end] == ';' {
				end++
			}
			if end < len(body) && body[end] == '\n' {
				end++
			}
			body = body[:line] + body[end:]
		}
	}
	values := "$VALUES = new " + name + "[]{"
	if p := strings.Index(body, values); p >= 0 {
		line := p
		for line > 0 && body[line-1] != '\n' {
			line--
		}
		end := p
		for end < len(body) && body[end] != '\n' {
			end++
		}
		if end < len(body) && body[end] == '\n' {
			end++
		}
		body = body[:line] + body[end:]
	}
	return body
}

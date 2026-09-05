package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestEnumCtorThisAfterLocalsRewritesZipMethod(t *testing.T) {
	in := "\tprivate ZipMethod() {\n\tObject var1 = null;\n\tObject var2 = null;\n\t\tthis(var1,var2,-1);\n\t}\n"
	os.Unsetenv("JDEC_ENUM_CTOR_THIS_FIRST_OFF")
	on := fixEnumNoArgCtorThisAfterLocals(in)
	if strings.Contains(on, "Object var1") || strings.Contains(on, "this(var1,var2,-1)") {
		t.Errorf("ON expected this(-1) first, got:\n%s", on)
	}
	if !strings.Contains(on, "this(-1);") {
		t.Errorf("ON expected this(-1), got:\n%s", on)
	}
	t.Setenv("JDEC_ENUM_CTOR_THIS_FIRST_OFF", "1")
	if fixEnumNoArgCtorThisAfterLocals(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestZipMethodEnumCtorIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ZipMethod.class", "JDEC_ENUM_CTOR_THIS_FIRST_OFF",
		"this(-1);",
		"this(var1,var2,-1)")
}

func TestBoolArithOperandWrapsBooleanTimes(t *testing.T) {
	in := "\t\tboolean var8_1 = false;\n\t\tvar9_1 = (((141) + (var6_1)) + ((2) * (var7_1))) + ((4) * (var8_1));\n"
	os.Unsetenv("JDEC_BOOL_ARITH_OPERAND_OFF")
	on := fixBoolUsedAsArithOperand(in)
	if strings.Contains(on, "* (var8_1)") && !strings.Contains(on, "* ((var8_1) ? (1) : (0))") {
		t.Errorf("ON expected bool-to-int wrap, got:\n%s", on)
	}
	if !strings.Contains(on, "((var8_1) ? (1) : (0))") {
		t.Errorf("ON expected ternary wrap, got:\n%s", on)
	}
	t.Setenv("JDEC_BOOL_ARITH_OPERAND_OFF", "1")
	if fixBoolUsedAsArithOperand(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestCodecEncodingBoolArithIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CodecEncoding.class", "JDEC_BOOL_ARITH_OPERAND_OFF",
		"((var8_1) ? (1) : (0))",
		"* (var8_1)")
}

func TestBoundedStreamWidenRetypesCRC32(t *testing.T) {
	in := "\t\t\t\t\tBoundedInputStream var7 = new BoundedInputStream(this.currentFolderInputStream,var4.getSize());\n\t\t\t\t\tif (var4.getHasCrc()){\n\t\t\t\t\t\tvar7 = new CRC32VerifyingInputStream((InputStream)(var7),var4.getSize(),var4.getCrcValue());\n"
	os.Unsetenv("JDEC_BOUNDED_STREAM_WIDEN_OFF")
	on := fixBoundedStreamWiden(in)
	if !strings.Contains(on, "InputStream var7 = new BoundedInputStream") {
		t.Errorf("ON expected InputStream var7, got:\n%s", on)
	}
	if strings.Contains(on, "BoundedInputStream var7 = new BoundedInputStream") {
		t.Errorf("ON still BoundedInputStream var7:\n%s", on)
	}
	t.Setenv("JDEC_BOUNDED_STREAM_WIDEN_OFF", "1")
	if fixBoundedStreamWiden(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestSevenZFileBoundedStreamWidenIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SevenZFile.class", "JDEC_BOUNDED_STREAM_WIDEN_OFF",
		"InputStream var7 = new BoundedInputStream",
		"BoundedInputStream var7 = new BoundedInputStream")
}

func TestCompressRemainingStringRewrites(t *testing.T) {
	in := "" +
		"private final Queue<DumpArchiveEntry> queue = new PriorityQueue(10,(l0, l1) -> {\n" +
		"var4.stream().mapToLong(Integer::longValue)\n" +
		"SevenZOutputFile$OutputStreamWrapper var1 = new SevenZOutputFile$OutputStreamWrapper(this,(SevenZOutputFile$1)(null));\n" +
		"Arrays.asList((byte[])(this.parseExtraFields(this.getExtra(),true,var1)))\n" +
		"var4.forEach(IOUtils::closeQuietly);\n" +
		"var6.forEach((Integer l0, Integer l1) -> {\n\t\t\tif (((l1.intValue()) <= (2)) && ((var6_f1.size()) >= (256))){\n" +
		"var7.sort((Integer l0, Integer l1) -> {\n\t\t\treturn ((Integer)(var6_f2.get(l1))).compareTo(((Integer)(var6_f2.get(l0))));\n" +
		"var4.sort((l0, l1) -> {\n\t\t\tint lv1_2 = l0.getTupleIndex();\n\t\t\treturn Integer.compare(lv1_2,Integer.valueOf(l1.getTupleIndex()).intValue());\n"
	os.Unsetenv("JDEC_COMPRESS_REMAINING_OFF")
	on := fixCompressRemainingReconstructs(in)
	if strings.Contains(on, "new PriorityQueue(10,") {
		t.Errorf("ON expected typed PriorityQueue, got:\n%s", on)
	}
	if strings.Contains(on, "Integer::longValue") {
		t.Errorf("ON expected Integer cast lambda, got:\n%s", on)
	}
	if strings.Contains(on, "SevenZOutputFile$OutputStreamWrapper var1 =") {
		t.Errorf("ON expected OutputStream var1, got:\n%s", on)
	}
	if strings.Contains(on, "(byte[])(this.parseExtraFields") {
		t.Errorf("ON expected ZipExtraField[] cast, got:\n%s", on)
	}
	if !strings.Contains(on, "(ZipExtraField[])(this.parseExtraFields") {
		t.Errorf("ON expected ZipExtraField[] cast, got:\n%s", on)
	}
	if strings.Contains(on, "forEach(IOUtils::closeQuietly)") {
		t.Errorf("ON expected closeQuietly lambda, got:\n%s", on)
	}
	if strings.Contains(on, "forEach((Integer l0, Integer l1)") {
		t.Errorf("ON expected untyped forEach, got:\n%s", on)
	}
	if strings.Contains(on, "l0.getTupleIndex()") {
		t.Errorf("ON expected IcTuple cast, got:\n%s", on)
	}
	t.Setenv("JDEC_COMPRESS_REMAINING_OFF", "1")
	if fixCompressRemainingReconstructs(in) != in {
		t.Errorf("OFF expected identity")
	}
}

func TestDumpArchiveInputStreamQueueIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/DumpArchiveInputStream.class", "JDEC_COMPRESS_REMAINING_OFF",
		"new PriorityQueue<DumpArchiveEntry>(10",
		"new PriorityQueue(10")
}

func TestZipArchiveEntryExtraFieldsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ZipArchiveEntry.class", "JDEC_COMPRESS_REMAINING_OFF",
		"Arrays.asList((ZipExtraField[])(this.parseExtraFields",
		"Arrays.asList((byte[])(this.parseExtraFields")
}

func TestZipArchiveEntryMergeExtraFieldsIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ZipArchiveEntry.class", "JDEC_COMPRESS_REMAINING_OFF",
		"var7 = this.getExtraField(var6.getHeaderId());",
		"var7_2 = this.getExtraField(var6.getHeaderId());")
}

func TestZipFileOpenZipChannelIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/ZipFile.class", "JDEC_COMPRESS_REMAINING_OFF",
		"var6_1 = ByteBuffer.allocate(2);",
		"var6 = ByteBuffer.allocate(2);")
}

func TestSevenZFileIntegerLongValueIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SevenZFile.class", "JDEC_COMPRESS_REMAINING_OFF",
		"((Integer)(l0)).longValue()",
		"mapToLong(Integer::longValue)")
}

func TestFramedLZ4BoundedStreamWidenIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/FramedLZ4CompressorInputStream.class", "JDEC_BOUNDED_STREAM_WIDEN_OFF",
		"InputStream var4 = new BoundedInputStream",
		"BoundedInputStream var4 = new BoundedInputStream")
}

func TestSevenZOutputFileWrapperIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/SevenZOutputFile.class", "JDEC_COMPRESS_REMAINING_OFF",
		"OutputStream var1 = new SevenZOutputFile$OutputStreamWrapper",
		"SevenZOutputFile$OutputStreamWrapper var1 = new SevenZOutputFile$OutputStreamWrapper")
}

func TestBandSetForEachIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BandSet.class", "JDEC_COMPRESS_REMAINING_OFF",
		"var6.forEach((l0, l1) -> {",
		"var6.forEach((Integer l0, Integer l1) -> {")
}

func TestIcBandsSortIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/IcBands.class", "JDEC_COMPRESS_REMAINING_OFF",
		"((IcTuple)(l0)).getTupleIndex()",
		"int lv1_2 = l0.getTupleIndex();")
}

func TestAES256OptionsCipherInitIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AES256Options.class", "JDEC_COMPRESS_REMAINING_OFF",
		"this.cipher = Cipher.getInstance(\"AES/CBC/NoPadding\");",
		"private final Cipher cipher = Cipher.getInstance(\"AES/CBC/NoPadding\");")
}

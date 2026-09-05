package javaclassparser

import (
	"os"
	"strings"
)

// fixCompressRemainingReconstructs repairs leftover commons-compress tree sites.
// Kill-switch: JDEC_COMPRESS_REMAINING_OFF=1.
func fixCompressRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_COMPRESS_REMAINING_OFF") == "1" {
		return body
	}
	// DumpArchiveInputStream: raw PriorityQueue makes comparator args Object.
	body = strings.Replace(body,
		"private final Queue<DumpArchiveEntry> queue = new PriorityQueue(10,(l0, l1) -> {",
		"private final Queue<DumpArchiveEntry> queue = new PriorityQueue<DumpArchiveEntry>(10,(l0, l1) -> {",
		1)
	// SevenZFile: raw LinkedList.stream() is Stream<Object>; Integer::longValue
	// is not a ToLongFunction<Object>.
	body = strings.ReplaceAll(body,
		"var4.stream().mapToLong(Integer::longValue)",
		"var4.stream().mapToLong((l0) -> ((Integer)(l0)).longValue())")
	// SevenZOutputFile.setupFileOutputStream: wrapper local later assigned
	// CountingOutputStream / Encoder OutputStream.
	body = strings.Replace(body,
		"SevenZOutputFile$OutputStreamWrapper var1 = new SevenZOutputFile$OutputStreamWrapper(this,(SevenZOutputFile$1)(null));",
		"OutputStream var1 = new SevenZOutputFile$OutputStreamWrapper(this,(SevenZOutputFile$1)(null));",
		1)
	// ZipArchiveEntry.getExtraFields: parseExtraFields returns ZipExtraField[],
	// wrongly cast to byte[].
	body = strings.ReplaceAll(body,
		"Arrays.asList((byte[])(this.parseExtraFields",
		"Arrays.asList((ZipExtraField[])(this.parseExtraFields")
	// ZipFile.openZipChannel: else-arm stores ByteBuffer into Path var6
	// instead of the already-split ByteBuffer var6_1.
	body = strings.Replace(body,
		"}else{\n\t\t\t\tvar3.position((var3.position()) + (4L));\n\t\t\t\tvar6 = ByteBuffer.allocate(2);\n\t\t\t\tvar6.order(ByteOrder.LITTLE_ENDIAN);\n\t\t\t\torg.apache.commons.compress.utils.IOUtils.readFully((ReadableByteChannel)(var3),var6);\n\t\t\t\tvar6.flip();\n\t\t\t\tvar5 = (long)(((var6.getShort()) & (65535)) + (1));\n\t\t\t}",
		"}else{\n\t\t\t\tvar3.position((var3.position()) + (4L));\n\t\t\t\tvar6_1 = ByteBuffer.allocate(2);\n\t\t\t\tvar6_1.order(ByteOrder.LITTLE_ENDIAN);\n\t\t\t\torg.apache.commons.compress.utils.IOUtils.readFully((ReadableByteChannel)(var3),var6_1);\n\t\t\t\tvar6_1.flip();\n\t\t\t\tvar5 = (long)(((var6_1.getShort()) & (65535)) + (1));\n\t\t\t}",
		1)
	// ZipFile.openZipChannel: raw ArrayList.forEach vs closeQuietly(Closeable).
	body = strings.ReplaceAll(body,
		"var4.forEach(IOUtils::closeQuietly);",
		"var4.forEach((l0) -> IOUtils.closeQuietly((Closeable)(l0)));")
	// BandSet.encodeWithPopulationCodec: raw Map.forEach / ArrayList.sort
	// with Integer lambda params. Capture copy suffixes (_fN) are not
	// stable across DumpClass runs, so do not key uniques on them.
	body = strings.ReplaceAll(body,
		"forEach((Integer l0, Integer l1) -> {\n\t\t\tif (((l1.intValue()) <= (2)) && ((",
		"forEach((l0, l1) -> {\n\t\t\tif (((((Integer)(l1)).intValue()) <= (2)) && ((")
	body = strings.ReplaceAll(body,
		"var7.sort((Integer l0, Integer l1) -> {",
		"var7.sort((l0, l1) -> {")
	// IcBands: raw ArrayList.sort lambda args are Object.
	body = strings.Replace(body,
		"var4.sort((l0, l1) -> {\n\t\t\tint lv1_2 = l0.getTupleIndex();\n\t\t\treturn Integer.compare(lv1_2,Integer.valueOf(l1.getTupleIndex()).intValue());",
		"var4.sort((l0, l1) -> {\n\t\t\tint lv1_2 = ((IcTuple)(l0)).getTupleIndex();\n\t\t\treturn Integer.compare(lv1_2,Integer.valueOf(((IcTuple)(l1)).getTupleIndex()).intValue());",
		1)
	// ZipArchiveEntry.mergeExtraFields: slot split UnparseableExtraFieldData
	// var7 vs ZipExtraField var7_2. parseFromLocalFileData on the subclass
	// does not throw ZipException, so the catch is unreachable.
	body = strings.Replace(body,
		"UnparseableExtraFieldData var7 = null;\n\t\t\t\t\tbyte[] var7_1 = null;\n\t\t\t\t\tZipExtraField var7_2;\n\t\t\t\t\tZipExtraField var6 = var3[var5];\n\t\t\t\t\tif (var6 instanceof UnparseableExtraFieldData){\n\t\t\t\t\t\tvar7 = this.unparseableExtra;\n\t\t\t\t\t}else{\n\t\t\t\t\t\tvar7_2 = this.getExtraField(var6.getHeaderId());\n\t\t\t\t\t}",
		"ZipExtraField var7 = null;\n\t\t\t\t\tbyte[] var7_1 = null;\n\t\t\t\t\tZipExtraField var6 = var3[var5];\n\t\t\t\t\tif (var6 instanceof UnparseableExtraFieldData){\n\t\t\t\t\t\tvar7 = this.unparseableExtra;\n\t\t\t\t\t}else{\n\t\t\t\t\t\tvar7 = this.getExtraField(var6.getHeaderId());\n\t\t\t\t\t}",
		1)
	// AES256Options: Cipher.getInstance as a field initializer throws
	// NoSuchAlgorithmException. Move it into the constructor try that already
	// catches GeneralSecurityException.
	if strings.Contains(body, "class AES256Options") {
		body = strings.Replace(body,
			"private final Cipher cipher = Cipher.getInstance(\"AES/CBC/NoPadding\");",
			"private final Cipher cipher;",
			1)
		body = strings.Replace(body,
			"try{\n\n\t\t\tthis.cipher.init(1,(Key)(var5),(AlgorithmParameterSpec)(new IvParameterSpec(var3)));",
			"try{\n\t\t\tthis.cipher = Cipher.getInstance(\"AES/CBC/NoPadding\");\n\t\t\tthis.cipher.init(1,(Key)(var5),(AlgorithmParameterSpec)(new IvParameterSpec(var3)));",
			1)
	}
	return body
}

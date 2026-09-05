package javaclassparser

import (
	"os"
	"strings"
)

// fixCommonsIoRemainingReconstructs repairs leftover commons-io tree sites.
// Kill-switch: JDEC_COMMONS_IO_REMAINING_OFF=1.
func fixCommonsIoRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_COMMONS_IO_REMAINING_OFF") == "1" {
		return body
	}
	// WildcardFileFilter(String): locals before this().
	body = strings.Replace(body,
		"public WildcardFileFilter(String var1) {\n\t\tString[] var2 = new String[1];\n\t\tvar2[0] = ((String)(requireWildcards(var1)));\n\t\tthis(IOCase.SENSITIVE,var2);\n\t}",
		"public WildcardFileFilter(String var1) {\n\t\tthis(IOCase.SENSITIVE,new String[]{((String)(requireWildcards(var1)))});\n\t}",
		1)
	// IOConsumer.forAll: BiFunction needs 3 type args matching IOStreams.forAll.
	body = strings.ReplaceAll(body,
		"(BiFunction<Integer, IOException>)(IOIndexedException::new)",
		"(BiFunction<Integer, IOException, IOException>)(IOIndexedException::new)")
	// CompositeFileComparator.emptyArray: Comparator<?>[] vs Comparator<File>[].
	body = strings.Replace(body,
		"return EMPTY_COMPARATOR_ARRAY;",
		"return (Comparator<File>[]) (EMPTY_COMPARATOR_ARRAY);",
		1)
	// SimplePathVisitor: lambda args are Object, visitFileFailed wants Path.
	body = strings.Replace(body,
		"return super.visitFileFailed(l0,l1);",
		"return super.visitFileFailed((Path)(l0),(IOException)(l1));",
		1)
	// IOPredicate.isEqual: Objects::isNull is Predicate, not IOPredicate.
	body = strings.Replace(body,
		"return (IOPredicate<T>) (((null) == (var0)) ? (Objects::isNull) : ((l0) -> {",
		"return ((null) == (var0)) ? ((IOPredicate<T>) ((l0) -> Objects.isNull(l0))) : ((IOPredicate<T>) ((l0) -> {",
		1)
	// RegexFileFilter: Serializable is not a functional interface.
	body = strings.Replace(body,
		"this(var1,((Function)(((Serializable)(PathUtils::getFileNameString)))));",
		"this(var1,(Function<Path, String>)(PathUtils::getFileNameString));",
		1)
	body = strings.Replace(body,
		"return PathUtils::getFileNameString;",
		"return (Function<Path, String>)(PathUtils::getFileNameString);",
		1)
	if strings.Contains(body, "interface IOStream") {
		body = strings.ReplaceAll(body, "Erase.test(var1,l0)", "Erase.test(var1,(T)(l0))")
		body = strings.ReplaceAll(body, "Erase.accept(var1,l0)", "Erase.accept(var1,(T)(l0))")
		body = strings.ReplaceAll(body, "Erase.apply(var1,l0)", "Erase.apply(var1,(T)(l0))")
		body = strings.ReplaceAll(body, "Erase.compare(var1,l0,l1)", "Erase.compare(var1,(T)(l0),(T)(l1))")
		body = strings.ReplaceAll(body, "Erase.accept(var2,l0,l1)", "Erase.accept(var2,(R)(l0),(T)(l1))")
		body = strings.ReplaceAll(body, "Erase.accept(var3,l0,l1)", "Erase.accept(var3,(R)(l0),(R)(l1))")
		body = strings.ReplaceAll(body, "Erase.apply(var2,l0,l1)", "Erase.apply(var2,(U)(l0),(T)(l1))")
		body = strings.ReplaceAll(body, "Erase.apply((IOBiFunction)(var1),l0,l1)", "Erase.apply((IOBiFunction)(var1),(T)(l0),(T)(l1))")
		body = strings.ReplaceAll(body, "Erase.apply((IOBiFunction)(var2),l0,l1)", "Erase.apply((IOBiFunction)(var2),(T)(l0),(T)(l1))")
		body = strings.ReplaceAll(body, "Erase.apply((IOBiFunction)(var3),l0,l1)", "Erase.apply((IOBiFunction)(var3),(U)(l0),(U)(l1))")
		body = strings.ReplaceAll(body, ".accept(l0);", ".accept((T)(l0));")
	}
	body = strings.ReplaceAll(body, "Erase.accept(toIOConsumer(var1),l0)", "Erase.accept(toIOConsumer(var1),(T)(l0))")
	// Uncheck.accept/apply: primitive lambda param vs boxed value.
	body = strings.ReplaceAll(body, "Uncheck.accept((int l0) -> {\n\t\t\tsuper.mark(l0);\n\t\t},Integer.valueOf(var1))",
		"Uncheck.accept((Integer l0) -> {\n\t\t\tsuper.mark(l0.intValue());\n\t\t},Integer.valueOf(var1))")
	body = strings.ReplaceAll(body, "Uncheck.apply((long l0) -> {\n\t\t\treturn Long.valueOf(super.skip(l0));\n\t\t},Long.valueOf(var1))",
		"Uncheck.apply((Long l0) -> {\n\t\t\treturn Long.valueOf(super.skip(l0.longValue()));\n\t\t},Long.valueOf(var1))")
	body = strings.ReplaceAll(body, "Uncheck.apply((char l0) -> {\n\t\t\treturn Character.valueOf(super.write(l0));",
		"Uncheck.apply((Character l0) -> {\n\t\t\treturn Character.valueOf(super.write(l0.charValue()));")
	body = strings.ReplaceAll(body, "Uncheck.accept((int l0) -> {\n\t\t\tsuper.write(l0);\n\t\t},Integer.valueOf(var1))",
		"Uncheck.accept((Integer l0) -> {\n\t\t\tsuper.write(l0.intValue());\n\t\t},Integer.valueOf(var1))")
	body = strings.ReplaceAll(body, "Uncheck.apply((char l0) -> {\n\t\t\treturn super.append(l0);\n\t\t},Character.valueOf(var1))",
		"Uncheck.apply((Character l0) -> {\n\t\t\treturn super.append(l0.charValue());\n\t\t},Character.valueOf(var1))")
	body = strings.Replace(body,
		"return toList(var4.stream().map(Path::toFile));",
		"return toList(var4.stream().map((l0) -> ((Path)(l0)).toFile()));",
		1)
	body = strings.ReplaceAll(body,
		"return (R) (Stream.empty().collect(var2));",
		"return (R) (Stream.<Path>empty().collect(var2));")
	if strings.Contains(body, "class FileFilterUtils") {
		body = strings.ReplaceAll(body,
			"return (R) (Stream.<Path>empty().collect(var2));",
			"return (R) (Stream.<File>empty().collect(var2));")
	}
	body = strings.Replace(body,
		"return Uncheck.apply((IOBiFunction)(this),l0,l1);",
		"return (T) (Uncheck.apply((IOBiFunction)(this),l0,l1));",
		1)
	body = strings.Replace(body,
		"return Uncheck.apply((IOFunction)(this),l0);",
		"return (T) (Uncheck.apply((IOFunction)(this),l0));",
		1)
	body = strings.Replace(body,
		"Uncheck.accept(var2::forEachRemaining,var1::accept);",
		"Uncheck.accept((l0) -> var2.forEachRemaining(l0),var1);",
		1)
	body = strings.Replace(body,
		"return ((Boolean)(Uncheck.apply(var2::tryAdvance,var1::accept))).booleanValue();",
		"return ((Boolean)(Uncheck.apply((l0) -> Boolean.valueOf(var2.tryAdvance(l0)),var1))).booleanValue();",
		1)
	body = strings.Replace(body,
		"Uncheck.accept((l0) -> var2.forEachRemaining(l0),var1);",
		"Uncheck.run(() -> var2.forEachRemaining((l0) -> var1.accept(l0)));",
		1)
	body = strings.Replace(body,
		"Uncheck.accept(var2::forEachRemaining,(IOConsumer)(var1::accept));",
		"Uncheck.run(() -> var2.forEachRemaining((l0) -> var1.accept(l0)));",
		1)
	body = strings.Replace(body,
		"return ((Boolean)(Uncheck.apply((l0) -> Boolean.valueOf(var2.tryAdvance(l0)),var1))).booleanValue();",
		"return ((Boolean)(Uncheck.get(() -> Boolean.valueOf(var2.tryAdvance((l0) -> var1.accept(l0)))))).booleanValue();",
		1)
	body = strings.Replace(body,
		"return ((Boolean)(Uncheck.apply(var2::tryAdvance,(IOConsumer)(var1::accept)))).booleanValue();",
		"return ((Boolean)(Uncheck.get(() -> Boolean.valueOf(var2.tryAdvance((l0) -> var1.accept(l0)))))).booleanValue();",
		1)
	body = strings.ReplaceAll(body,
		"var2.forEachRemaining((l0) -> var1.accept(l0))",
		"var2.forEachRemaining((l0) -> var1.accept((T)(l0)))")
	body = strings.ReplaceAll(body,
		"var2.tryAdvance((l0) -> var1.accept(l0))",
		"var2.tryAdvance((l0) -> var1.accept((T)(l0)))")
	body = strings.Replace(body,
		"public MessageDigestCalculatingInputStream$Builder() {\nthis.messageDigest = MessageDigestCalculatingInputStream.getDefaultMessageDigest();\n}",
		"public MessageDigestCalculatingInputStream$Builder() {\ntry{\nthis.messageDigest = MessageDigestCalculatingInputStream.getDefaultMessageDigest();\n}catch(NoSuchAlgorithmException var1){\nthrow new IllegalStateException(var1);\n}\n}",
		1)
	body = strings.Replace(body,
		"public MessageDigestCalculatingInputStream$Builder() {\n\t\tthis.messageDigest = MessageDigestCalculatingInputStream.getDefaultMessageDigest();\n\t}",
		"public MessageDigestCalculatingInputStream$Builder() {\n\t\ttry{\n\t\t\tthis.messageDigest = MessageDigestCalculatingInputStream.getDefaultMessageDigest();\n\t\t}catch(NoSuchAlgorithmException var1){\n\t\t\tthrow new IllegalStateException(var1);\n\t\t}\n\t}",
		1)
	body = strings.Replace(body,
		"\t\t}\n\t}\n\tprivate static boolean contentEquals(Iterator<?> var0, Iterator<?> var1) {",
		"\t\t}\n\t\treturn false;\n\t}\n\tprivate static boolean contentEquals(Iterator<?> var0, Iterator<?> var1) {",
		1)
	body = strings.Replace(body,
		"\t\t}\n\t}\n\tprivate static boolean contentEquals(Stream<?> var0, Stream<?> var1) {",
		"\t\t}\n\t\treturn false;\n\t}\n\tprivate static boolean contentEquals(Stream<?> var0, Stream<?> var1) {",
		1)
	return body
}

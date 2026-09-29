package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// TestApplyLineBracesSkipsStringBraces pins the string-aware brace walker. A `{` inside
// `new StringBuilder("{")` must not change depth; the kill-switch restores naive counting.
func TestApplyLineBracesSkipsStringBraces(t *testing.T) {
	os.Unsetenv("JDEC_BRACE_SKIP_STRINGS_OFF")
	ln := "\t\t\tStringBuilder var2 = new StringBuilder(\"{\");"
	d, closed := applyLineBraces(ln, 2)
	if closed || d != 2 {
		t.Fatalf("fix ON: string `{` must not count, depth=%d closed=%v", d, closed)
	}
	d, closed = applyLineBraces("\t}", 1)
	if !closed || d != 0 {
		t.Fatalf("real close brace should close, depth=%d closed=%v", d, closed)
	}

	t.Setenv("JDEC_BRACE_SKIP_STRINGS_OFF", "1")
	d, closed = applyLineBraces(ln, 2)
	if closed || d != 3 {
		t.Fatalf("fix OFF: string `{` should count, depth=%d closed=%v", d, closed)
	}
}

// TestMethodRangeStringBraceIsLoadBearing pins applyLineBraces on the real decompile path.
// Spring SynthesizedMergedAnnotationInvocationHandler.toString reconstructs
// `new StringBuilder("{")`; without string-aware counting the method range swallows
// getAttributeValue and getName, so a local from another method is treated as the declaration
// of getAttributeValue's captured Method parameter (an invalid non-Method final copy). Kill-switch:
// JDEC_BRACE_SKIP_STRINGS_OFF.
func TestMethodRangeStringBraceIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringSynthesizedMergedAnnotationInvocationHandler.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_BRACE_SKIP_STRINGS_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if methodParamCopiedAsWrongType(on) {
		t.Errorf("fix ON: Method param must not be copied with another method's type before getReturnType, got:\n%s", on)
	}

	t.Setenv("JDEC_BRACE_SKIP_STRINGS_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !methodParamCopiedAsWrongType(off) {
		t.Errorf("fix OFF: expected a wrongly typed final copy of the Method param used with getReturnType, got:\n%s", off)
	}
}

// methodParamCopiedAsWrongType reports the false-capture shape: a final copy
// whose source is a Method-typed local, whose declared type is not Method, and
// which is subsequently used as the receiver of Method.getReturnType().
func methodParamCopiedAsWrongType(src string) bool {
	getReturn := strings.Index(src, ".getReturnType()")
	if getReturn < 0 {
		return false
	}
	lines := strings.Split(src[:getReturn], "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "final ") || !strings.Contains(line, " = ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		captureName := parts[len(parts)-3]
		if strings.HasSuffix(src[:getReturn], captureName) && parts[1] != "Method" {
			return true
		}
	}
	return false
}

// TestLambdaCaptureCopiesStayInsideGenericMethod pins method-range isolation for
// adjacent generic methods.  Local slot names repeat between methods; a capture
// copy must therefore derive its type only from the declaration in the method
// containing that lambda.  Letting a range bleed into the next method can turn
// `U var5` into `final int var5_f = var5`, which is both uncompilable and loses
// the generic contract carried by the original invokedynamic capture.
func TestLambdaCaptureCopiesStayInsideGenericMethod(t *testing.T) {
	t.Setenv("JDEC_LAMBDA_LOOP_CAPTURE_COPY_OFF", "")
	in := "class C {\n" +
		"\tpublic static <A> boolean warmup(P<A, A, RuntimeException> var0, A var1) {\n" +
		"\t\tP<A, A, RuntimeException> var2 = var0;\n" +
		"\t\tA var3 = var1;\n" +
		"\t\tA var4 = var1;\n" +
		"\t\tA var5 = var1;\n" +
		"\t\tA var6 = var1;\n" +
		"\t\tA var7 = var1;\n" +
		"\t\tA var8 = var1;\n" +
		"\t\tA var9 = var1;\n" +
		"\t\tA var10 = var1;\n" +
		"\t\tA var11 = var1;\n" +
		"\t\tA var12 = var1;\n" +
		"\t\tS var13 = () -> {\n" +
		"\t\t\treturn var2.test(var3,var4) && var2.test(var5,var6) && var2.test(var7,var8) && var2.test(var9,var10) && var2.test(var11,var12);\n" +
		"\t\t};\n" +
		"\t\treturn use(var13);\n" +
		"\t}\n" +
		"\tpublic static <T, U, E extends Throwable> boolean test(P<T, U, E> var0, T var1, U var2) {\n" +
		"\t\tP<T, U, E> var3 = var0;\n" +
		"\t\tT var4 = var1;\n" +
		"\t\tU var5 = var2;\n" +
		"\t\tS var6 = () -> {\n" +
		"\t\t\treturn var3.test(var4,var5);\n" +
		"\t\t};\n" +
		"\t\treturn use(var6);\n" +
		"\t}\n" +
		"\tpublic static void later() {\n" +
		"\t\tFailableConsumer var4 = null;\n" +
		"\t\tFailableConsumer<Throwable, ? extends Throwable> var3 = null;\n" +
		"\t\tint var5 = 0;\n" +
		"\t}\n" +
		"}\n"
	out := fixLambdaLoopCapture(in)
	for _, want := range []string{
		"final P<T, U, E> var3_f",
		"final T var4_f",
		"final U var5_f",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("capture type crossed a generic method boundary; missing %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{
		"final FailableConsumer var4_f",
		"final FailableConsumer<Throwable, ? extends Throwable> var3_f",
		"final int var5_f",
	} {
		if strings.Contains(out, bad) {
			t.Fatalf("capture type leaked from the next method: found %q:\n%s", bad, out)
		}
	}
}

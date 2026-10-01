package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

func TestAdversarialLambdaCaptureUsesLexicalDeclaration(t *testing.T) {
	types := []string{"Payload", "long", "List<String>", "byte[]"}
	for i, first := range types {
		second := types[(i+1)%len(types)]
		in := "class C {\n\tvoid run(boolean var0) {\n\t\tif (var0) {\n" +
			"\t\t\t" + first + " var1 = first();\n\t\t\tRunnable var2 = () -> {\n\t\t\t\tleft(var1);\n\t\t\t};\n" +
			"\t\t} else {\n\t\t\t" + second + " var1 = second();\n" +
			"\t\t\tRunnable var2 = () -> {\n\t\t\t\tright(var1);\n\t\t\t};\n\t\t}\n\t}\n}\n"
		out := fixLambdaLoopCapture(in)
		for _, want := range []string{first, second} {
			pattern := `final ` + regexp.QuoteMeta(want) + ` var1_f\d+ = var1;`
			if !regexp.MustCompile(pattern).MatchString(out) {
				t.Fatalf("capture lost its visible declaration type %q:\n%s", want, out)
			}
		}
		// A declaration-keyed traversal must not depend on map iteration order.
		for n := 0; n < 12; n++ {
			if other := fixLambdaLoopCapture(in); other != out {
				t.Fatalf("capture rendering changed between identical inputs:\n%s\n%s", out, other)
			}
		}
	}
}

func TestAdversarialLambdaCaptureRejectsInvisibleDeclarations(t *testing.T) {
	for _, declaration := range []string{
		"\t\tPayload var1 = later();\n",
		"\t\tif (flag()) {\n\t\t\tPayload var1 = earlier();\n\t\t}\n",
	} {
		lambda := "\t\tRunnable var2 = () -> {\n\t\t\tread(var1);\n\t\t};\n"
		for _, body := range []string{lambda + declaration, declaration + lambda} {
			in := "class C {\n\tvoid run() {\n" + body + "\t}\n}\n"
			if strings.HasPrefix(declaration, "\t\tPayload") && strings.HasPrefix(body, declaration) {
				continue // The declaration is visible in this one positive ordering.
			}
			if out := fixLambdaLoopCapture(in); out != in {
				t.Fatalf("an out-of-scope/future declaration manufactured a capture:\n%s", out)
			}
		}
	}
}

func TestAdversarialLambdaCaptureKeepsNestedBindingAndLiteral(t *testing.T) {
	in := "class C {\n\tvoid run() {\n\t\tPayload var1 = source();\n" +
		"\t\tRunnable var2 = () -> {\n\t\t\tString lv1_3 = \"var1 } -> {\";\n" +
		"\t\t\tlong lv1_4 = number();\n\t\t\tRunnable lv1_5 = () -> {\n" +
		"\t\t\t\tread(var1, lv1_4); // var1 }\n\t\t\t};\n\t\t};\n\t}\n}\n"
	out := fixLambdaLoopCapture(in)
	if !strings.Contains(out, `"var1 } -> {"`) || !strings.Contains(out, "// var1 }") {
		t.Fatalf("a literal/comment was rewritten as a captured variable:\n%s", out)
	}
	if got := strings.Count(out, " = var1;"); got != 1 {
		t.Fatalf("outer capture must bind once before the outer lambda, got %d:\n%s", got, out)
	}
	if !regexp.MustCompile(`final long lv1_4_f\d+ = lv1_4;`).MatchString(out) {
		t.Fatalf("nested lambda lost its own enclosing local:\n%s", out)
	}
}

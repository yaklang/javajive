package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestClassLiteralExceptionDomainRequiresOriginalEvaluationPC(t *testing.T) {
	for _, subject := range []struct {
		name string
		typ  types.JavaType
	}{
		{"class", types.NewJavaClass("example.Subject")},
		{"class array", types.NewJavaArrayType(types.NewJavaClass("example.Subject"))},
		{"primitive array", types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))},
	} {
		t.Run(subject.name, func(t *testing.T) {
			for _, change := range []string{"inside", "exclusive end", "gap", "missing origin", "negative origin", "missing subject"} {
				t.Run(change, func(t *testing.T) {
					literal := values.NewJavaClassValue(subject.typ)
					literal.HasOriginPC, literal.OriginPC = true, 19
					covered := func(pc int) bool { return pc >= 10 && pc < 20 || pc >= 30 && pc < 40 }
					switch change {
					case "exclusive end":
						literal.OriginPC = 20
					case "gap":
						literal.OriginPC = 25
					case "missing origin":
						literal.HasOriginPC = false
					case "negative origin":
						literal.OriginPC = -1
					case "missing subject":
						literal.JavaType = nil
					}
					want := change == "inside"
					if got := finallyCoveredValue(literal, covered); got != want {
						t.Fatalf("class resolution domain=%t want=%t", got, want)
					}
					call := layerTestCall(15)
					call.Arguments = []values.JavaValue{literal}
					if got := finallyCoveredValue(call, covered); got != want {
						t.Fatalf("folded class resolution domain=%t want=%t", got, want)
					}
				})
			}
		})
	}
}

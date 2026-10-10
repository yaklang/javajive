package values

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestJDKInferenceFormalRequiresExactDeclaration(t *testing.T) {
	for key, mask := range staticJDKInferenceFormals {
		t.Run(key.owner+"."+key.name+key.descriptor, func(t *testing.T) {
			call := &FunctionCallExpression{ClassName: key.owner, FunctionName: key.name, Descriptor: key.descriptor, Kind: InvokeStatic}
			for i := -1; i <= 8; i++ {
				want := i >= 0 && i < 8 && mask&(1<<uint(i)) != 0
				if got := call.jdkStaticInferenceFormal(i); got != want {
					t.Fatalf("formal %d: got %v want %v", i, got, want)
				}
				if want {
					for _, arg := range []JavaValue{NewJavaLiteral("value", types.NewJavaClass("java.lang.String")), NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))} {
						if cast := call.witnessDescriptorArgCast(i, arg, &class_context.ClassContext{}); cast != "" {
							t.Fatalf("generic formal %d received inference-changing cast %q", i, cast)
						}
					}
				}
			}
			call.ClassName = strings.ReplaceAll(key.owner, ".", "/")
			if call.jdkStaticInferenceFormal(0) != (mask&1 != 0) {
				t.Fatal("internal owner name was not normalized")
			}
			for _, mutation := range []string{"owner", "name", "descriptor", "instance"} {
				other := *call
				switch mutation {
				case "owner":
					other.ClassName = "user.Collections"
				case "name":
					other.FunctionName += "Other"
				case "descriptor":
					other.Descriptor = "(Ljava/lang/Object;)Ljava/lang/String;"
				case "instance":
					other.Kind = InvokeVirtual
				}
				for i := 0; i < 2; i++ {
					if other.jdkStaticInferenceFormal(i) {
						t.Fatalf("%s mismatch must not inherit signature proof", mutation)
					}
				}
			}
		})
	}
}

func TestRequireNonNullKeepsMessageOverloadPin(t *testing.T) {
	call := &FunctionCallExpression{ClassName: "java.util.Objects", FunctionName: "requireNonNull", Descriptor: "(Ljava/lang/Object;Ljava/lang/String;)Ljava/lang/Object;", Kind: InvokeStatic}
	null := NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
	if got := call.witnessDescriptorArgCast(1, null, &class_context.ClassContext{}); got != "String" {
		t.Fatalf("null message must still select String rather than Supplier: %q", got)
	}
}

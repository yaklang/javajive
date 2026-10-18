package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestOriginalLambdaParameterTargetKeepsPhysicalFormals(t *testing.T) {
	for _, variant := range []string{"original", "ordinary source", "captures", "wrong handle kind", "changed inputs", "changed result", "marker target", "unknown interface", "missing context", "missing bootstrap", "primitive interface"} {
		t.Run(variant, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "fixture.Owner"}
			ctx.SiblingClassSig = func(owner string) (string, map[string]string, bool) {
				return "", nil, owner == "fixture/IntAction"
			}
			d := &Decompiler{FunctionContext: ctx, InvokeDynamicName: "apply"}
			raw := types.NewJavaClass("java.util.function.Function")
			descriptor := "(Ljava/lang/Object;)Ljava/lang/Object;"
			methodType := func(descriptor string) values.JavaValue {
				return values.NewCustomValue(func(*class_context.ClassContext) string { return descriptor }, func() types.JavaType { return types.NewJavaClass("java.lang.invoke.MethodType") })
			}
			impl := t19Impl("fixture.Owner", "lambda$factory$0", descriptor, RefInvokeStatic)
			static := []values.JavaValue{methodType(descriptor), impl, methodType(descriptor)}
			value := values.NewCustomValue(func(*class_context.ClassContext) string { return "(final Object item) -> {return item;}" }, func() types.JavaType { return raw })
			source := LambdaBodySource{Code: value.String(ctx), FixedParameterTypes: true}
			captures := 0
			var actual types.JavaType = raw
			switch variant {
			case "ordinary source":
				source.FixedParameterTypes = false
			case "captures":
				captures = 1
			case "wrong handle kind":
				impl.RefKind = RefInvokeVirtual
			case "changed inputs":
				static[2] = methodType("(Ljava/lang/String;)Ljava/lang/Object;")
			case "changed result":
				static[2] = methodType("(Ljava/lang/Object;)Ljava/lang/String;")
			case "marker target":
				d.blockPartialFunctionalTarget = true
			case "unknown interface":
				actual = types.NewJavaClass("fixture.Unknown")
			case "missing context":
				d.FunctionContext = nil
			case "missing bootstrap":
				static = static[:2]
			case "primitive interface":
				actual = types.NewJavaPrimer(types.JavaInteger)
			}
			result, err := retainOriginalLambdaParameterTarget(d, value, source, actual, impl, static, captures)
			want := variant == "original" || variant == "ordinary source"
			if (err == nil && result != nil) != want {
				t.Fatalf("result=%v err=%v want=%v", result != nil, err, want)
			}
			if !want && !errors.Is(err, errFixedLambdaParameterTarget) {
				t.Fatal("source representability failure misclassified as malformed input", err)
			}
			if variant == "ordinary source" && result != value {
				t.Fatal("unconstrained body source acquired a fixed type view")
			}
			if variant == "original" {
				if name, known := types.RawClassFQN(result.Type()); !known || name != "java.util.function.Function" {
					t.Fatal("result erasure changed")
				}
				text := result.String(ctx)
				if !strings.Contains(text, "Function<Object, Object>") || !strings.Contains(text, "final Object item") {
					t.Fatal("fixed physical target/formal lost", text)
				}
			}
		})
	}
}

func TestOriginalLambdaRawParameterTargetRequiresDeclaredErasure(t *testing.T) {
	for _, variant := range []string{"original", "unknown", "wrong owner", "incomplete members", "incomplete parents", "class owner", "inaccessible", "wrong descriptor", "wrong name", "static", "bridge", "generic method", "duplicate"} {
		t.Run(variant, func(t *testing.T) {
			metadata := callbinding.Class{Name: "fixture/IntAction", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: "(I)I", Public: true}}}
			known := true
			switch variant {
			case "unknown":
				known = false
			case "wrong owner":
				metadata.Name = "fixture/Other"
			case "incomplete members":
				metadata.MembersComplete = false
			case "incomplete parents":
				metadata.ParentsComplete = false
			case "class owner":
				metadata.IsInterface = false
			case "inaccessible":
				metadata.Public = false
			case "wrong descriptor":
				metadata.Methods[0].Desc = "(J)I"
			case "wrong name":
				metadata.Methods[0].Name = "other"
			case "static":
				metadata.Methods[0].Static = true
			case "bridge":
				metadata.Methods[0].Bridge = true
			case "generic method":
				metadata.Methods[0].Signature = "<T:Ljava/lang/Object;>(I)I"
			case "duplicate":
				metadata.Methods = append(metadata.Methods, metadata.Methods[0])
			}
			ctx := &class_context.ClassContext{InvocationMetadata: func(string) (callbinding.Class, bool) { return metadata, known }}
			d := &Decompiler{FunctionContext: ctx, InvokeDynamicName: "read"}
			if got := fixedRawLambdaParameterTarget(d, "fixture.IntAction", "(I)I", "(I)I"); got != (variant == "original") {
				t.Fatalf("declared raw target=%v for %s", got, variant)
			}
			if fixedRawLambdaParameterTarget(d, "fixture.IntAction", "(J)I", "(I)I") {
				t.Fatal("raw declaration changed the original implementation inputs")
			}
		})
	}
}

func TestLambdaBodySourceConstraintRequiresActualQualifiedCallback(t *testing.T) {
	for _, declaration := range []LambdaOriginalDeclaration{LambdaDeclarationSyntheticBody, LambdaDeclarationOrdinary, LambdaDeclarationUnknown} {
		d := &Decompiler{FunctionContext: &class_context.ClassContext{ClassName: "Owner"}}
		d.OriginalLambdaDeclaration = func(string, string, string) LambdaOriginalDeclaration { return declaration }
		impl := t19Impl("Owner", "lambda$factory$0", "(I)I", RefInvokeStatic)
		actual, legacy := 0, 0
		d.DumpClassLambdaMethod = func(string, string, *utils.VariableId, []values.JavaValue) (string, error) {
			legacy++
			return "legacy", nil
		}
		d.DumpClassLambdaSourceAtOrigin = func(name, descriptor string, id *utils.VariableId, captures []values.JavaValue, adapter *LambdaReferenceAdapter, pc int) (LambdaBodySource, error) {
			actual++
			if pc != 0 || name != impl.Member || descriptor != impl.Description || len(captures) != 0 {
				t.Fatal("callback origin/implementation changed")
			}
			return LambdaBodySource{Code: "(final int value) -> {return value;}", FixedParameterTypes: true}, nil
		}
		source, err := dumpLambdaSourceWithReferenceAdapter(d, impl, nil, t19IntOpType(), nil, 0)
		if declaration != LambdaDeclarationSyntheticBody {
			if err == nil || actual != 0 || legacy != 0 {
				t.Fatal("source constraint borrowed an unqualified implementation")
			}
			continue
		}
		if err != nil || !source.FixedParameterTypes || actual != 1 || legacy != 0 {
			t.Fatal("completed callback constraint lost", source, err)
		}
		withoutOrigin, err := dumpLambdaSourceWithReferenceAdapter(d, impl, nil, t19IntOpType(), nil)
		if err != nil || withoutOrigin.FixedParameterTypes || actual != 1 || legacy != 1 {
			t.Fatal("legacy callback fabricated a source constraint")
		}
	}
}

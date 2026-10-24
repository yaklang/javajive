package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"strings"
	"testing"
)

func TestGenericFieldOverloadPreservesOriginalDescriptorOnlyWithBindingEvidence(t *testing.T) {
	for _, scenario := range []string{"inherited", "parameterized local", "field chain disabled", "inherited signature disabled", "wrong symbolic receiver", "wrong current declaration", "negative invocation PC", "oversized invocation PC", "no witness", "wrong origin", "wrong field", "wrong field type", "wrong descriptor", "wrong receiver", "custom receiver", "stack receiver", "missing signature", "wrong bound", "raw generic", "free variable", "shadow", "no invocation metadata", "unique", "generic selected", "generic rival", "varargs selected", "varargs rival", "bridge selected", "bridge rival", "selected missing", "different invocation descriptor", "special invocation", "dynamic invocation", "constructor", "missing invocation origin", "unexcluded rival", "unrelated rival"} {
		t.Run(scenario, func(t *testing.T) {
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("sample.Child"))
			receiver.IsThis = true
			field := NewRefMember(receiver, "value", types.NewJavaClass("java.lang.Object"))
			field.HasOriginPC = true
			field.OriginPC = 5
			witness := &JavaClassMember{Name: "sample/Child", Member: "value", Description: "Ljava/lang/Object;"}
			field.MarkOriginalFieldRead(witness, 5)
			call := &FunctionCallExpression{ClassName: "sample.Calls", FunctionName: "read", Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;", Kind: InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 8, Arguments: []JavaValue{field}}
			ctx := &class_context.ClassContext{ClassName: "sample.Child", ClassSig: "Lsample/Box<Ljava/lang/String;>;"}
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				if scenario == "missing signature" {
					return "", nil, false
				}
				if n == "sample/Child" {
					return "Lsample/Box<Ljava/lang/String;>;", nil, true
				}
				sig := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				if scenario == "wrong bound" {
					sig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				}
				return sig, nil, n == "sample/Box"
			}
			ctx.SiblingFieldSig = func(n, f string) (string, bool) {
				if n == "sample/Child" && scenario == "shadow" {
					return "Ljava/lang/Number;", true
				}
				sig := "TT;"
				if scenario == "free variable" {
					sig = "TU;"
				}
				return sig, n == "sample/Box" && f == "value"
			}
			methods := []callbinding.Method{{Name: "read", Desc: call.Descriptor, Static: true, Public: true}, {Name: "read", Desc: "(Ljava/lang/String;)Ljava/lang/Object;", Static: true, Public: true}}
			switch scenario {
			case "parameterized local":
				receiver.IsThis = false
				field.originalFieldRead.owner = "sample/Box"
				receiver.ResetVarType(types.NewParameterizedType("sample.Box", []types.JavaType{types.NewJavaClass("java.lang.String")}))
			case "field chain disabled":
				ctx.Env = func(k string) string {
					if k == "JDEC_GENERIC_FIELD_CHAIN_OFF" {
						return "1"
					}
					return ""
				}
			case "inherited signature disabled":
				ctx.Env = func(k string) string {
					if k == "JDEC_INHERITED_FIELD_SIG_OFF" {
						return "1"
					}
					return ""
				}
			case "wrong symbolic receiver":
				field.originalFieldRead.owner = "sample/Other"
			case "wrong current declaration":
				ctx.ClassSig = "Ljava/lang/Object;"
			case "negative invocation PC":
				call.OriginPC = -1
			case "oversized invocation PC":
				call.OriginPC = 65536
			case "no witness":
				field.originalFieldRead = nil
			case "wrong origin":
				field.OriginPC++
			case "wrong field":
				field.Member = "other"
			case "wrong field type":
				field.JavaType = types.NewJavaClass("java.lang.String")
			case "wrong descriptor":
				field.originalFieldRead.descriptor = "Ljava/lang/Number;"
			case "wrong receiver":
				receiver.ResetVarType(types.NewJavaClass("sample.Other"))
			case "custom receiver":
				receiver.CustomValue = &CustomValue{}
			case "stack receiver":
				receiver.StackVar = NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			case "raw generic":
				receiver.IsThis = false
				receiver.ResetVarType(types.NewJavaClass("sample.Box"))
			case "unique":
				methods = methods[:1]
			case "generic selected":
				methods[0].Generic = true
			case "generic rival":
				methods[1].Generic = true
			case "varargs selected":
				methods[0].Varargs = true
			case "varargs rival":
				methods[1].Varargs = true
			case "bridge selected":
				methods[0].Bridge = true
			case "bridge rival":
				methods[1].Bridge = true
			case "selected missing":
				methods = methods[1:]
			case "different invocation descriptor":
				call.Descriptor = "(Ljava/lang/String;)Ljava/lang/Object;"
			case "special invocation":
				call.IsSpecialInvoke = true
				call.Kind = InvokeSpecial
			case "dynamic invocation":
				call.Kind = InvokeDynamic
			case "constructor":
				call.FunctionName = "<init>"
			case "missing invocation origin":
				call.HasOriginPC = false
			case "unexcluded rival":
				methods = append(methods, callbinding.Method{Name: "read", Desc: "(Ljava/lang/Object;)Ljava/lang/String;", Static: true, Public: true})
			case "unrelated rival":
				methods[1].Desc = "(Ljava/lang/Number;)Ljava/lang/Object;"
			}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
				if n == "java/lang/Object" || n == "java/lang/String" {
					return callbinding.Class{Name: n, MembersComplete: true, ParentsComplete: true, Public: true}, true
				}
				return callbinding.Class{Name: n, MembersComplete: true, ParentsComplete: true, Public: true, Methods: methods}, n == "sample/Calls"
			}
			if scenario == "no invocation metadata" {
				ctx.InvocationMetadata = nil
			}
			want := ""
			if scenario == "inherited" || scenario == "parameterized local" {
				want = "Object"
			}
			if got := call.instantiatedFieldOverloadCast(0, ctx); got != want {
				t.Fatalf("cast=%q want %q", got, want)
			}
			if want != "" {
				source := call.ArgumentStrings(ctx)[0]
				if !strings.Contains(source, "(Object)") || strings.Count(source, ".value") != 1 {
					t.Fatalf("descriptor view/evaluation %s", source)
				}
			}
			if call.Arguments[0] != field || field.Object != receiver || (scenario != "negative invocation PC" && scenario != "oversized invocation PC" && call.OriginPC != 8) {
				t.Fatal("rewrote value or evaluation origin")
			}
		})
	}
}

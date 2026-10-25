package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestConcreteInputOfGenericResultKeepsOverloadPin(t *testing.T) {
	for _, variant := range []string{"parameterized argument", "raw argument", "unique", "method variable", "substituted formal"} {
		t.Run(variant, func(t *testing.T) {
			desc := "(Lproof/View;)Ljava/lang/Object;"
			signature := "<R:Ljava/lang/Object;>(Lproof/View;)TR;"
			classSig := "Ljava/lang/Object;"
			if variant == "method variable" {
				signature = "<R:Lproof/View;>(TR;)TR;"
			}
			if variant == "substituted formal" {
				classSig = "<E:Lproof/View;>Ljava/lang/Object;"
				signature = "(TE;)Ljava/lang/Object;"
			}
			ctx := &class_context.ClassContext{ClassName: "proof.Router", ClassSig: classSig, TypeParams: []string{"T"}}
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				return classSig, map[string]string{class_context.MethodDescKey("select", desc): signature}, n == "proof/Router"
			}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
				ms := []callbinding.Method{{Name: "select", Desc: desc, Signature: signature, Generic: true, Public: true}}
				if variant != "unique" {
					ms = append(ms, callbinding.Method{Name: "select", Desc: "(Lproof/Token;)Ljava/lang/Object;", Public: true})
				}
				return callbinding.Class{Name: n, Public: true, MembersComplete: true, ParentsComplete: true, Methods: ms}, n == "proof/Router"
			}
			receiver := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Router"))
			receiver.IsThis = true
			argType := types.NewParameterizedType("proof.Token", []types.JavaType{types.NewJavaTypeVariable("T")})
			if variant == "raw argument" {
				argType = types.NewJavaClass("proof.Token")
			}
			arg := NewJavaRef(utils.NewRootVariableId(), nil, argType)
			arg.Id.SetName("token")
			mt, _ := types.ParseMethodDescriptor(desc)
			call := &FunctionCallExpression{Object: receiver, ClassName: "proof.Router", FunctionName: "select", Descriptor: desc, FuncType: mt.FunctionType(), Kind: InvokeVirtual, Arguments: []JavaValue{arg}}
			got := call.witnessDescriptorArgCast(0, arg, ctx)
			want := ""
			if variant == "parameterized argument" || variant == "raw argument" {
				want = "View"
			}
			if got != want {
				t.Fatalf("descriptor pin=%q want=%q", got, want)
			}
			if call.Arguments[0] != arg || arg.Type() != argType {
				t.Fatal("binding mutated evaluated operand")
			}
		})
	}
}

func TestConcreteDescriptorFormalDoesNotEraseSourceConstraints(t *testing.T) {
	concrete := types.NewJavaClass("proof.View")
	for _, sig := range []string{"Tproof.View;", "Lproof/View<*>;", "[Lproof/View;", "Ljava/lang/Object;", "Lproof/Other;"} {
		if sameConcreteDescriptorFormal(types.ParseSignature(sig), concrete) {
			t.Fatalf("lost source constraint %s", sig)
		}
	}
	if sameConcreteDescriptorFormal(nil, concrete) || sameConcreteDescriptorFormal(concrete, nil) {
		t.Fatal("missing evidence")
	}
}

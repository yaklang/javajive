package statements

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestGenericLocalConsumerSealsInvocationAfterDeclarationRecovery(t *testing.T) {
	for _, scenario := range []string{"recovered formal", "whole web formal", "erased consumer", "reassignment", "original checked argument"} {
		t.Run(scenario, func(t *testing.T) {
			desc := "(Ljava/lang/String;Ljava/lang/Class;)Ljava/lang/Object;"
			sig := "<T:Ljava/lang/Object;>(Ljava/lang/String;Ljava/lang/Class<TT;>;)TT;"
			meta := map[string]callbinding.Class{
				"proof/Owner":      {Name: "proof/Owner", Public: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "read", Desc: desc, Generic: true, Signature: sig}, {Name: "read", Desc: "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;"}}},
				"java/lang/String": {Name: "java/lang/String", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}},
				"java/lang/Class":  {Name: "java/lang/Class", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}},
				"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
			}
			ctx := &class_context.ClassContext{ClassName: "proof.Owner", ClassSig: "<E:Ljava/lang/Object;>Ljava/lang/Object;", ClassTypeParams: []string{"E"}, TypeParams: []string{"E", "T"}, CurrentMethodSig: sig}
			ctx.InvocationMetadata = func(n string) (callbinding.Class, bool) { c, ok := meta[strings.ReplaceAll(n, ".", "/")]; return c, ok }
			ctx.SiblingClassSig = func(n string) (string, map[string]string, bool) {
				return ctx.ClassSig, map[string]string{class_context.MethodDescKey("read", desc): sig, class_context.MethodSigKey("read", 2): sig}, strings.ReplaceAll(n, ".", "/") == "proof/Owner"
			}
			self := ternaryCastTestRef("this", "proof.Owner")
			self.IsThis = true
			classArg := ternaryCastTestRef("kind", "java.lang.Class")
			classArg.ResetVarType(types.NewParameterizedType("java.lang.Class", []types.JavaType{types.NewJavaClass("T")}))
			key := values.JavaValue(ternaryCastTestRef("key", "java.lang.String"))
			if scenario == "original checked argument" {
				key = &values.CastExpression{Value: ternaryCastTestRef("key", "java.lang.Object"), TargetType: types.NewJavaClass("java.lang.String"), OriginPC: 7}
			}
			ft, err := types.ParseMethodDescriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			call := &values.FunctionCallExpression{ClassName: "proof.Owner", FunctionName: "read", Descriptor: desc, Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: 19, Object: self, FuncType: ft.FunctionType(), Arguments: []values.JavaValue{key, classArg}}
			left := ternaryCastTestRef("result", "java.lang.Object")
			first := scenario != "reassignment"
			if scenario == "whole web formal" || !first {
				left.WebDeclType = types.NewJavaClass("T")
			}
			if scenario == "erased consumer" {
				left.WebDeclType = types.NewJavaClass("java.lang.Object")
			}
			statement := NewAssignStatement(left, call, first)
			witness := call.Witness()
			text := statement.String(ctx)
			if scenario == "erased consumer" {
				if !strings.HasPrefix(text, "Object result = ") || !strings.Contains(text, "(Class)(kind)") {
					t.Fatalf("proper erased consumer lost: %s", text)
				}
			} else {
				if first && !strings.HasPrefix(text, "T result = ") {
					t.Fatalf("generic consumer lost: %s", text)
				}
				if strings.Contains(text, "(Class)(kind)") {
					t.Fatalf("raw argument erased the actual T consumer: %s", text)
				}
			}
			if scenario == "original checked argument" && !strings.Contains(text, "((String)(key))") {
				t.Fatalf("original operand CHECKCAST lost: %s", text)
			}
			if call.Witness() != witness || call.Object != self || call.Arguments[0] != key || call.Arguments[1] != classArg || left.Type().String(ctx) != "Object" {
				t.Fatal("rendering mutated physical invocation, operands or computational slot")
			}
		})
	}
}

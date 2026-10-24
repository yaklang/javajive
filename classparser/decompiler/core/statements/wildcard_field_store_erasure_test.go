package statements

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestWildcardFieldStoreErasureRequiresOriginalStoreAndExactResult(t *testing.T) {
	for _, variant := range []string{"original", "virtual", "interface", "unicode field", "shadowed simple type", "synthetic store", "missing store PC", "different store PC", "replaced target", "replaced receiver", "replaced RHS", "different supplied RHS", "foreign owner", "non-this receiver", "opaque receiver", "array store", "declaration", "missing signature", "wrong field signature", "raw field signature", "array signature", "long signature", "wrong field word", "missing invoke PC", "negative invoke PC", "oversized invoke PC", "dynamic", "special", "static mismatch", "missing receiver", "missing function type", "wrong descriptor result", "wrong result word", "bad descriptor", "argument count", "nil argument", "lambda", "conditional", "local value", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			const descriptor = "(Ljava/lang/Object;)Ljava/util/function/Function;"
			fieldName := "function"
			if variant == "unicode field" {
				fieldName = "函数"
			}
			ctx := &class_context.ClassContext{ClassName: "proof.Owner", PackageName: "proof", ClassSig: "<K:Ljava/lang/Object;V:Ljava/lang/Object;>Ljava/lang/Object;", TypeParams: []string{"K", "V"}, ClassTypeParams: []string{"K", "V"}, FieldSignatures: map[string]string{fieldName: "Ljava/util/function/Function<Ljava/lang/Iterable<+TK;>;Ljava/util/Map<TK;TV;>;>;"}}
			typ := types.NewJavaClass("java.util.function.Function")
			self := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
			self.IsThis = true
			left := values.NewRefMember(self, fieldName, typ)
			renders := 0
			argument := values.NewCustomValue(func(*class_context.ClassContext) string { renders++; return "originalArgument()" }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
			call := &values.FunctionCallExpression{ClassName: "proof.Maker", FunctionName: "produce", Descriptor: descriptor, Kind: values.InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: 11, FuncType: &types.JavaFuncType{ReturnType: typ}, Arguments: []values.JavaValue{argument}}
			var rhs values.JavaValue = call
			if variant == "lambda" {
				rhs = values.NewCustomValue(func(*class_context.ClassContext) string { renders++; return "x -> x" }, func() types.JavaType { return typ })
			}
			if variant == "conditional" {
				rhs = values.NewTernaryExpression(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), call, call)
			}
			if variant == "local value" {
				rhs = values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			}
			a := NewAssignStatement(left, rhs, false)
			a.OriginPC = 29
			a.HasOriginPC = true
			member := values.NewJavaClassMember("proof.Owner", fieldName, "Ljava/util/function/Function;", typ)
			a.MarkOriginalInstanceFieldStore(member, 29)
			supplied := rhs
			switch variant {
			case "virtual", "interface":
				call.IsStatic = false
				call.Kind = values.InvokeVirtual
				if variant == "interface" {
					call.Kind = values.InvokeInterface
				}
				call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Maker"))
			case "shadowed simple type":
				ctx.TypeParams = append(ctx.TypeParams, "Function")
			case "synthetic store":
				a.originalInstanceStore = nil
			case "missing store PC":
				a.HasOriginPC = false
			case "different store PC":
				a.OriginPC++
			case "replaced target":
				a.LeftValue = values.NewRefMember(self, fieldName, typ)
			case "replaced receiver":
				left.Object = values.NewJavaRef(utils.NewRootVariableId().Next(), nil, self.Type())
			case "replaced RHS":
				a.JavaValue = call.Clone()
			case "different supplied RHS":
				supplied = call.Clone()
			case "foreign owner":
				ctx.ClassName = "proof.Other"
			case "non-this receiver":
				self.IsThis = false
			case "opaque receiver":
				self.CustomValue = argument
			case "array store":
				a.ArrayMember = values.NewJavaArrayMember(self, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
			case "declaration":
				a.IsDeclare = true
			case "missing signature":
				delete(ctx.FieldSignatures, fieldName)
			case "wrong field signature":
				ctx.FieldSignatures[fieldName] = "Ljava/util/List<TK;>;"
			case "raw field signature":
				ctx.FieldSignatures[fieldName] = "Ljava/util/function/Function;"
			case "array signature":
				ctx.FieldSignatures[fieldName] = "[Ljava/util/function/Function<TK;TV;>;"
			case "long signature":
				ctx.FieldSignatures[fieldName] = strings.Repeat("x", 4097)
			case "wrong field word":
				left = values.NewRefMember(self, fieldName, types.NewJavaClass("java.lang.Object"))
				a.LeftValue = left
				a.originalInstanceStore = nil
				a.MarkOriginalInstanceFieldStore(member, 29)
			case "missing invoke PC":
				call.HasOriginPC = false
			case "negative invoke PC":
				call.OriginPC = -1
			case "oversized invoke PC":
				call.OriginPC = 65536
			case "dynamic":
				call.Kind = values.InvokeDynamic
			case "special":
				call.Kind = values.InvokeSpecial
				call.IsSpecialInvoke = true
			case "static mismatch":
				call.IsStatic = false
			case "missing receiver":
				call.IsStatic = false
				call.Kind = values.InvokeVirtual
			case "missing function type":
				call.FuncType = nil
			case "wrong descriptor result":
				call.Descriptor = "(Ljava/lang/Object;)Ljava/lang/Object;"
			case "wrong result word":
				call.FuncType = &types.JavaFuncType{ReturnType: types.NewJavaClass("java.lang.Object")}
			case "bad descriptor":
				call.Descriptor = "not a descriptor"
			case "argument count":
				call.Arguments = nil
			case "nil argument":
				call.Arguments = []values.JavaValue{nil}
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(c, workbudget.Limits{})
			}
			want := ""
			if variant == "original" || variant == "virtual" || variant == "interface" || variant == "unicode field" {
				want = "Function"
			}
			if variant == "shadowed simple type" {
				want = "java.util.function.Function"
			}
			before := call.Witness()
			if got := a.wildcardFieldStoreErasure(ctx, supplied); got != want {
				t.Fatalf("erased store view=%q want=%q", got, want)
			}
			if renders != 0 || call.Witness() != before {
				t.Fatal("a type view rendered an operand or changed the original invocation")
			}
		})
	}
}

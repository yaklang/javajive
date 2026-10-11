package values

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

// JLS 18.2.3 permits a corresponding instantiated supertype to constrain a
// parameterized formal. A bare inference variable instead receives the actual
// anonymous subtype as a lower bound; its parent is not an equality witness.
func TestSourceAnonymousParentConstrainsParameterizedFormalsOnly(t *testing.T) {
	for _, variant := range []string{"parameterized class", "parameterized interface", "bare variable", "bounded bare variable"} {
		t.Run(variant, func(t *testing.T) {
			parent, header := "probe/Parent", "Lprobe/Parent<Ljava/lang/String;>;"
			param, bound := "Lprobe/Parent<TT;>;", "Ljava/lang/Object;"
			positive := variant == "parameterized class" || variant == "parameterized interface"
			if variant == "parameterized interface" {
				parent, header, param = "probe/Mapper", "Ljava/lang/Object;Lprobe/Mapper<Ljava/lang/String;>;", "Lprobe/Mapper<TT;>;"
			}
			if !positive {
				param = "TT;"
				parent = "java/lang/Object"
				if variant == "bounded bare variable" {
					parent, bound = "probe/Parent", "Lprobe/Parent;"
				}
			}
			desc := "(L" + parent + ";)Lprobe/Box;"
			sig := "<T:" + bound + ">(" + param + ")Lprobe/Box<TT;>;"
			metas := map[string]callbinding.Class{
				"probe/Factory": {Name: "probe/Factory", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "wrap", Desc: desc, Signature: sig, Generic: true, Static: true, Public: true}}},
				"probe/Parent":  {Name: "probe/Parent", MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;"},
				"probe/Mapper":  {Name: "probe/Mapper", MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;", IsInterface: true},
			}
			allocation := &NewExpression{JavaType: types.NewJavaClass("probe.Owner$1"), OriginPC: 3, HasOriginPC: true}
			constructor := &FunctionCallExpression{ClassName: "probe.Owner$1", FunctionName: "<init>", Descriptor: "()V", Object: allocation, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 7, HasOriginPC: true}
			allocation.ConstructorCall = constructor
			ctx := &class_context.ClassContext{ClassName: "probe.Owner", FunctionName: "make", CurrentMethodDesc: "()V", InvocationMetadata: func(n string) (callbinding.Class, bool) { m, ok := metas[n]; return m, ok }}
			ctx.SourceAnonymousParentSignature = func(owner, descriptor, sourceOwner, name, methodDesc string, newPC, pc int) (string, bool) {
				return header, owner == "probe.Owner$1" && descriptor == "()V" && sourceOwner == "probe.Owner" && name == "make" && methodDesc == "()V" && newPC == 3 && pc == 7
			}
			call := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "wrap", Descriptor: desc, IsStatic: true, Kind: InvokeStatic, OriginPC: 10, HasOriginPC: true, Arguments: []JavaValue{allocation}, FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("probe.Box")}}
			before := allocation.JavaType.Copy()
			got := call.SourceInvocationResultType(ctx)
			if (got != nil) != positive || positive && got.String(ctx) != "Box<String>" {
				t.Fatalf("%s: parent constraint returned %v, want positive=%v", variant, got, positive)
			}
			if call.Arguments[0] != allocation || allocation.ConstructorCall != constructor || !reflect.DeepEqual(before, allocation.JavaType) || call.FuncType.ReturnType.String(ctx) != "Box" {
				t.Fatal("constraint query changed the anonymous value or computational result")
			}
		})
	}
}

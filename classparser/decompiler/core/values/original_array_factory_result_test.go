package values

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func originalArrayFactoryResultFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class) {
	const descriptor = "([Ljava/lang/Object;)Ljava/util/List;"
	const signature = "<E:Ljava/lang/Object;>([TE;)Ljava/util/List<TE;>;"
	metadata := map[string]callbinding.Class{
		"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
		"java/lang/String": {Name: "java/lang/String", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true},
		"probe/Factory": {Name: "probe/Factory", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true,
			Methods: []callbinding.Method{{Name: "pack", Desc: descriptor, Signature: signature, Static: true, Public: true, Generic: true, Varargs: true}}},
	}
	ctx := &class_context.ClassContext{
		InvocationMetadata: func(name string) (callbinding.Class, bool) { c, ok := metadata[name]; return c, ok },
		SiblingClassSig: func(name string) (string, map[string]string, bool) {
			c, ok := metadata[name]
			methods := map[string]string{}
			for _, m := range c.Methods {
				methods[class_context.MethodDescKey(m.Name, m.Desc)] = m.Signature
			}
			return c.Signature, methods, ok
		},
	}
	input := &FunctionCallExpression{ClassName: "probe.Source", FunctionName: "replace", Descriptor: "(Ljava/lang/Object;)Ljava/lang/Object;", IsStatic: true, Kind: InvokeStatic, OriginPC: 5, HasOriginPC: true}
	array, _ := types.ParseDescriptor("[Ljava/lang/Object;")
	typ, _ := types.ParseMethodDescriptor(descriptor)
	f := &FunctionCallExpression{ClassName: "probe.Factory", FunctionName: "pack", Descriptor: descriptor, IsStatic: true, Kind: InvokeStatic, OriginPC: 13, HasOriginPC: true,
		FuncType: typ.FunctionType(), Arguments: []JavaValue{NewOriginalCheckCast(input, array, 10)}}
	return f, ctx, metadata
}

func TestOriginalArrayFactoryResultKeepsExistingCheckAndOpaqueProducer(t *testing.T) {
	for _, descriptor := range []string{"[Ljava/lang/Object;", "[Ljava/lang/String;", "[[I"} {
		t.Run(descriptor, func(t *testing.T) {
			f, ctx, _ := originalArrayFactoryResultFixture()
			target, _ := types.ParseDescriptor(descriptor)
			producer := f.Arguments[0].(*CastExpression).Value
			f.Arguments[0] = NewOriginalCheckCast(producer, target, 10)
			before := f.Clone()
			if !ErasedFactoryReturn(ctx, f, "Ljava/util/List;") {
				t.Fatal("original fixed-array operand did not isolate return inference")
			}
			if !reflect.DeepEqual(before, f) || f.Arguments[0].(*CastExpression).Value != producer {
				t.Fatal("result-only proof changed shared invocation, check or producer")
			}
			if _, physical, known := f.Arguments[0].(*CastExpression).OriginalCheckCastWitness(ctx); !known || physical != descriptor {
				t.Fatal("original runtime array component was narrowed")
			}
		})
	}
	t.Run("platform declaration only", func(t *testing.T) {
		f, ctx, _ := originalArrayFactoryResultFixture()
		ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, false }
		if !ErasedFactoryReturn(ctx, f, "Ljava/util/List;") {
			t.Fatal("complete platform Signature was not available within the erasure proof")
		}
		if _, signatures, known := ctx.SiblingClassSig(f.ClassName); known || len(signatures) != 0 {
			t.Fatal("result proof published platform signatures to legacy argument inference")
		}
	})
}

func TestOriginalArrayFactoryResultRefusesUnprovedArrayAndBinding(t *testing.T) {
	for _, variant := range []string{"input array only", "synthetic cast", "tagged cast", "changed check target", "changed check PC", "check after invoke", "primitive array", "missing origin", "wrong kind", "instance", "special", "wrong descriptor", "different result", "changed type model", "unknown owner", "owner identity", "incomplete members", "incomplete parents", "interface owner", "nonpublic method", "nonstatic method", "bridge", "competing overload", "duplicate declaration", "empty signature", "trailing signature", "extra bound", "dependent bound", "foreign formal", "different formal", "fixed result", "signature map mismatch", "missing signature map", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, metadata := originalArrayFactoryResultFixture()
			owner := metadata["probe/Factory"]
			cast := f.Arguments[0].(*CastExpression)
			result := "Ljava/util/List;"
			switch variant {
			case "input array only":
				f.Arguments[0] = cast.Value
			case "synthetic cast":
				cast.Binding = true
			case "tagged cast":
				f.Arguments[0] = &CastExpression{Value: cast.Value, TargetType: cast.TargetType, OriginPC: cast.OriginPC, OriginalCheckCast: true}
			case "changed check target":
				cast.TargetType, _ = types.ParseDescriptor("[Ljava/lang/String;")
			case "changed check PC":
				cast.OriginPC = -1
			case "check after invoke":
				cast.OriginPC = f.OriginPC + 1
			case "primitive array":
				typ, _ := types.ParseDescriptor("[I")
				f.Arguments[0] = NewOriginalCheckCast(cast.Value, typ, 10)
			case "missing origin":
				f.HasOriginPC = false
			case "wrong kind":
				f.Kind = InvokeVirtual
			case "instance":
				f.IsStatic = false
			case "special":
				f.IsSpecialInvoke = true
			case "wrong descriptor":
				f.Descriptor = "(Ljava/lang/Object;)Ljava/util/List;"
			case "different result":
				result = "Ljava/util/Set;"
			case "changed type model":
				f.FuncType.ParamTypes[0], _ = types.ParseDescriptor("[Ljava/lang/String;")
			case "unknown owner":
				delete(metadata, "probe/Factory")
			case "owner identity":
				owner.Name = "probe/Other"
			case "incomplete members":
				owner.MembersComplete = false
			case "incomplete parents":
				owner.ParentsComplete = false
			case "interface owner":
				owner.IsInterface = true
			case "nonpublic method":
				owner.Methods[0].Public = false
			case "nonstatic method":
				owner.Methods[0].Static = false
			case "bridge":
				owner.Methods[0].Bridge = true
			case "competing overload":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: f.FunctionName, Desc: "([Ljava/lang/String;)Ljava/util/List;", Static: true, Public: true})
			case "duplicate declaration":
				owner.Methods = append(owner.Methods, owner.Methods[0])
			case "empty signature":
				owner.Methods[0].Signature = ""
			case "trailing signature":
				owner.Methods[0].Signature += "garbage"
			case "extra bound":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;:Ljava/io/Serializable;>([TE;)Ljava/util/List<TE;>;"
			case "dependent bound":
				owner.Methods[0].Signature = "<E:TF;F:Ljava/lang/Object;>([TE;)Ljava/util/List<TE;>;"
			case "foreign formal":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;>([TE;)Ljava/util/List<TF;>;"
			case "different formal":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;>([Ljava/lang/String;)Ljava/util/List<TE;>;"
			case "fixed result":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;>([TE;)Ljava/util/List<Ljava/lang/String;>;"
			case "signature map mismatch":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", map[string]string{}, true }
			case "missing signature map":
				ctx.SiblingClassSig = nil
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				cancel, stop := context.WithCancel(context.Background())
				stop()
				ctx.Work = workbudget.New(cancel, workbudget.Limits{})
			}
			if variant != "unknown owner" {
				metadata["probe/Factory"] = owner
			}
			if ErasedFactoryReturn(ctx, f, result) {
				t.Fatal("unproved array result inference was admitted")
			}
		})
	}
}

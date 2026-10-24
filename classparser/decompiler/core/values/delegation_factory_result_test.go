package values

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func delegationFactoryResultFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class) {
	factory, ctx, metadata := originalArrayFactoryResultFixture()
	factory.Object = NewJavaClassValue(types.NewJavaClass("probe.Factory"))
	producer := factory.Arguments[0].(*CastExpression).Value.(*FunctionCallExpression)
	producer.Object = NewJavaClassValue(types.NewJavaClass("probe.Source"))
	producerType, _ := types.ParseMethodDescriptor(producer.Descriptor)
	producer.FuncType, producer.Arguments = producerType.FunctionType(), []JavaValue{JavaNull}
	metadata["probe/Source"] = callbinding.Class{Name: "probe/Source", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: producer.FunctionName, Desc: producer.Descriptor, Public: true, Static: true}}}
	metadata["java/util/Collection"] = callbinding.Class{Name: "java/util/Collection", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}}
	metadata["java/util/List"] = callbinding.Class{Name: "java/util/List", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/util/Collection"}, Signature: "<E:Ljava/lang/Object;>Ljava/lang/Object;Ljava/util/Collection<TE;>;"}
	metadata["proof/Owner"] = callbinding.Class{Name: "proof/Owner", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{
		{Name: "<init>", Desc: "(ZLjava/util/List;)V", Signature: "(ZLjava/util/List<Ljava/lang/String;>;)V", Generic: true},
		{Name: "<init>", Desc: "([Ljava/lang/String;)V", Varargs: true},
		{Name: "<init>", Desc: "(Z[Ljava/lang/String;)V", Varargs: true},
	}}
	ctx.ClassName, ctx.FunctionName, ctx.CurrentMethodDesc = "proof.Owner", "<init>", "([Ljava/lang/String;)V"
	self := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
	self.IsThis, self.IsParam = true, true
	self.MarkOriginalParameter(0)
	self.MarkOriginalReceiver()
	method, _ := types.ParseMethodDescriptor("(ZLjava/util/List;)V")
	call := &FunctionCallExpression{ClassName: "proof.Owner", FunctionName: "<init>", Descriptor: "(ZLjava/util/List;)V", FuncType: method.FunctionType(), Kind: InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: 20, Object: self,
		Arguments: []JavaValue{NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), factory}}
	return call, ctx, metadata
}

func TestDelegationFactoryResultViewBelongsToSelectedParameter(t *testing.T) {
	for _, variant := range []string{"different arity true", "different arity false", "same arity parameter", "renamed owner"} {
		t.Run(variant, func(t *testing.T) {
			call, ctx, metadata := delegationFactoryResultFixture()
			if variant == "different arity false" {
				call.Arguments[0] = NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))
			}
			if variant == "same arity parameter" {
				ctx.CurrentMethodDesc = "(Z[Ljava/lang/String;)V"
				flag := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
				flag.IsParam = true
				flag.MarkOriginalParameter(1)
				call.Arguments[0] = flag
			}
			if variant == "renamed owner" {
				owner := metadata["proof/Owner"]
				delete(metadata, owner.Name)
				owner.Name = "changed/Renamed"
				metadata[owner.Name] = owner
				call.ClassName, ctx.ClassName = "changed.Renamed", "changed.Renamed"
				call.Object.(*JavaRef).ResetVarType(types.NewJavaClass("changed.Renamed"))
			}
			before := call.Clone()
			target := call.delegationFactoryResultCast(1, call.Arguments[1], ctx)
			if target != "List<String>" {
				t.Fatal("selected Signature did not supply the same-erasure result view", target)
			}
			if !reflect.DeepEqual(before, call) {
				t.Fatal("consumer proof changed shared producer, checks or delegation")
			}
			if source := call.renderProvenArgumentCast(1, target, call.Arguments[1], ctx); !strings.Contains(source, "(List<String>)(List)") || strings.Count(source, "pack(") != 1 || !strings.Contains(source, "Object[]") {
				t.Fatal("result view changed evaluation or the original array check", source)
			}
		})
	}
}

func TestDelegationFactoryResultCannotBorrowAnotherSourceOrDeclaration(t *testing.T) {
	for _, variant := range []string{"ordinary invocation", "super invocation", "nonconstructor source", "missing PC", "unsealed receiver", "different receiver type", "unknown owner", "incomplete owner", "generic owner", "duplicate target", "static target", "bridge target", "varargs target", "method formal", "unknown concrete argument", "arity mismatch", "signature map mismatch", "applicable rival", "incomplete rival hierarchy", "noncanonical boolean", "wrong boolean width", "opaque boolean", "unproved producer", "source access denial", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			call, ctx, metadata := delegationFactoryResultFixture()
			owner := metadata["proof/Owner"]
			switch variant {
			case "ordinary invocation":
				call.Kind, call.IsSpecialInvoke = InvokeVirtual, false
			case "super invocation":
				call.ClassName = "proof.Other"
			case "nonconstructor source":
				ctx.FunctionName = "factory"
			case "missing PC":
				call.HasOriginPC = false
			case "unsealed receiver":
				self := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
				self.IsThis = true
				call.Object = self
			case "different receiver type":
				call.Object.(*JavaRef).ResetVarType(types.NewJavaClass("proof.Other"))
			case "unknown owner":
				delete(metadata, owner.Name)
			case "incomplete owner":
				owner.MembersComplete = false
			case "generic owner":
				owner.Signature = "<S:Ljava/lang/Object;>Ljava/lang/Object;"
			case "duplicate target":
				owner.Methods = append(owner.Methods, owner.Methods[0])
			case "static target":
				owner.Methods[0].Static = true
			case "bridge target":
				owner.Methods[0].Bridge = true
			case "varargs target":
				owner.Methods[0].Varargs = true
			case "method formal":
				owner.Methods[0].Signature = "<S:Ljava/lang/Object;>(ZLjava/util/List<TS;>;)V"
			case "unknown concrete argument":
				delete(metadata, "java/lang/String")
			case "arity mismatch":
				call.Descriptor = "(Ljava/util/List;)V"
			case "signature map mismatch":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, true }
			case "applicable rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "<init>", Desc: "(ZLjava/util/Collection;)V"})
			case "incomplete rival hierarchy":
				c := metadata["java/util/Collection"]
				c.ParentsComplete = false
				metadata[c.Name] = c
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "<init>", Desc: "(ZLproof/Unknown;)V"})
			case "noncanonical boolean":
				call.Arguments[0] = NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger))
			case "wrong boolean width":
				call.Arguments[0] = NewJavaLiteral(int64(1), types.NewJavaPrimer(types.JavaLong))
			case "opaque boolean":
				call.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			case "unproved producer":
				call.Arguments[1] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.util.List"))
			case "source access denial":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
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
				metadata[owner.Name] = owner
			}
			if target := call.delegationFactoryResultCast(1, call.Arguments[1], ctx); target != "" {
				t.Fatal("unproved declaration/inference/operand permission", target)
			}
		})
	}
}

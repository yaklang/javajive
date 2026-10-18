package values

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func erasedFactoryReceiverFixture() (*FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class) {
	getter, _, ctx, classes := erasedZeroArgumentFixture()
	cell := classes["proof/Cell"]
	cell.Methods = []callbinding.Method{{Name: "compare", Desc: "(Ljava/lang/Object;Ljava/lang/Object;)I", Signature: "(TV;TV;)I", Generic: true, Public: true, ExceptionsKnown: true}}
	classes[cell.Name] = cell
	factory := classes["proof/Factory"]
	factory.Methods = []callbinding.Method{{Name: "make", Desc: "()Lproof/Cell;", Signature: "<X:Lproof/Order<-TX;>;>()Lproof/Cell<TX;>;", Generic: true, Static: true, Public: true, ExceptionsKnown: true}}
	classes[factory.Name] = factory
	classes["proof/Order"] = callbinding.Class{Name: "proof/Order", Public: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}}
	owner := classes["proof/Owner"]
	owner.Signature = "<E:Lproof/Order<-TE;>;>Ljava/lang/Object;"
	classes[owner.Name] = owner
	ctx.ClassSig = owner.Signature
	ctx.CurrentMethodDesc, ctx.CurrentMethodSig = "(Lproof/Order;Lproof/Order;)I", "(TE;TE;)I"
	producer := getter.Object.(*FunctionCallExpression)
	producer.Arguments, producer.Descriptor = nil, factory.Methods[0].Desc
	ft, _ := types.ParseMethodDescriptor(producer.Descriptor)
	producer.FuncType = ft.FunctionType()
	ft, _ = types.ParseMethodDescriptor(cell.Methods[0].Desc)
	call := &FunctionCallExpression{Object: producer, ClassName: "proof.Cell", FunctionName: "compare", Descriptor: cell.Methods[0].Desc, FuncType: ft.FunctionType(), HasOriginPC: true, OriginPC: 12, Kind: InvokeVirtual}
	for _, slot := range []int{1, 2} {
		arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaTypeVariable("E"))
		arg.IsParam = true
		arg.MarkOriginalParameter(slot)
		call.Arguments = append(call.Arguments, arg)
	}
	return call, ctx, classes
}

func TestErasedFactoryReceiverKeepsOriginalInvocationAndBinderScope(t *testing.T) {
	for _, variant := range []string{
		"plain", "interface", "void", "null operand", "metadata signature", "normalized factory class", "same-package owner", "dependent factory bound", "same binder spelling", "existing public type head",
		"missing origin", "bad PC", "missing factory origin", "bad factory PC", "static consumer", "special consumer", "dynamic consumer",
		"instance factory", "dynamic factory", "factory with input", "wrong factory result", "wrong producer type", "inferred receiver",
		"reference result", "missing consumer model", "wrong consumer result model", "wrong consumer arity model", "wrong source result model", "method-generic consumer", "fixed member", "wrong member erasure", "unknown class formal",
		"incomplete member family", "incomplete factory family", "missing ancestor", "hierarchy cycle", "inherited member", "wrong interface role",
		"competing consumer", "duplicate consumer", "bridge consumer", "varargs consumer", "unknown consumer exceptions", "consumer exceptions",
		"raw factory", "foreign factory binder", "wrong factory erasure", "competing factory", "duplicate factory", "bridge factory", "varargs factory",
		"unknown factory exceptions", "factory exceptions", "inaccessible consumer", "inaccessible factory", "public declaration without source", "unknown nonpublic declaration", "denied existing type head", "stale signature table",
		"forged parameter", "wrong parameter slot", "inferred argument", "unproved nested inference", "new narrowing cast", "missing metadata",
	} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, classes := erasedFactoryReceiverFixture()
			cell, factory := classes["proof/Cell"], classes["proof/Factory"]
			producer := f.Object.(*FunctionCallExpression)
			want := variant == "plain" || variant == "interface" || variant == "void" || variant == "null operand" || variant == "metadata signature" || variant == "normalized factory class" || variant == "same-package owner" || variant == "dependent factory bound" || variant == "same binder spelling" || variant == "existing public type head" || variant == "public declaration without source"
			switch variant {
			case "interface":
				f.Kind, cell.IsInterface = InvokeInterface, true
			case "void":
				f.Descriptor, cell.Methods[0].Desc, cell.Methods[0].Signature = "(Ljava/lang/Object;Ljava/lang/Object;)V", "(Ljava/lang/Object;Ljava/lang/Object;)V", "(TV;TV;)V"
				ft, _ := types.ParseMethodDescriptor(f.Descriptor)
				f.FuncType = ft.FunctionType()
			case "null operand":
				f.Arguments[0] = NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
			case "metadata signature":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, false }
			case "normalized factory class":
				get := ctx.SiblingClassSig
				ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
					cs, methods, ok := get(name)
					if name == factory.Name {
						cs = "Ljava/lang/Object;"
					}
					return cs, methods, ok
				}
			case "same-package owner":
				cell.Public, factory.Public = false, false
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return true, true }
			case "dependent factory bound":
				factory.Methods[0].Signature = "<X:TY;Y:Lproof/Order<-TY;>;>()Lproof/Cell<TX;>;"
			case "same binder spelling":
				factory.Methods[0].Signature = "<E:Lproof/Order<-TE;>;>()Lproof/Cell<TE;>;"
			case "existing public type head", "denied existing type head":
				cell.Methods = append(cell.Methods, factory.Methods[0])
				producer.ClassName = "proof.Cell"
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, variant == "denied existing type head" }
			case "public declaration without source", "unknown nonpublic declaration":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, false }
				if variant == "unknown nonpublic declaration" {
					cell.Public = false
				}
			case "missing origin":
				f.HasOriginPC = false
			case "bad PC":
				f.OriginPC = -1
			case "missing factory origin":
				producer.HasOriginPC = false
			case "bad factory PC":
				producer.OriginPC = 65536
			case "static consumer":
				f.IsStatic, f.Kind = true, InvokeStatic
			case "special consumer":
				f.IsSpecialInvoke, f.Kind = true, InvokeSpecial
			case "dynamic consumer":
				f.Kind = InvokeDynamic
			case "instance factory":
				producer.IsStatic, producer.Kind = false, InvokeVirtual
			case "dynamic factory":
				producer.Kind = InvokeDynamic
			case "factory with input":
				producer.Arguments = append(producer.Arguments, f.Arguments[0])
			case "wrong factory result":
				producer.Descriptor = "()Ljava/lang/Object;"
			case "wrong producer type":
				producer.FuncType.ReturnType = types.NewJavaClass("proof.Other")
			case "inferred receiver":
				f.Object = NewCustomValue(func(*class_context.ClassContext) string { return "factory()" }, func() types.JavaType { return types.NewJavaClass("proof.Cell") })
			case "reference result":
				f.Descriptor = "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/lang/Object;"
			case "missing consumer model":
				f.FuncType = nil
			case "wrong consumer result model":
				f.FuncType.ReturnType = types.NewJavaPrimer(types.JavaLong)
			case "wrong consumer arity model":
				f.FuncType.ParamTypes = nil
			case "wrong source result model":
				f.SourceReturnType = types.NewJavaClass("java.lang.Object")
			case "method-generic consumer":
				cell.Methods[0].Signature = "<V:Ljava/lang/Object;>(TV;TV;)I"
			case "fixed member":
				cell.Methods[0].Signature = "(Ljava/lang/Object;Ljava/lang/Object;)I"
			case "wrong member erasure":
				cell.Methods[0].Signature = "(TV;TV;)J"
			case "unknown class formal":
				cell.Methods[0].Signature = "(TUnknown;TUnknown;)I"
			case "incomplete member family":
				cell.MembersComplete = false
			case "incomplete factory family":
				factory.MembersComplete = false
			case "missing ancestor":
				cell.Parents = []string{"proof/Missing"}
			case "hierarchy cycle":
				cell.Parents = []string{cell.Name}
			case "inherited member":
				parent := cell
				parent.Name = "proof/Parent"
				classes[parent.Name] = parent
				cell.Parents, cell.Methods = []string{parent.Name}, nil
			case "wrong interface role":
				f.Kind = InvokeInterface
			case "competing consumer":
				cell.Methods = append(cell.Methods, callbinding.Method{Name: "compare", Desc: "(Lproof/Order;Lproof/Order;)I", Public: true})
			case "duplicate consumer":
				cell.Methods = append(cell.Methods, cell.Methods[0])
			case "bridge consumer":
				cell.Methods[0].Bridge = true
			case "varargs consumer":
				cell.Methods[0].Varargs = true
			case "unknown consumer exceptions":
				cell.Methods[0].ExceptionsKnown = false
			case "consumer exceptions":
				cell.Methods[0].Exceptions = []string{"java/io/IOException"}
			case "raw factory":
				factory.Methods[0].Signature = "()Lproof/Cell;"
			case "foreign factory binder":
				factory.Methods[0].Signature = "<X:TE;>()Lproof/Cell<TX;>;"
			case "wrong factory erasure":
				factory.Methods[0].Signature = "<X:Ljava/lang/Object;>()Ljava/lang/Object;"
			case "competing factory":
				factory.Methods = append(factory.Methods, callbinding.Method{Name: "make", Desc: "()Lproof/Other;", Static: true, Public: true})
			case "duplicate factory":
				factory.Methods = append(factory.Methods, factory.Methods[0])
			case "bridge factory":
				factory.Methods[0].Bridge = true
			case "varargs factory":
				factory.Methods[0].Varargs = true
			case "unknown factory exceptions":
				factory.Methods[0].ExceptionsKnown = false
			case "factory exceptions":
				factory.Methods[0].Exceptions = []string{"java/io/IOException"}
			case "inaccessible consumer":
				ctx.SiblingClassAccessible = func(name string) (bool, bool) { return name != cell.Name, true }
			case "inaccessible factory":
				ctx.SiblingClassAccessible = func(name string) (bool, bool) { return name != factory.Name, true }
			case "stale signature table":
				get := ctx.SiblingClassSig
				ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
					cs, methods, ok := get(name)
					if name == cell.Name {
						cs = "<Z:Ljava/lang/Object;>Ljava/lang/Object;"
					}
					return cs, methods, ok
				}
			case "forged parameter":
				f.Arguments[0] = NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Order"))
			case "wrong parameter slot":
				ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Order"))
				ref.IsParam = true
				ref.MarkOriginalParameter(99)
				f.Arguments[0] = ref
			case "inferred argument":
				f.Arguments[0] = NewCustomValue(func(*class_context.ClassContext) string { return "value" }, func() types.JavaType { return types.NewJavaClass("proof.Order") })
			case "unproved nested inference":
				ft, _ := types.ParseMethodDescriptor("()Lproof/Order;")
				f.Arguments[0] = &FunctionCallExpression{ClassName: "proof.Factory", FunctionName: "other", Descriptor: "()Lproof/Order;", FuncType: ft.FunctionType(), IsStatic: true, Kind: InvokeStatic, OriginPC: 9, HasOriginPC: true}
			case "new narrowing cast":
				f.Arguments[0] = &CastExpression{Value: f.Arguments[0], TargetType: types.NewJavaClass("proof.Order"), Binding: true, OriginPC: 9}
			case "missing metadata":
				ctx.InvocationMetadata = nil
			}
			classes[cell.Name], classes[factory.Name] = cell, factory
			object := f.Object
			args := append([]JavaValue(nil), f.Arguments...)
			out, known := f.planErasedFactoryReceiver(ctx)
			if known != want {
				t.Fatalf("factory receiver=%v want=%v", known, want)
			}
			if f.Object != object || f.bindingPlanned {
				t.Fatal("planning mutated the original invocation")
			}
			for i, a := range args {
				if f.Arguments[i] != a {
					t.Fatal("planning changed a shared operand")
				}
			}
			if known {
				if out.Witness() != f.Witness() || !out.bindingPlanned {
					t.Fatal("changed original invoke identity")
				}
				cast, ok := out.Object.(*CastExpression)
				if !ok || cast.Value != object || !cast.Binding || bindingType(cast.TargetType) != "Lproof/Cell;" {
					t.Fatal("changed the producer or receiver erasure")
				}
				for i, a := range out.Arguments {
					c, ok := a.(*CastExpression)
					if !ok || !c.Binding || c.Value != args[i] || bindingType(c.TargetType) != "Ljava/lang/Object;" {
						t.Fatal("lost an exact nonthrowing descriptor view")
					}
				}
			}
		})
	}
}

func TestErasedFactoryReceiverMetersSharedMetadataAndCopies(t *testing.T) {
	f, ctx, _ := erasedFactoryReceiverFixture()
	ctx.Work = workbudget.New(nil, workbudget.Limits{})
	if _, known := f.planErasedFactoryReceiver(ctx); !known {
		t.Fatal("missing initial proof")
	}
	before := ctx.Work.Used(workbudget.CounterGraphScans)
	if _, known := f.planErasedFactoryReceiver(ctx); !known || ctx.Work.Used(workbudget.CounterGraphScans) <= before {
		t.Fatal("a warm lookup escaped the shared request budget")
	}
	for _, variant := range []string{"work", "storage", "copies", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			f, ctx, _ := erasedFactoryReceiverFixture()
			c, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := workbudget.Limits{}
			switch variant {
			case "work":
				limits.MaxGraphScans = 1
			case "storage":
				limits.MaxOutputBytes = 1
			case "copies":
				limits.MaxNodeCopies = 1
			case "canceled":
				cancel()
			}
			ctx.Work = workbudget.New(c, limits)
			if _, known := f.planErasedFactoryReceiver(ctx); known || ctx.Work.Err() == nil || f.bindingPlanned {
				t.Fatal("unavailable proof published a plan or lost its resource failure")
			}
		})
	}
}

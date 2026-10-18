package values

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func erasedZeroArgumentFixture() (*FunctionCallExpression, *FunctionCallExpression, *class_context.ClassContext, map[string]callbinding.Class) {
	classes := map[string]callbinding.Class{}
	for _, name := range []string{"java/lang/Object", "java/util/List", "proof/Owner", "proof/Cell", "proof/Factory"} {
		classes[name] = callbinding.Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true}
	}
	list := classes["java/util/List"]
	list.Parents = []string{"java/lang/Object"}
	classes[list.Name] = list
	owner := classes["proof/Owner"]
	owner.Signature = "<E:Ljava/lang/Object;>Ljava/lang/Object;"
	owner.Parents = []string{"java/lang/Object"}
	owner.Methods = []callbinding.Method{{Name: "consume", Desc: "(Ljava/util/List;)Ljava/util/List;", Signature: "(Ljava/util/List<TE;>;)Ljava/util/List<TE;>;", Generic: true, ExceptionsKnown: true}}
	classes[owner.Name] = owner
	cell := classes["proof/Cell"]
	cell.Signature = "<V:Ljava/lang/Object;>Ljava/lang/Object;"
	cell.Parents = []string{"java/lang/Object"}
	cell.Methods = []callbinding.Method{{Name: "get", Desc: "()Ljava/util/List;", Signature: "()Ljava/util/List<TV;>;", Public: true, Generic: true, ExceptionsKnown: true}}
	classes[cell.Name] = cell
	factory := classes["proof/Factory"]
	factory.Methods = []callbinding.Method{{Name: "make", Desc: "([Ljava/lang/Object;)Lproof/Cell;", Signature: "<X:Ljava/lang/Object;>([TX;)Lproof/Cell<TX;>;", Static: true, Public: true, Generic: true, ExceptionsKnown: true}}
	classes[factory.Name] = factory
	ctx := &class_context.ClassContext{ClassName: "proof.Owner", ClassSig: owner.Signature, TypeParams: []string{"E"}, ClassTypeParams: []string{"E"}, CurrentMethodDesc: "()Ljava/util/List;", CurrentMethodSig: "()Ljava/util/List<TE;>;"}
	ctx.InvocationMetadata = func(name string) (callbinding.Class, bool) { c, ok := classes[name]; return c, ok }
	ctx.SiblingClassSig = func(name string) (string, map[string]string, bool) {
		c, known := classes[name]
		methods := map[string]string{}
		for _, m := range c.Methods {
			methods[class_context.MethodDescKey(m.Name, m.Desc)] = m.Signature
		}
		return c.Signature, methods, known
	}
	makeCall := func(owner, name, descriptor string, pc int) *FunctionCallExpression {
		ft, _ := types.ParseMethodDescriptor(descriptor)
		return &FunctionCallExpression{ClassName: owner, FunctionName: name, Descriptor: descriptor, Kind: InvokeVirtual, OriginPC: pc, HasOriginPC: true, FuncType: ft.FunctionType()}
	}
	producer := makeCall("proof.Factory", "make", factory.Methods[0].Desc, 5)
	producer.Kind, producer.IsStatic = InvokeStatic, true
	producer.Arguments = []JavaValue{NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))}
	getter := makeCall("proof.Cell", "get", cell.Methods[0].Desc, 8)
	getter.Object = producer
	this := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
	this.IsThis, this.IsParam = true, true
	this.MarkOriginalParameter(0)
	this.MarkOriginalReceiver()
	consumer := makeCall("proof.Owner", "consume", owner.Methods[0].Desc, 11)
	consumer.Object, consumer.Arguments = this, []JavaValue{getter}
	return getter, consumer, ctx, classes
}

func TestErasedZeroArgumentResultKeepsProducerAndOriginalResultBoundary(t *testing.T) {
	for _, variant := range []string{"factory", "original parameter", "metadata-only signature", "known parameterized receiver", "unknown receiver", "forged parameter", "changed producer descriptor", "missing producer origin", "dynamic producer", "missing original call", "negative PC", "oversized PC", "static", "special", "wrong interface role", "bridge", "varargs", "unknown exceptions", "checked exceptions", "generic throws", "method inference", "unbound result", "raw result", "array result", "incomplete owner", "incomplete parent", "unknown signature", "missing metadata", "inaccessible class", "different zero-arg return", "stale original table", "duplicate target", "contradictory generic flag", "signature bound"} {
		t.Run(variant, func(t *testing.T) {
			getter, _, ctx, classes := erasedZeroArgumentFixture()
			cell := classes["proof/Cell"]
			producer := getter.Object.(*FunctionCallExpression)
			want := variant == "factory" || variant == "original parameter" || variant == "metadata-only signature"
			switch variant {
			case "original parameter", "forged parameter", "known parameterized receiver":
				ctx.CurrentMethodDesc = "(Lproof/Cell;)Ljava/util/List;"
				ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Cell"))
				ref.IsParam = true
				if variant != "forged parameter" {
					ref.MarkOriginalParameter(1)
				}
				if variant == "known parameterized receiver" {
					ref.ResetVarType(types.NewParameterizedType("proof.Cell", []types.JavaType{types.NewJavaTypeVariable("E")}))
				}
				getter.Object = ref
			case "unknown receiver":
				getter.Object = NewCustomValue(func(*class_context.ClassContext) string { return "unproved()" }, func() types.JavaType { return types.NewJavaClass("proof.Cell") })
			case "changed producer descriptor":
				producer.Descriptor = "([Ljava/lang/Object;)Ljava/lang/Object;"
			case "missing producer origin":
				producer.HasOriginPC = false
			case "dynamic producer":
				producer.Kind = InvokeDynamic
			case "missing original call":
				getter.HasOriginPC = false
			case "negative PC":
				getter.OriginPC = -1
			case "oversized PC":
				getter.OriginPC = 65536
			case "static":
				getter.Kind, getter.IsStatic = InvokeStatic, true
			case "special":
				getter.Kind, getter.IsSpecialInvoke = InvokeSpecial, true
			case "wrong interface role":
				getter.Kind = InvokeInterface
			case "bridge":
				cell.Methods[0].Bridge = true
			case "varargs":
				cell.Methods[0].Varargs = true
			case "unknown exceptions":
				cell.Methods[0].ExceptionsKnown = false
			case "checked exceptions":
				cell.Methods[0].Exceptions = []string{"java/io/IOException"}
			case "generic throws":
				cell.Methods[0].Signature += "^TV;"
			case "method inference":
				cell.Methods[0].Signature = "<V:Ljava/lang/Object;>()Ljava/util/List<TV;>;"
			case "unbound result":
				cell.Methods[0].Signature = "()Ljava/util/List<TUnknown;>;"
			case "raw result":
				cell.Methods[0].Signature = "()Ljava/util/List;"
			case "array result":
				getter.Descriptor, cell.Methods[0].Desc, cell.Methods[0].Signature = "()[Ljava/lang/Object;", "()[Ljava/lang/Object;", "()[TV;"
			case "incomplete owner":
				cell.MembersComplete = false
			case "incomplete parent":
				parent := classes["java/lang/Object"]
				parent.ParentsComplete = false
				classes[parent.Name] = parent
			case "metadata-only signature", "unknown signature":
				ctx.SiblingClassSig = func(string) (string, map[string]string, bool) { return "", nil, false }
				if variant == "unknown signature" {
					cell.Methods[0].Signature = ""
				}
			case "missing metadata":
				ctx.InvocationMetadata = nil
			case "inaccessible class":
				ctx.SiblingClassAccessible = func(string) (bool, bool) { return false, true }
			case "different zero-arg return":
				cell.Methods = append(cell.Methods, callbinding.Method{Name: "get", Desc: "()Ljava/util/ArrayList;", Public: true})
			case "stale original table":
				cell.Signature = "<W:Ljava/lang/Object;>Ljava/lang/Object;"
			case "duplicate target":
				cell.Methods = append(cell.Methods, cell.Methods[0])
			case "contradictory generic flag":
				cell.Methods[0].Generic = false
			case "signature bound":
				cell.Methods[0].Signature = strings.Repeat("x", 8193)
			}
			classes[cell.Name] = cell
			original := getter.Object
			out, known := getter.planErasedZeroArgumentResult(ctx)
			if known != want || known && (out == nil || out.Witness() != getter.Witness() || !out.bindingPlanned) {
				t.Fatalf("result boundary=%v want=%v", known, want)
			}
			if getter.Object != original || getter.bindingPlanned || len(getter.Arguments) != 0 {
				t.Fatal("planning mutated the shared original invocation")
			}
			if known {
				cast, ok := out.Object.(*CastExpression)
				if !ok || !cast.Binding || cast.Value != original || bindingType(cast.TargetType) != "Lproof/Cell;" {
					t.Fatal("the view changed receiver evaluation, source owner or runtime erasure")
				}
			}
		})
	}
}

func TestErasedClassArgumentResultRequiresAClosedSingleClassFormalConsumer(t *testing.T) {
	for _, variant := range []string{"plain", "self-bound result", "array rival", "method variable", "shadowed class formal", "raw-erased formal", "forced-erased formal", "forged this", "changed this type", "static", "foreign receiver", "multiple arguments", "non-getter", "incomplete family", "unknown result", "generic rival", "unknown nominal relation", "same-parameter return rival", "varargs rival", "changed class header", "duplicate target"} {
		t.Run(variant, func(t *testing.T) {
			getter, consumer, ctx, classes := erasedZeroArgumentFixture()
			owner := classes["proof/Owner"]
			want := variant == "plain" || variant == "array rival" || variant == "self-bound result"
			switch variant {
			case "self-bound result":
				owner.Signature = "<SELF:Lproof/Owner<TSELF;TE;>;E:Ljava/lang/Object;>Ljava/lang/Object;"
				ctx.ClassSig, ctx.TypeParams, ctx.ClassTypeParams = owner.Signature, []string{"SELF", "E"}, []string{"SELF", "E"}
				owner.Methods[0].Desc, owner.Methods[0].Signature = "(Ljava/util/List;)Lproof/Owner;", "(Ljava/util/List<TE;>;)TSELF;"
				consumer.Descriptor = owner.Methods[0].Desc
				ft, _ := types.ParseMethodDescriptor(consumer.Descriptor)
				consumer.FuncType = ft.FunctionType()
			case "array rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "([Ljava/lang/Object;)Ljava/util/List;", Signature: "([TE;)Ljava/util/List<TE;>;", Generic: true})
			case "method variable":
				owner.Methods[0].Signature = "<E:Ljava/lang/Object;>(Ljava/util/List<TE;>;)Ljava/util/List<TE;>;"
			case "shadowed class formal":
				ctx.CurrentMethodSig = "<E:Ljava/lang/Object;>()Ljava/util/List<TE;>;"
			case "raw-erased formal":
				ctx.RawEraseTypeVars = map[string]bool{"E": true}
			case "forced-erased formal":
				ctx.ForceParamEraseTypeVars = map[string]string{"E": "java.lang.Object"}
			case "forged this":
				ref := NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("proof.Owner"))
				ref.IsThis, ref.IsParam = true, true
				consumer.Object = ref
			case "changed this type":
				consumer.Object.(*JavaRef).ResetVarType(types.NewJavaClass("proof.Other"))
			case "static":
				consumer.IsStatic, consumer.Kind = true, InvokeStatic
			case "foreign receiver":
				consumer.ClassName = "proof.Other"
			case "multiple arguments":
				consumer.Arguments = append(consumer.Arguments, getter)
			case "non-getter":
				consumer.Arguments[0] = NewJavaLiteral(nil, types.NewJavaClass("java.util.List"))
			case "incomplete family":
				owner.MembersComplete = false
			case "unknown result":
				getter.Descriptor = "()Ljava/lang/Object;"
			case "generic rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "(Ljava/lang/Object;)Ljava/util/List;", Signature: "<X:Ljava/lang/Object;>(TX;)Ljava/util/List<TX;>;", Generic: true})
			case "unknown nominal relation":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "(Lproof/Marker;)Ljava/util/List;"})
				list := classes["java/util/List"]
				list.Parents = []string{"proof/Missing"}
				classes[list.Name] = list
			case "same-parameter return rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "(Ljava/util/List;)Ljava/lang/Object;"})
			case "varargs rival":
				owner.Methods = append(owner.Methods, callbinding.Method{Name: "consume", Desc: "([Ljava/lang/Object;)Ljava/util/List;", Varargs: true})
			case "changed class header":
				ctx.ClassSig = "<Different:Ljava/lang/Object;>Ljava/lang/Object;"
			case "duplicate target":
				owner.Methods = append(owner.Methods, owner.Methods[0])
			}
			classes[owner.Name] = owner
			view := consumer.erasedClassArgumentResultView(ctx)
			if (view != nil) != want {
				t.Fatalf("argument boundary=%v want=%v", view != nil, want)
			}
			if view != nil {
				cast, ok := view.(*CastExpression)
				if !ok || bindingType(cast.TargetType) != "Ljava/util/List;" || !strings.Contains(cast.TargetType.String(ctx), "E") {
					t.Fatal("lost the selected parameter's class-formal constraint")
				}
			}
			if consumer.Arguments[0] == getter && getter.bindingPlanned || consumer.bindingPlanned {
				t.Fatal("planning changed shared invocation inputs")
			}
		})
	}
}

func TestErasedZeroArgumentResultMetersWarmMetadataAndCancellation(t *testing.T) {
	getter, consumer, ctx, _ := erasedZeroArgumentFixture()
	ctx.Work = workbudget.New(nil, workbudget.Limits{})
	if _, known := getter.planErasedZeroArgumentResult(ctx); !known || ctx.Work.Used(workbudget.CounterGraphScans) == 0 {
		t.Fatal("the result proof did not charge warmed metadata")
	}
	before := ctx.Work.Used(workbudget.CounterGraphScans)
	if _, known := getter.planErasedZeroArgumentResult(ctx); !known || ctx.Work.Used(workbudget.CounterGraphScans) <= before {
		t.Fatal("a repeated result proof reset or escaped its shared work")
	}
	for _, variant := range []string{"work", "storage", "copies", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			workContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := workbudget.Limits{}
			if variant == "work" {
				limits.MaxGraphScans = 1
			} else if variant == "storage" {
				limits.MaxOutputBytes = 1
			} else if variant == "copies" {
				limits.MaxNodeCopies = 1
			} else {
				cancel()
			}
			ctx.Work = workbudget.New(workContext, limits)
			if _, known := getter.planErasedZeroArgumentResult(ctx); known || consumer.erasedClassArgumentResultView(ctx) != nil || ctx.Work.Err() == nil {
				t.Fatal("an unavailable request granted an erased result view")
			}
		})
	}
}

func TestErasedClassReceiverRefinementCannotBorrowMethodInference(t *testing.T) {
	for _, variant := range []string{"class member", "method variable", "non-generic class", "unproved operand", "changed invoke", "narrowed receiver", "exhausted graph"} {
		t.Run(variant, func(t *testing.T) {
			getter, _, ctx, classes := erasedZeroArgumentFixture()
			ctx.CurrentMethodDesc = "(Ljava/util/List;)Ljava/util/List;"
			arg := NewJavaRef(utils.NewRootVariableId(), nil, types.NewParameterizedType("java.util.List", []types.JavaType{types.NewJavaTypeVariable("E")}))
			arg.IsParam = true
			if variant != "unproved operand" {
				arg.MarkOriginalParameter(1)
			}
			cell := classes["proof/Cell"]
			signature := "(Ljava/util/List<TV;>;)Lproof/Cell<TV;>;"
			if variant == "method variable" {
				signature = "<X:Ljava/lang/Object;>(Ljava/util/List<TX;>;)Lproof/Cell<TX;>;"
			} else if variant == "non-generic class" {
				cell.Signature = "Ljava/lang/Object;"
				signature = "(Ljava/util/List<Ljava/lang/String;>;)Lproof/Cell;"
			}
			cell.Methods = append(cell.Methods, callbinding.Method{Name: "fluent", Desc: "(Ljava/util/List;)Lproof/Cell;", Signature: signature, Generic: true, Public: true, ExceptionsKnown: true})
			classes[cell.Name] = cell
			ft, _ := types.ParseMethodDescriptor(cell.Methods[1].Desc)
			before := &FunctionCallExpression{ClassName: "proof.Cell", FunctionName: "fluent", Descriptor: cell.Methods[1].Desc, Kind: InvokeVirtual, HasOriginPC: true, OriginPC: 7, Object: getter.Object, Arguments: []JavaValue{arg}, FuncType: ft.FunctionType()}
			after := before.Clone()
			after.Object = &CastExpression{Value: before.Object, TargetType: types.NewJavaClass("proof.Cell"), Binding: true, OriginPC: 7}
			after.Arguments[0] = &CastExpression{Value: arg, TargetType: types.NewJavaClass("java.util.List"), Binding: true, OriginPC: 7}
			after.bindingPlanned = true
			remaining := 128
			switch variant {
			case "changed invoke":
				after.OriginPC++
			case "narrowed receiver":
				after.Object.(*CastExpression).TargetType = types.NewJavaClass("proof.SubCell")
			case "exhausted graph":
				remaining = 0
			}
			if known := erasedClassOnlyProjection(before, after, ctx, &remaining); known != (variant == "class member") {
				t.Fatalf("receiver projection=%v for %s", known, variant)
			}
			if before.Object != getter.Object || before.Arguments[0] != arg || before.bindingPlanned {
				t.Fatal("checking a plan mutated the original producer or argument")
			}
		})
	}
}

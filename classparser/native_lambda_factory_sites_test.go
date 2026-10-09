package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

const nativeSharedLambdaProofFixture = `class AnonymousLambdaProofOwner {
 Object make(final Object capture){return new Object(){
  Object get(Object argument){java.util.function.Supplier<Object> a=()->capture;java.util.function.Supplier<Object> b=()->argument;return a.get()==b.get()?a.get():b.get();}
 };}
} class AnonymousLambdaProofForeign {}`

func TestNativeSharedLambdaFactoryRequiresExactOriginalSite(t *testing.T) {
	files := nativeCompileClasses(t, nativeSharedLambdaProofFixture)
	for _, variant := range []string{"original", "unknown pc", "wrong pc", "foreign method", "foreign code", "wrong arity", "duplicate consumption", "duplicate site", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			_, packet := nativeAnonymousLambdaTestPacket(t, files)
			view := packet.lambdaImplementation
			var name, desc string
			for method := range view.lambdaImplementations {
				name, _ = sourceBridgeUTF8(view.object, method.NameIndex)
				desc, _ = sourceBridgeUTF8(view.object, method.DescriptorIndex)
			}
			factories := view.lambdaContext.factorySites[name+desc]
			if len(factories) != 2 {
				t.Fatalf("actual javac shared factories: %d", len(factories))
			}
			site := factories[0]
			d := NewClassObjectDumper(view.object)
			d.nativeAnonymousLambdaCurrent = view
			d.CurrentMethod = site.method
			d.FuncCtx = &class_context.ClassContext{}
			code, pc, hasPC := site.code, site.pc, true
			captured := make([]values.JavaValue, len(site.operands))
			for i := range captured {
				captured[i] = values.JavaNull
			}
			switch variant {
			case "unknown pc":
				hasPC = false
			case "wrong pc":
				pc++
			case "foreign method":
				copy := *site.method
				d.CurrentMethod = &copy
			case "foreign code":
				copy := *code
				code = &copy
			case "wrong arity":
				captured = nil
			case "duplicate consumption":
				d.nativeLambdaFactorySources = map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource{site: {}}
			case "duplicate site":
				view.lambdaContext.factorySites[name+desc] = append(factories, site)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, ok := d.recordNativeLambdaFactorySource(name, desc, code, captured, pc, hasPC)
			if ok != (variant == "original") || ok && got != site {
				t.Fatalf("site consumed=%v exact=%v", ok, got == site)
			}
			if ok {
				var method *MemberInfo
				for m := range view.lambdaImplementations {
					method = m
				}
				if nativeLambdaFactorySourcesClosed(method, factories, d, nil) {
					t.Fatal("captured names alone certified an unrendered body")
				}
			}
		})
	}
}

func TestNativeSharedLambdaFinalGraphCannotBorrowParsedFactory(t *testing.T) {
	for _, variant := range []string{"original", "empty source", "missing pc", "wrong pc", "unknown captures", "different operand", "duplicate factory", "deferred historical local", "opaque node", "cycle", "budget"} {
		t.Run(variant, func(t *testing.T) {
			capture := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass("java.lang.Object"))
			lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "()->capture" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Supplier") }, nil)
			lambda.Flag = "lambda"
			lambda.CapturesKnown = true
			lambda.Captures = []values.JavaValue{capture}
			lambda.HasOriginPC = true
			lambda.OriginPC = 7
			source := &nativeLambdaLocalCaptureSource{factoryPC: 7, hasFactoryPC: true, values: []values.JavaValue{capture}, body: []statements.Statement{&statements.ReturnStatement{JavaValue: lambda}}}
			var work *workbudget.Budget
			switch variant {
			case "empty source":
				source.body = []statements.Statement{}
			case "missing pc":
				lambda.HasOriginPC = false
			case "wrong pc":
				lambda.OriginPC++
			case "unknown captures":
				lambda.CapturesKnown = false
			case "different operand":
				lambda.Captures[0] = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, capture.Type())
			case "duplicate factory":
				source.body = append(source.body, &statements.ReturnStatement{JavaValue: lambda})
			case "deferred historical local":
				local := values.NewJavaRef(utils.NewRootVariableId(), lambda, lambda.Type())
				source.body = []statements.Statement{&statements.ReturnStatement{JavaValue: local}}
			case "opaque node":
				lambda.Flag = "opaque"
			case "cycle":
				lambda.Captures = []values.JavaValue{lambda}
				source.values = []values.JavaValue{lambda}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if got := nativeLambdaFactoryRetained(source, work); got != (variant == "original") {
				t.Fatalf("final source retained=%v", got)
			}
		})
	}
}

func TestNativeSharedLambdaSourceClosesEveryOriginalEnvironment(t *testing.T) {
	files := nativeCompileClasses(t, nativeSharedLambdaProofFixture)
	for _, variant := range []string{"original", "missing site", "missing factory witness", "wrong factory pc", "foreign method", "foreign code", "wrong parameter slot", "same typed wrong receiver", "shared body cache", "foreign body", "stub body", "missing body", "checked escape", "duplicate packet", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			_, packet := nativeAnonymousLambdaTestPacket(t, files)
			view := packet.lambdaImplementation
			var method *MemberInfo
			for m := range view.lambdaImplementations {
				method = m
			}
			name, _ := sourceBridgeUTF8(view.object, method.NameIndex)
			desc, _ := sourceBridgeUTF8(view.object, method.DescriptorIndex)
			factories := view.lambdaContext.factorySites[name+desc]
			d := NewClassObjectDumper(view.object)
			d.nativeLambdaFactorySources = map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource{}
			d.lambdaMethods[name] = []string{desc}
			for _, site := range factories {
				operand := site.operands[0]
				typ := types.NewJavaClass("java.lang.Object")
				ref := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, typ)
				ref.IsParam = true
				ref.MarkOriginalParameter(operand.slot)
				var capture values.JavaValue = ref
				if operand.owner != "" {
					receiver := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, types.NewJavaClass(operand.owner))
					receiver.IsParam = true
					receiver.IsThis = true
					receiver.MarkOriginalParameter(0)
					receiver.MarkOriginalReceiver()
					field := values.NewRefMember(receiver, operand.member, typ)
					field.MarkOriginalFieldRead(values.NewJavaClassMember(operand.owner, operand.member, operand.descriptor, typ), operand.pc)
					field.HasOriginPC = true
					field.OriginPC = operand.pc
					capture = field
				}
				lambda := values.NewCustomValue(func(*class_context.ClassContext) string { return "()->capture" }, func() types.JavaType { return types.NewJavaClass("java.util.function.Supplier") })
				lambda.Flag = "lambda"
				lambda.HasOriginPC = true
				lambda.OriginPC = site.pc
				lambda.CapturesKnown = true
				lambda.Captures = []values.JavaValue{capture}
				d.nativeLambdaFactorySources[site] = &nativeLambdaLocalCaptureSource{method: site.method, code: site.code, factoryPC: site.pc, hasFactoryPC: true, values: lambda.Captures, body: []statements.Statement{&statements.ReturnStatement{JavaValue: lambda}}, implementationBody: &dumpedMethods{member: method, code: "return LCAP0;", bodyCode: "return LCAP0;"}}
			}
			source := d.nativeLambdaFactorySources[factories[0]]
			var work *workbudget.Budget
			switch variant {
			case "missing site":
				delete(d.nativeLambdaFactorySources, factories[1])
			case "missing factory witness":
				source.hasFactoryPC = false
			case "wrong factory pc":
				source.factoryPC++
			case "foreign method":
				copy := *source.method
				source.method = &copy
			case "foreign code":
				copy := *source.code
				source.code = &copy
			case "wrong parameter slot":
				for _, s := range d.nativeLambdaFactorySources {
					if ref, ok := s.values[0].(*values.JavaRef); ok {
						replacement := values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, ref.Type())
						replacement.IsParam = true
						replacement.MarkOriginalParameter(7)
						s.values[0] = replacement
					}
				}
			case "same typed wrong receiver":
				for _, s := range d.nativeLambdaFactorySources {
					if field, ok := s.values[0].(*values.RefMember); ok {
						field.Object = values.NewJavaRef(utils.NewRootVariableId(), values.JavaNull, field.Object.Type())
					}
				}
			case "shared body cache":
				d.nativeLambdaFactorySources[factories[1]].implementationBody = source.implementationBody
			case "foreign body":
				copy := *method
				source.implementationBody.member = &copy
			case "stub body":
				source.implementationBody.bodyCode = "stub"
			case "missing body":
				source.implementationBody = nil
			case "checked escape":
				source.implementationBody.checkedEscape = true
			case "duplicate packet":
				view.lambdaContext.factorySites[name+desc] = append(factories, factories[0])
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberLambdaSourceClosed(view, d, work); got != (variant == "original") {
				t.Fatalf("complete factory source=%v", got)
			}
		})
	}
}

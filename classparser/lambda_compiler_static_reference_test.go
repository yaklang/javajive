package javaclassparser

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialStaticCompilerReferenceReopensOriginalTableNamespaceAndPorts(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, defaultCompilerRegistrationFixture, "DefaultCompilerCases", "none")
	for _, scenario := range []string{"original", "missing factory", "unsealed factory", "wrong factory PC", "detached factory method", "detached factory code", "duplicate factory declaration", "duplicate factory code", "wrong capture count", "borrowed receiver role", "wrong source packet", "lost cache", "lost declaration table", "omitted original method", "duplicate method", "changed parent", "unknown exceptions", "invented generic flag", "changed static flag", "changed varargs flag", "changed class signature", "lost member completeness", "lost parent completeness", "value obscures type", "type variable obscures type", "changed qualifier", "changed original pack", "changed original stack", "changed original locals", "canceled", "work", "memory"} {
		t.Run(scenario, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["DefaultCompilerCases.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options.SourceCompiler, d.options.TargetSourceVersion = ModernJavac, 8
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			var factoryMethod *MemberInfo
			var code *CodeAttribute
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name == "zReference" {
					factoryMethod = method
					for _, attribute := range method.Attributes {
						if body, ok := attribute.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if factoryMethod == nil || code == nil {
				t.Fatal("missing authored factory")
			}
			descriptor, _ := sourceBridgeUTF8(object, factoryMethod.DescriptorIndex)
			d.CurrentMethod = factoryMethod
			d.FuncCtx = &class_context.ClassContext{ClassName: object.GetClassName(), FunctionName: "zReference", CurrentMethodDesc: descriptor, IsStatic: true}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(object.ConstantPool, index) })
			if decoder.ParseOpcode() != nil {
				t.Fatal("authored factory opcodes")
			}
			pc := -1
			for _, instruction := range constructorMotionOps(decoder) {
				if instruction.Instr.OpCode == core.OP_INVOKEDYNAMIC {
					if pc != -1 {
						t.Fatal("ambiguous authored factory")
					}
					pc = int(instruction.CurrentOffset)
				}
			}
			request, known := d.newLambdaMethodSourceRequest(0)
			request.factory = &lambdaSourceFactory{object: object, method: factoryMethod, code: code, pc: pc}
			if !known || !request.factory.seal(nil) {
				t.Fatal("original factory seal")
			}
			site, known := d.originalCompilerReferenceSite(request.factory)
			if !known {
				t.Fatal("original typed bootstrap edge")
			}
			if !d.ordinaryArrowChangesStaticImplementationPorts(site.implementation.Member, site.implementation.Description, request) {
				t.Fatal("original Object ports must not fall back to String/Integer arrow ports")
			}
			source, known := d.dumpCompilerVarargsReference(site.implementation.Member, site.implementation.Description, nil, request)
			if !known || source.ReferenceTarget == nil || source.ReferenceOwner != object.GetClassName() || !strings.Contains(source.Code, "::join") {
				t.Fatalf("missing completed static source packet: known=%v source=%+v", known, source)
			}
			var body *dumpedMethods
			for _, receipt := range d.lambdaMethodSourceBodies {
				body = receipt.body
			}
			if body == nil || !d.lambdaMethodSourcePacketClosed(body.member, body, nil) {
				t.Fatal("original static packet did not close")
			}
			provider := body.compilerReference.context.InvocationMetadata
			mutateMetadata := func(mutate func(*callbinding.Class)) {
				body.compilerReference.context.InvocationMetadata = func(name string) (callbinding.Class, bool) {
					cl, known := provider(name)
					if name == object.GetClassName() {
						cl.Methods, cl.Parents = slices.Clone(cl.Methods), slices.Clone(cl.Parents)
						mutate(&cl)
					}
					return cl, known
				}
			}
			switch scenario {
			case "missing factory":
				body.sourceRequest.factory = nil
			case "unsealed factory":
				request.factory.sealed = false
			case "wrong factory PC":
				request.factory.pc++
			case "detached factory method":
				copy := *factoryMethod
				request.factory.method = &copy
			case "detached factory code":
				copy := *code
				request.factory.code = &copy
			case "duplicate factory declaration":
				object.Methods = append(object.Methods, factoryMethod)
			case "duplicate factory code":
				factoryMethod.Attributes = append(factoryMethod.Attributes, code)
			case "wrong capture count":
				body.sourceRequest.captureCount = 1
			case "borrowed receiver role":
				body.compilerReference.static = false
			case "wrong source packet":
				body.code = "DefaultCompilerCases::aOrdinary"
			case "lost cache":
				delete(d.dumpedMethodsSet, body.sourceCacheKey)
			case "lost declaration table":
				body.compilerReference.context.InvocationMetadata = nil
			case "omitted original method":
				mutateMetadata(func(cl *callbinding.Class) { cl.Methods = cl.Methods[1:] })
			case "duplicate method":
				mutateMetadata(func(cl *callbinding.Class) { cl.Methods = append(cl.Methods, cl.Methods[0]) })
			case "changed parent":
				mutateMetadata(func(cl *callbinding.Class) { cl.Parents = nil })
			case "changed class signature":
				mutateMetadata(func(cl *callbinding.Class) { cl.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;" })
			case "lost member completeness":
				mutateMetadata(func(cl *callbinding.Class) { cl.MembersComplete = false })
			case "lost parent completeness":
				mutateMetadata(func(cl *callbinding.Class) { cl.ParentsComplete = false })
			case "unknown exceptions", "invented generic flag", "changed static flag", "changed varargs flag":
				mutateMetadata(func(cl *callbinding.Class) {
					for i := range cl.Methods {
						if cl.Methods[i].Name != "join" {
							continue
						}
						switch scenario {
						case "unknown exceptions":
							cl.Methods[i].ExceptionsKnown = false
						case "invented generic flag":
							cl.Methods[i].Generic = true
						case "changed static flag":
							cl.Methods[i].Static = false
						case "changed varargs flag":
							cl.Methods[i].Varargs = false
						}
					}
				})
			case "value obscures type":
				body.compilerReference.context.Arguments = []string{object.GetClassName()}
			case "type variable obscures type":
				body.compilerReference.context.TypeParams = []string{object.GetClassName()}
			case "changed qualifier":
				body.compilerReference.qualifier = "Unrelated"
			case "changed original pack":
				body.compilerReference.shape.code.Code[0] = core.OP_ICONST_1
			case "changed original stack":
				body.compilerReference.shape.code.MaxStack++
			case "changed original locals":
				body.compilerReference.shape.code.MaxLocals++
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			before := object.Bytes()
			if closed := d.lambdaMethodSourcePacketClosed(body.member, body, d.Work); closed != (scenario == "original") {
				t.Fatalf("mutated static source packet closed=%v", closed)
			}
			if !bytes.Equal(before, object.Bytes()) {
				t.Fatal("source proof mutated its original declaration or Code")
			}
		})
	}
}

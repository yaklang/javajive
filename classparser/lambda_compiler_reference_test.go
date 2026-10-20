package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialCompilerVarargsReferenceRequiresOriginalFactoryAndRoles(t *testing.T) {
	modern, _ := t04Tools(t)
	fixture := strings.NewReplacer("void accept(Object value){", "Object[] accept(Object... values){if(values.length!=1)throw new AssertionError(\"varargs shape\");Object value=values[0];", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;}", "BoundEffects.lastValue=value;if(BoundEffects.mode==3)throw BoundEffects.failure;return values;}").Replace(constructorBoundReferenceFamilyFixture)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "BoundConstructorOwner", "none", named)
					for _, variant := range []string{"original", "missing factory", "unsealed factory", "different factory PC", "detached factory declaration", "detached factory code", "wrong receiver slot", "implicit receiver", "wrong receiver source type", "missing declarations", "incomplete overload family", "changed invocation kind", "lost varargs declaration", "unknown target exceptions", "checked target exception", "wrong SAM signature", "wrong source representation", "changed body packet", "changed body stack limit", "changed body local limit", "changed factory stack limit", "changed factory local limit", "detached source cache", "ordinary body representation", "missing source proof", "changed original pack", "private original target", "ordinary array original target", "static original target", "missing original target", "negative source language", "unknown source compiler", "implicit source language", "future class major", "canceled", "work", "memory"} {
						t.Run(variant, func(t *testing.T) {
							obj, err := Parse(bytes.Clone(files["BoundConstructorOwner$Parts.class"]))
							if err != nil {
								t.Fatal(err)
							}
							d := NewClassObjectDumper(obj)
							d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
							d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
							var method *MemberInfo
							var code *CodeAttribute
							for _, m := range obj.Methods {
								name, _ := sourceBridgeUTF8(obj, m.NameIndex)
								desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
								if name == "<init>" && desc == "(LBoundProducer;I)V" {
									method = m
									for _, a := range m.Attributes {
										if c, ok := a.(*CodeAttribute); ok {
											code = c
										}
									}
								}
							}
							if code == nil {
								t.Fatal("original authored factory missing")
							}
							d.CurrentMethod = method
							d.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName(), FunctionName: "<init>", CurrentMethodDesc: "(LBoundProducer;I)V"}
							d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
							receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("BoundProducer"))
							receiver.IsParam = true
							receiver.MarkOriginalParameter(1)
							receiver.Id.SetName("producer")
							decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
							if decoder.ParseOpcode() != nil {
								t.Fatal("original opcodes")
							}
							pc := -1
							for _, op := range constructorMotionOps(decoder) {
								if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
									if pc != -1 {
										t.Fatal("unique authored factory")
									}
									pc = int(op.CurrentOffset)
								}
							}
							request, ok := d.newLambdaMethodSourceRequest(1)
							if !ok {
								t.Fatal("request")
							}
							request.factory = &lambdaSourceFactory{object: obj, method: method, code: code, pc: pc}
							if !request.factory.seal(nil) {
								t.Fatal("factory seal")
							}
							site, ok := d.originalCompilerReferenceSite(request.factory)
							if !ok {
								t.Fatal("original bootstrap")
							}
							name, desc := site.implementation.Member, site.implementation.Description
							source, ok := d.dumpCompilerVarargsReference(name, desc, []values.JavaValue{receiver}, request)
							if !ok || source.ReferenceTarget == nil {
								t.Fatal("original compiler reference requires a completed typed source proof")
							}
							var body *dumpedMethods
							for _, r := range d.lambdaMethodSourceBodies {
								body = r.body
							}
							if body == nil || !d.lambdaMethodSourcePacketClosed(body.member, body, nil) {
								t.Fatal("completed original reference receipt")
							}
							before := bytes.Clone(code.Code)
							provider := d.FuncCtx.InvocationMetadata
							mutateProvider := func(fn func(string, *callbinding.Class)) {
								wrapped := func(n string) (callbinding.Class, bool) {
									cl, ok := provider(n)
									cl.Methods = append([]callbinding.Method(nil), cl.Methods...)
									fn(n, &cl)
									return cl, ok
								}
								body.compilerReference.context.InvocationMetadata = wrapped
								d.FuncCtx.InvocationMetadata = wrapped
							}
							switch variant {
							case "missing factory":
								body.sourceRequest.factory = nil
							case "unsealed factory":
								request.factory.sealed = false
							case "different factory PC":
								request.factory.pc++
							case "detached factory declaration":
								copy := *method
								request.factory.method = &copy
							case "detached factory code":
								copy := *code
								request.factory.code = &copy
							case "wrong receiver slot":
								replacement := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("BoundProducer"))
								replacement.IsParam = true
								replacement.MarkOriginalParameter(2)
								body.compilerReference.receiver = replacement
							case "implicit receiver":
								receiver.IsThis = true
							case "wrong receiver source type":
								receiver.ResetVarType(types.NewJavaClass("java/lang/Object"))
							case "missing declarations":
								body.compilerReference.context.InvocationMetadata = nil
							case "incomplete overload family":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "BoundProducer" {
										cl.MembersComplete = false
									}
								})
							case "changed invocation kind":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "BoundProducer" {
										cl.IsInterface = true
									}
								})
							case "lost varargs declaration":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "BoundProducer" {
										for i := range cl.Methods {
											cl.Methods[i].Varargs = false
										}
									}
								})
							case "unknown target exceptions":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "BoundProducer" {
										for i := range cl.Methods {
											cl.Methods[i].ExceptionsKnown = false
										}
									}
								})
							case "checked target exception":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "BoundProducer" {
										for i := range cl.Methods {
											cl.Methods[i].Exceptions = []string{"java/io/IOException"}
										}
									}
								})
							case "wrong SAM signature":
								mutateProvider(func(n string, cl *callbinding.Class) {
									if n == "java/util/function/Consumer" {
										for i := range cl.Methods {
											if cl.Methods[i].Name == "accept" {
												cl.Methods[i].Signature = "(Ljava/lang/String;)V"
											}
										}
									}
								})
							case "wrong source representation":
								body.sourceRequest.role = methodSourceDeclaration
							case "changed body packet":
								body.code = "(ignored)->{}"
							case "changed body stack limit":
								body.compilerReference.shape.code.MaxStack++
							case "changed body local limit":
								body.compilerReference.shape.code.MaxLocals++
							case "changed factory stack limit":
								code.MaxStack++
							case "changed factory local limit":
								code.MaxLocals++
							case "detached source cache":
								copy := *body
								d.dumpedMethodsSet[body.sourceCacheKey] = &copy
							case "ordinary body representation":
								body.sourceRequest.role = methodSourceLambdaBody
							case "missing source proof":
								body.compilerReference = nil
							case "changed original pack":
								body.compilerReference.shape.code.Code[1] = core.OP_ICONST_0
							case "private original target", "ordinary array original target", "static original target", "missing original target":
								target, known := d.nativeAnnotationDeclarationResolver()("BoundProducer")
								if !known {
									t.Fatal("original target declaration")
								}
								for i, m := range target.Methods {
									name, _ := sourceBridgeUTF8(target, m.NameIndex)
									if name != "accept" {
										continue
									}
									switch variant {
									case "private original target":
										m.AccessFlags |= 2
									case "ordinary array original target":
										m.AccessFlags &^= 0x80
									case "static original target":
										m.AccessFlags |= 8
									case "missing original target":
										target.Methods = append(target.Methods[:i], target.Methods[i+1:]...)
									}
									break
								}
								raw := target.Bytes()
								originalResolver := d.foldSiblingResolver
								d.foldSiblingResolver = func(name string) ([]byte, bool) {
									if name == "BoundProducer" {
										return bytes.Clone(raw), true
									}
									return originalResolver(name)
								}
							case "negative source language":
								d.options.TargetSourceVersion = -1
							case "unknown source compiler":
								d.options.SourceCompiler = "unknown"
							case "implicit source language":
								d.options.TargetSourceVersion = 0
							case "future class major":
								d.obj.MajorVersion = 67
							case "canceled":
								ctx, cancel := context.WithCancel(context.Background())
								cancel()
								d.Work = workbudget.New(ctx, workbudget.Limits{})
							case "work":
								d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
							case "memory":
								d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
							}
							got := d.lambdaMethodSourcePacketClosed(body.member, body, d.Work)
							if got != (variant == "original" || variant == "implicit source language" && profile == ModernJavac) {
								t.Fatalf("%s source permission=%v", variant, got)
							}
							if !bytes.Equal(before, code.Code) {
								t.Fatal("proof mutated original factory Code")
							}
						})
					}
				})
			}
		})
	}
}

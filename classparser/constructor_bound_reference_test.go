package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorBoundReferenceRequiresOriginalEvaluationAndBinding(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, constructorBoundReferenceFamilyFixture, "BoundConstructorOwner", "none", false)
			for _, variant := range []string{"original", "factory consumed before delegation", "detached constructor declaration", "unknown old renderer", "unsealed factory", "changed capture", "copied parameter", "changed source receiver type", "wrong check receiver", "missing factory PC", "wrong check PC", "wrong delegation owner", "nonconstructor", "different compiler protocol", "unknown compiler profile", "foreign null-check owner", "extra prefix", "protected prefix", "missing declarations", "wrong declaration owner", "incomplete member family", "member overload", "varargs member", "static member", "method generic member", "noninterface target", "missing SAM signature", "changed SAM signature", "changed constructor signature", "duplicate original constructor signature", "missing bootstrap", "duplicate bootstrap", "different bootstrap", "static implementation handle", "special implementation handle", "extra bootstrap operand", "different receiver descriptor", "budget", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					obj, err := Parse(bytes.Clone(files["BoundConstructorOwner$Parts.class"]))
					if err != nil {
						t.Fatal(err)
					}
					d := NewClassObjectDumper(obj)
					d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
					d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
					d.FuncCtx = &class_context.ClassContext{ClassName: obj.GetClassName(), FunctionName: "<init>", CurrentMethodDesc: "(LBoundProducer;I)V"}
					d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
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
						t.Fatal("missing authored constructor")
					}
					beforeCode := bytes.Clone(code.Code)
					this := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass(obj.GetClassName()))
					this.IsParam, this.IsThis = true, true
					this.MarkOriginalParameter(0)
					this.MarkOriginalReceiver()
					receiver := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("BoundProducer"))
					receiver.IsParam = true
					receiver.MarkOriginalParameter(1)
					receiver.Id.SetName("producer")
					tag := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
					tag.IsParam = true
					tag.MarkOriginalParameter(2)
					params := []values.JavaValue{this, receiver, tag}
					factory := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error {
						panic("old source renderer is not binding evidence")
					}, func() types.JavaType { panic("old source type callback is not binding evidence") })
					factory.Flag, factory.IsMethodRef, factory.CapturesKnown, factory.NoOuterCapture = "lambda", true, true, true
					factory.OriginPC, factory.HasOriginPC, factory.InstantiatedMtdDesc = 7, true, "(Ljava/lang/Object;)V"
					factory.Captures = []values.JavaValue{receiver}
					if variant == "copied parameter" {
						copied := *receiver
						factory.Captures[0] = &copied
					}
					if variant != "unsealed factory" {
						factory.MarkOriginalLambdaFactory(7)
					}
					check := &values.FunctionCallExpression{ClassName: "java.lang.Object", FunctionName: "getClass", Descriptor: "()Ljava/lang/Class;", Kind: values.InvokeVirtual, Object: receiver, OriginPC: 3, HasOriginPC: true}
					if profile == ModernJavac {
						check.ClassName, check.FunctionName, check.Descriptor, check.Kind, check.IsStatic = "java.util.Objects", "requireNonNull", "(Ljava/lang/Object;)Ljava/lang/Object;", values.InvokeStatic, true
						check.Arguments = []values.JavaValue{receiver}
					}
					argument := &values.FunctionCallExpression{ClassName: "BoundEffects", FunctionName: "argument", Descriptor: "(I)I", IsStatic: true, Kind: values.InvokeStatic, OriginPC: 13, HasOriginPC: true, Arguments: []values.JavaValue{tag}}
					delegate := &values.FunctionCallExpression{ClassName: obj.GetClassName(), FunctionName: "<init>", Descriptor: "(Ljava/util/function/Consumer;I)V", Kind: values.InvokeSpecial, Object: this, OriginPC: 16, HasOriginPC: true, Arguments: []values.JavaValue{factory, argument}}
					body := []statements.Statement{&statements.ExpressionStatement{Expression: check}, &statements.ExpressionStatement{Expression: delegate}}
					provider := d.FuncCtx.InvocationMetadata
					d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
						cl, ok := provider(name)
						cl.Methods = append([]callbinding.Method(nil), cl.Methods...)
						if variant == "missing declarations" {
							return cl, false
						}
						if name == "BoundProducer" {
							switch variant {
							case "wrong declaration owner":
								cl.Name = "DifferentProducer"
							case "incomplete member family":
								cl.ParentsComplete = false
							case "member overload":
								cl.Methods = append(cl.Methods, callbinding.Method{Name: "accept", Desc: "(Ljava/lang/String;)V", Public: true})
							}
							for i := range cl.Methods {
								if cl.Methods[i].Name == "accept" {
									cl.Methods[i].Varargs = variant == "varargs member"
									cl.Methods[i].Static = variant == "static member"
									cl.Methods[i].Generic = variant == "method generic member"
								}
							}
						}
						if name == "java/util/function/Consumer" {
							if variant == "noninterface target" {
								cl.IsInterface = false
							}
							for i := range cl.Methods {
								if cl.Methods[i].Name == "accept" {
									switch variant {
									case "missing SAM signature":
										cl.Methods[i].Signature = ""
									case "changed SAM signature":
										cl.Methods[i].Signature = "(Ljava/lang/String;)V"
									}
								}
							}
						}
						return cl, ok
					}
					var boot *BootstrapMethodsAttribute
					for _, a := range obj.Attributes {
						if b, ok := a.(*BootstrapMethodsAttribute); ok {
							boot = b
						}
					}
					if boot == nil || len(boot.BootstrapMethods) != 1 {
						t.Fatal("authored metafactory inventory")
					}
					b := boot.BootstrapMethods[0]
					switch variant {
					case "factory consumed before delegation":
						originalArgumentIndex := append([]byte(nil), code.Code[14:16]...)
						originalDelegateIndex := append([]byte(nil), code.Code[17:19]...)
						tail := []byte{core.OP_POP, core.OP_ACONST_NULL, core.OP_ILOAD_2, core.OP_INVOKESTATIC, originalArgumentIndex[0], originalArgumentIndex[1], core.OP_INVOKESPECIAL, originalDelegateIndex[0], originalDelegateIndex[1], core.OP_RETURN}
						code.Code = append(append([]byte(nil), code.Code[:12]...), tail...)
						delegate.OriginPC, argument.OriginPC = 18, 15
						beforeCode = bytes.Clone(code.Code)
					case "detached constructor declaration":
						copy := *method
						method = &copy
					case "changed capture":
						copy := *receiver
						factory.Captures[0] = &copy
					case "changed source receiver type":
						receiver.ResetVarType(types.NewJavaClass("java.lang.Object"))
					case "wrong check receiver":
						check.Object, check.Arguments = tag, []values.JavaValue{tag}
					case "missing factory PC":
						factory.HasOriginPC = false
					case "wrong check PC":
						check.OriginPC = 7
					case "wrong delegation owner":
						delegate.ClassName = "DifferentOwner"
					case "nonconstructor":
						method.NameIndex = obj.ThisClass
					case "different compiler protocol":
						d.options.SourceCompiler = map[SourceCompilerProfile]SourceCompilerProfile{ModernJavac: NativeJavac8, NativeJavac8: ModernJavac}[profile]
					case "unknown compiler profile":
						d.options.SourceCompiler = SourceCompilerProfile("unknown")
					case "foreign null-check owner":
						index := int(code.Code[4])<<8 | int(code.Code[5])
						ref := nativeConstantMember(obj.ConstantPool[index-1])
						if ref == nil {
							t.Fatal("original null-check reference")
						}
						ref.ClassIndex = uint16(obj.ConstantPoolManager.AddNewClassInfo("ForeignNullCheck"))
						check.ClassName = "ForeignNullCheck"
					case "extra prefix":
						body = append([]statements.Statement{&statements.ExpressionStatement{Expression: argument}}, body...)
					case "protected prefix":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 1, EndPc: 12, HandlerPc: 19}}
					case "changed constructor signature", "duplicate original constructor signature":
						for _, m := range obj.Methods {
							desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
							if desc == delegate.Descriptor {
								for _, a := range m.Attributes {
									if sig, ok := a.(*SignatureAttribute); ok {
										if variant == "duplicate original constructor signature" {
											m.Attributes = append(m.Attributes, sig)
										} else {
											obj.ConstantPool[sig.SignatureIndex-1].(*ConstantUtf8Info).Value = "(Ljava/util/function/Consumer<Ljava/lang/String;>;I)V"
										}
									}
								}
							}
						}
					case "missing bootstrap":
						obj.Attributes = nil
					case "duplicate bootstrap":
						obj.Attributes = append(obj.Attributes, boot)
					case "different bootstrap":
						obj.ConstantPool[b.BootstrapMethodRef-1].(*ConstantMethodHandleInfo).ReferenceKind = 8
					case "static implementation handle":
						obj.ConstantPool[b.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo).ReferenceKind = 6
					case "special implementation handle":
						obj.ConstantPool[b.BootstrapArguments[1]-1].(*ConstantMethodHandleInfo).ReferenceKind = 7
					case "extra bootstrap operand":
						b.BootstrapArguments = append(b.BootstrapArguments, b.BootstrapArguments[0])
						b.NumBootstrapArguments++
					case "different receiver descriptor":
						site := obj.ConstantPool[core.Convert2bytesToInt(code.Code[8:10])-1].(*ConstantInvokeDynamicInfo)
						nt := obj.ConstantPool[site.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						obj.ConstantPool[nt.DescriptorIndex-1].(*ConstantUtf8Info).Value = "(Ljava/lang/Object;)Ljava/util/function/Consumer;"
					case "budget":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					before := append([]statements.Statement(nil), body...)
					got := d.constructorBoundReferenceSource(code, body, params, method)
					want := variant == "original" || variant == "unknown old renderer" || variant == "different compiler protocol"
					if (len(got) == len(body)-1) != want {
						t.Fatalf("source check absorbed=%v, want %v", len(got) == len(body)-1, want)
					}
					if !reflect.DeepEqual(body, before) || !bytes.Equal(code.Code, beforeCode) || delegate.Arguments[0] != factory || delegate.Arguments[1] != argument {
						t.Fatal("projection mutated the original typed body, operands or bytecode")
					}
					if want {
						call := got[0].(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression)
						if call == delegate || call.Arguments[1] != argument || !strings.Contains(call.Arguments[0].String(d.FuncCtx), "producer::accept") {
							t.Fatal("projected reference lost the original receiver or later argument")
						}
					} else if !reflect.DeepEqual(got, body) {
						t.Fatal("refused proof changed its source statements")
					}
				})
			}
		})
	}
}

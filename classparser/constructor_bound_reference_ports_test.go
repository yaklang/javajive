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

func TestConstructorBoundReferencePortsRequireOrderedOriginalFactoryConsumers(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = native
			}
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, constructorBoundReferencePortsFixture, "BoundPairOwner", "none", false)
			for _, variant := range []string{"exact", "swapped factories", "swapped checks", "changed normal port", "wrong factory port", "changed receiver", "duplicate factory", "unsealed factory", "wrong check receiver", "extra check", "foreign null check", "changed SAM argument", "free constructor variable", "missing declarations", "wrong owner", "work", "cancelled"} {
				t.Run(variant, func(t *testing.T) {
					obj := mustParseLowering(t, bytes.Clone(files["BoundPairOwner.class"]))
					d := NewClassObjectDumper(obj)
					d.options.SourceCompiler, d.options.TargetSourceVersion = profile, 8
					d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok }
					const desc = "(Ljava/lang/Object;LPairProducer;LPairProducer;J)V"
					d.FuncCtx = &class_context.ClassContext{ClassName: "BoundPairOwner", FunctionName: "<init>", CurrentMethodDesc: desc}
					d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
					var method *MemberInfo
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n == "<init>" {
							method = m
						}
					}
					code := constructorLoweringCode(method)
					if code == nil {
						t.Fatal("original constructor missing")
					}
					before := bytes.Clone(code.Code)
					decoder := core.NewDecompiler(code.Code, nil)
					if decoder.ParseOpcode() != nil {
						t.Fatal("original instructions")
					}
					ops := constructorMotionOps(decoder)
					newParam := func(slot int, typ types.JavaType) *values.JavaRef {
						r := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
						r.IsParam = true
						r.MarkOriginalParameter(slot)
						return r
					}
					this := newParam(0, types.NewJavaClass("BoundPairOwner"))
					this.IsThis = true
					this.MarkOriginalReceiver()
					token := newParam(1, types.NewJavaClass("java/lang/Object"))
					left, right := newParam(2, types.NewJavaClass("PairProducer")), newParam(3, types.NewJavaClass("PairProducer"))
					bits := newParam(4, types.NewJavaPrimer(types.JavaLong))
					params := []values.JavaValue{this, token, left, right, bits}
					var factories []*values.CustomValue
					var checks []*values.FunctionCallExpression
					var delegate *values.FunctionCallExpression
					for i, op := range ops {
						if op.Instr.OpCode == core.OP_INVOKEDYNAMIC {
							receiver := left
							if len(factories) == 1 {
								receiver = right
							}
							f := values.NewStreamingCustomValue(func(*class_context.ClassContext, *workbudget.Writer) error { panic("old renderer is not evidence") }, func() types.JavaType { panic("old type callback is not evidence") })
							f.IsMethodRef, f.CapturesKnown = true, true
							f.Flag, f.OriginPC, f.HasOriginPC = "lambda", int(op.CurrentOffset), true
							f.Captures = []values.JavaValue{receiver}
							f.MarkOriginalLambdaFactory(int(op.CurrentOffset))
							factories = append(factories, f)
							checkOp := ops[i-2]
							member := constructorMotionMember(obj, checkOp, checkOp.Instr.OpCode)
							if member == nil {
								t.Fatal("original intrinsic check")
							}
							call := &values.FunctionCallExpression{ClassName: member.Name, FunctionName: member.Member, Descriptor: member.Description, Kind: values.InvokeVirtual, Object: receiver, OriginPC: int(checkOp.CurrentOffset), HasOriginPC: true}
							if checkOp.Instr.OpCode == core.OP_INVOKESTATIC {
								call.Kind, call.IsStatic, call.Object, call.Arguments = values.InvokeStatic, true, nil, []values.JavaValue{receiver}
							}
							checks = append(checks, call)
						} else if op.Instr.OpCode == core.OP_INVOKESPECIAL {
							member := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
							delegate = &values.FunctionCallExpression{ClassName: member.Name, FunctionName: member.Member, Descriptor: member.Description, Kind: values.InvokeSpecial, Object: this, OriginPC: int(op.CurrentOffset), HasOriginPC: true}
						}
					}
					if len(factories) != 2 || delegate == nil {
						t.Fatal("original factory inventory")
					}
					delegate.Arguments = []values.JavaValue{token, left, factories[0], factories[1], bits}
					provider := d.FuncCtx.InvocationMetadata
					d.FuncCtx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
						c, known := provider(n)
						c.Methods = append([]callbinding.Method(nil), c.Methods...)
						if variant == "missing declarations" {
							return c, false
						}
						if n == "PairParent" {
							if variant == "wrong owner" {
								c.Name = "UnrelatedParent"
							}
							for i := range c.Methods {
								if c.Methods[i].Name == "<init>" {
									if variant == "changed SAM argument" {
										c.Methods[i].Signature = strings.Replace(c.Methods[i].Signature, "Ljava/util/function/Supplier<Ljava/lang/Object;>;", "Ljava/util/function/Supplier<Ljava/lang/String;>;", 1)
									} else if variant == "free constructor variable" {
										c.Methods[i].Signature = strings.Replace(c.Methods[i].Signature, "TV;", "TMissing;", 1)
									}
								}
							}
						}
						return c, known
					}
					switch variant {
					case "swapped factories":
						delegate.Arguments[2], delegate.Arguments[3] = factories[1], factories[0]
					case "swapped checks":
						checks[0], checks[1] = checks[1], checks[0]
					case "changed normal port":
						delegate.Arguments[0] = right
					case "wrong factory port":
						delegate.Arguments[1], delegate.Arguments[2] = factories[0], left
					case "changed receiver":
						factories[1].Captures[0] = left
					case "duplicate factory":
						delegate.Arguments[3] = factories[0]
					case "unsealed factory":
						factories[1].CapturesKnown = false
					case "wrong check receiver":
						checks[0].Object, checks[0].Arguments = right, []values.JavaValue{right}
					case "extra check":
						checks = append(checks, checks[0])
					case "foreign null check":
						checks[0].ClassName = "ForeignNullCheck"
					case "work":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "cancelled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					var body []statements.Statement
					for _, check := range checks {
						body = append(body, &statements.ExpressionStatement{Expression: check})
					}
					body = append(body, &statements.ExpressionStatement{Expression: delegate})
					got, accepted := d.constructorBoundReferencePortListSource(code, body, params, method)
					if accepted != (variant == "exact") || !bytes.Equal(before, code.Code) {
						t.Fatalf("accepted=%v; original Code immutable=%v", accepted, bytes.Equal(before, code.Code))
					}
					if !accepted && !reflect.DeepEqual(body, got) {
						t.Fatal("refused source list changed")
					}
					if accepted && (len(got) != 1 || len(body) != 3 || delegate.Arguments[2] != factories[0] || delegate.Arguments[3] != factories[1]) {
						t.Fatal("source transaction mutated its inputs")
					}
				})
			}
		})
	}
}

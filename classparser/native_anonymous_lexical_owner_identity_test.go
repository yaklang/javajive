package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousLexicalInvocationKeepsOriginalOwnerIdentity(t *testing.T) {
	for _, pkg := range []string{"", "lexical.binding"} {
		source, owner, namespace := packagedLexicalReceiverFixture, "LexicalOwner", "unnamed"
		if pkg != "" {
			source = "package " + pkg + ";\n" + source
			owner, namespace = strings.ReplaceAll(pkg, ".", "/")+"/"+owner, pkg
		}
		files := nativeCompileDebugClasses(t, source, "none")
		t.Run(namespace, func(t *testing.T) {
			for _, variant := range []string{"dotted IR owner", "internal owner", "foreign owner", "projected source owner", "wrong name", "wrong descriptor", "wrong kind", "extra argument", "missing origin", "wrong origin", "wrong method context", "wrong original opcode", "shadow declaration", "missing owned target", "opaque operand", "budget", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					root, err := Parse(files[owner+".class"])
					if err != nil {
						t.Fatal(err)
					}
					d := z.nativeMemberReader(root)
					p := d.planNativeMemberFamily()
					if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymousForest == nil {
						t.Fatal("original lexical forest")
					}
					forest := p.anonymousForest
					leaf := forest.objects[owner+"$1$1"]
					if leaf == nil {
						t.Fatal("original nested anonymous object")
					}
					var consumer *nativeAnonymousLexicalConsumer
					for _, entry := range forest.consumers[leaf.GetClassName()]["owner()Ljava/lang/Object;"] {
						consumer = entry
					}
					if consumer == nil || consumer.getter != nil || consumer.declaredField != nil {
						t.Fatal("original direct lexical invocation")
					}
					reader := z.nativeMemberReader(leaf)
					reader.nativeAnonymousForest, reader.nativeMemberRoot = forest, p
					ctx := &class_context.ClassContext{ClassName: leaf.GetClassName(), FunctionName: "owner", CurrentMethodDesc: "()Ljava/lang/Object;"}
					ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(leaf.GetClassName()))
					ref.IsThis, ref.IsParam = true, true
					ref.MarkOriginalParameter(0)
					ref.MarkOriginalReceiver()
					var path []*nativeMemberLexicalRead
					for read := consumer.read; read != nil; read = read.prior {
						path = append(path, read)
					}
					var operand values.JavaValue = ref
					for i := len(path) - 1; i >= 0; i-- {
						read := path[i]
						field := values.NewRefMember(operand, read.field, types.NewJavaClass(read.descriptor[1:len(read.descriptor)-1]))
						field.HasOriginPC, field.OriginPC = true, read.pc
						operand = field
					}
					call := &values.FunctionCallExpression{Object: operand, ClassName: strings.ReplaceAll(consumer.owner, "/", "."), FunctionName: consumer.name, Descriptor: consumer.descriptor, Kind: values.InvokeVirtual, HasOriginPC: true, OriginPC: consumer.pc}
					switch variant {
					case "internal owner":
						call.ClassName = consumer.owner
					case "foreign owner":
						call.ClassName += "$Different"
					case "projected source owner":
						call.ClassName = strings.ReplaceAll(call.ClassName, "$", ".")
					case "wrong name":
						call.FunctionName = "different"
					case "wrong descriptor":
						call.Descriptor = "()Ljava/lang/String;"
					case "wrong kind":
						call.Kind = values.InvokeSpecial
					case "extra argument":
						call.Arguments = []values.JavaValue{ref}
					case "missing origin":
						call.HasOriginPC = false
					case "wrong origin":
						call.OriginPC++
					case "wrong method context":
						ctx.CurrentMethodDesc = "()I"
					case "wrong original opcode":
						for _, method := range leaf.Methods {
							name, _ := sourceBridgeUTF8(leaf, method.NameIndex)
							if name == "owner" {
								for _, attr := range method.Attributes {
									if code, ok := attr.(*CodeAttribute); ok {
										code.Code[consumer.pc] = core.OP_INVOKEINTERFACE
									}
								}
							}
						}
					case "shadow declaration":
						pool := NewConstantPoolWithConstant(&leaf.ConstantPool)
						leaf.Methods = append(leaf.Methods, &MemberInfo{NameIndex: uint16(pool.AddUtf8Info(consumer.name)), DescriptorIndex: uint16(pool.AddUtf8Info(consumer.descriptor))})
					case "missing owned target":
						delete(forest.units, consumer.owner)
					case "opaque operand":
						call.Object = values.NewCustomValue(func(*class_context.ClassContext) string { panic("identity proof must not render") }, ref.Type)
					case "budget":
						reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						request, cancel := context.WithCancel(context.Background())
						cancel()
						reader.Work = workbudget.New(request, workbudget.Limits{})
					}
					reader.wireNativeAnonymousLexicalConsumers(ctx)
					receiver, known := ctx.SourceLexicalInvocationReceiver(call)
					want := variant == "dotted IR owner" || variant == "internal owner"
					if known != want || receiver != "" || want && reader.nativeCaptureFailed {
						t.Fatalf("unqualified=%v receiver=%q captureFailed=%v", known, receiver, reader.nativeCaptureFailed)
					}
				})
			}
		})
	}
}

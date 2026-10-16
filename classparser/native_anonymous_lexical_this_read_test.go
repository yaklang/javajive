package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousLexicalThisQueryRequiresOriginalDeclarationAndPath(t *testing.T) {
	owner, fixture := enclosingThisInitializerFixture("reference")
	files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, "none", "8")
	for _, role := range []string{"named", "anonymous"} {
		t.Run(role, func(t *testing.T) {
			for _, variant := range []string{"original", "missing witness", "missing binding", "wrong binding", "wrong PC", "wrong inner PC", "wrong field", "wrong result type", "foreign receiver", "mutable alias", "opaque", "cycle", "foreign context", "wrong method", "wrong descriptor", "copied object", "missing object", "missing owner", "missing anchor", "copied anchor", "missing declaration", "duplicate declaration", "wrong declaration descriptor", "mutable capture", "static capture", "nonsynthetic capture", "extra attribute", "missing lexical role", "detached certificate", "failed family", "foreign family", "foreign forest", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					root, err := Parse(append([]byte(nil), files[owner+".class"]...))
					if err != nil {
						t.Fatal(err)
					}
					d := z.nativeMemberReader(root)
					members := d.planNativeMemberFamily()
					if members == nil || !d.planNativeMemberAnonymousScopes(members) {
						t.Fatal("original mixed source forest")
					}
					forest := members.anonymousForest
					leaf := forest.units[owner+"$Stage$1$1"]
					key := "<init>" + leaf.descriptor
					var read *nativeMemberLexicalRead
					for pc, candidate := range forest.reads[leaf.object.GetClassName()][key] {
						if forest.lexicalThis[leaf.object.GetClassName()][key][pc] && (forest.units[candidate.owner] != nil) == (role == "anonymous") {
							read = candidate
						}
					}
					if read == nil || read.prior == nil {
						t.Fatal("original multi-hop qualified THIS read")
					}
					ctx := &class_context.ClassContext{ClassName: leaf.object.GetClassName(), FunctionName: "<init>", CurrentMethodDesc: leaf.descriptor}
					ctx.DeclarationSourceName = func(string) (string, bool) { t.Fatal("read-only query formatted source"); return "", false }
					base := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(leaf.object.GetClassName()))
					base.IsThis = true
					var path []*nativeMemberLexicalRead
					for node := read; node != nil; node = node.prior {
						path = append(path, node)
					}
					var operand values.JavaValue = base
					for i := len(path) - 1; i >= 0; i-- {
						node := path[i]
						field := values.NewRefMember(operand, node.field, types.NewJavaClass(node.descriptor[1:len(node.descriptor)-1]))
						field.HasOriginPC, field.OriginPC = true, node.pc
						operand = field
					}
					field := operand.(*values.RefMember)
					original := &values.JavaClassMember{Name: read.owner, Member: read.field, Description: read.descriptor}
					if variant != "missing witness" {
						field.MarkOriginalFieldRead(original, read.pc)
					}
					current := NewClassObjectDumper(leaf.object)
					current.nativeAnonymousForest = forest
					current.nativeAnonymousBindings = map[string]string{}
					declarationOwner := read.descriptor[1 : len(read.descriptor)-1]
					spelling, known := members.sourceName(declarationOwner)
					if !known {
						spelling = declarationOwner
					}
					binding := nativeMemberCaptureIndexKey(read.owner, read.field)
					current.nativeAnonymousBindings[binding] = spelling + ".this"
					object := forest.objects[read.owner]
					var capture *MemberInfo
					for _, f := range object.Fields {
						if name, _ := sourceBridgeUTF8(object, f.NameIndex); name == read.field {
							capture = f
						}
					}
					if capture == nil {
						t.Fatal("original capture declaration")
					}
					switch variant {
					case "missing binding":
						delete(current.nativeAnonymousBindings, binding)
					case "wrong binding":
						current.nativeAnonymousBindings[binding] = "Foreign.this"
					case "wrong PC":
						field.OriginPC++
					case "wrong inner PC":
						field.Object.(*values.RefMember).OriginPC++
					case "wrong field":
						field.Member = "other"
					case "wrong result type":
						field.JavaType = types.NewJavaClass("java.lang.String")
					case "foreign receiver":
						base.IsThis = false
					case "mutable alias":
						field.Object = values.NewJavaRef(&utils.VariableId{}, field.Object, field.Object.Type())
					case "opaque":
						prior := field.Object
						field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { t.Fatal("rendered opaque receiver"); return "" }, func() types.JavaType { return prior.Type() })
					case "cycle":
						field.Object.(*values.RefMember).Object = field
					case "foreign context":
						ctx.ClassName = "Foreign"
					case "wrong method":
						ctx.FunctionName = "other"
					case "wrong descriptor":
						ctx.CurrentMethodDesc = "()V"
					case "copied object":
						copy := *current.obj
						current.obj = &copy
					case "missing object":
						delete(forest.objects, current.obj.GetClassName())
					case "missing owner":
						delete(forest.objects, declarationOwner)
					case "missing anchor":
						delete(members.lexicalObjects, read.owner)
					case "copied anchor":
						copy := *object
						members.lexicalObjects[read.owner] = &copy
					case "missing declaration":
						object.Fields = nil
					case "duplicate declaration":
						object.Fields = append(object.Fields, capture)
					case "wrong declaration descriptor":
						capture.DescriptorIndex = sourceBridgePoolString(t, object, "Ljava/lang/String;")
					case "mutable capture":
						capture.AccessFlags &^= 0x10
					case "static capture":
						capture.AccessFlags |= 8
					case "nonsynthetic capture":
						capture.AccessFlags &^= 0x1000
					case "extra attribute":
						capture.Attributes = append(capture.Attributes, &ConstantValueAttribute{})
					case "missing lexical role":
						delete(forest.lexicalThis[current.obj.GetClassName()][key], read.pc)
					case "detached certificate":
						copy := *read
						read = &copy
					case "failed family":
						members.failed = true
					case "foreign family":
						members.owner = "Foreign"
					case "foreign forest":
						copy := *forest
						members.anonymousForest = &copy
					case "work":
						current.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "memory":
						current.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						cx, cancel := context.WithCancel(context.Background())
						cancel()
						current.Work = workbudget.New(cx, workbudget.Limits{})
					}
					want := variant == "original" || variant == "missing witness" || variant == "missing binding" || variant == "wrong binding"
					if got := current.nativeAnonymousLexicalThisRead(read, field, field.OriginPC, field.Member, ctx); got != want {
						t.Fatalf("declaration/type query=%v want=%v", got, want)
					}
					if current.nativeCaptureFailed || members.failed != (variant == "failed family") {
						t.Fatal("read-only query committed source failure")
					}
					if variant == "original" || variant == "missing witness" || variant == "missing binding" || variant == "wrong binding" {
						ctx.DeclarationSourceName = members.sourceName
						current.wireNativeAnonymousForestCaptures(ctx)
						if variant == "missing witness" && nativeAnonymousInitializerLexicalCaptureField(original, field, ctx) {
							t.Fatal("type evidence substituted for original GETFIELD witness")
						}
						if variant == "missing binding" || variant == "wrong binding" {
							if _, known := ctx.SourceLexicalCapturedField(field, field.OriginPC, field.Member); known || !current.nativeCaptureFailed {
								t.Fatal("type evidence substituted for source binding permission")
							}
						}
					}
				})
			}
		})
	}
}

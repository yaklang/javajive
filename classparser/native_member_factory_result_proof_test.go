package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberFactoryResultRequiresOriginalThisAllocation(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, lexicalPrivateDependentShadowFixture(), debug)
		for _, variant := range []string{"original", "no family", "failed family", "missing original owner", "foreign original owner", "missing child", "foreign child", "static child", "wrong child name", "wrong physical child flags", "copied method", "duplicate original method", "wrong original superclass", "static method", "missing signature", "foreign signature prefix", "method formal shadows", "free signature variable", "malformed signature", "mismatched signature throws", "duplicate signature", "nil signature", "missing code", "duplicate code", "wrong stack", "foreign capture slot", "null capture", "wrong return", "extra instruction", "wrong NEW", "wrong invocation", "type annotation", "budget", "memory", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				z := nativeArchive(t, files)
				defer z.Close()
				root, e := Parse(files["LexicalCallOwner.class"])
				if e != nil {
					t.Fatal(e)
				}
				p := z.nativeMemberReader(root).planNativeMemberFamily()
				if p == nil {
					t.Fatal("original complete source family")
				}
				obj := p.lexicalObjects["LexicalCallOwner$Mid$Value"]
				child := p.children["LexicalCallOwner$Mid$Value$Reader"]
				if obj == nil || child == nil {
					t.Fatal("missing original lexical member")
				}
				var method *MemberInfo
				var code *CodeAttribute
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "reader" {
						method = m
						for _, a := range m.Attributes {
							if v, ok := a.(*CodeAttribute); ok {
								code = v
							}
						}
					}
				}
				if method == nil || code == nil {
					t.Fatal("missing original factory")
				}
				sig, ok := nativeMethodLocalOriginalSignature(obj, method.Attributes, nil)
				if !ok {
					t.Fatal("original result signature")
				}
				ctx := &class_context.ClassContext{ClassName: strings.ReplaceAll(obj.GetClassName(), "/", "."), LexicalClassName: "Value", DeclarationSourceName: func(n string) (string, bool) { return p.sourceName(n) }}
				_, _, result := types.ParseMethodSignatureFull(sig, ctx)
				d := z.nativeMemberReader(obj)
				d.nativeMemberRoot = p
				d.nativeMemberCurrent = p.children[obj.GetClassName()]
				d.FuncCtx = ctx
				switch variant {
				case "no family":
					d.nativeMemberRoot = nil
				case "failed family":
					p.failed = true
				case "missing original owner":
					delete(p.lexicalObjects, obj.GetClassName())
				case "foreign original owner":
					other, e := Parse(files["LexicalCallOwner$Mid$Value.class"])
					if e != nil {
						t.Fatal(e)
					}
					p.lexicalObjects[obj.GetClassName()] = other
				case "missing child":
					delete(p.children, child.object.GetClassName())
				case "foreign child":
					other, e := Parse(files["LexicalCallOwner$Mid$Value$Reader.class"])
					if e != nil {
						t.Fatal(e)
					}
					p.lexicalObjects[child.object.GetClassName()] = other
				case "static child":
					child.static = true
				case "wrong child name":
					child.name = "OtherReader"
				case "wrong physical child flags":
					for _, a := range child.object.Attributes {
						if table, ok := a.(*InnerClassesAttribute); ok {
							for _, r := range table.Classes {
								n, _ := sourceBridgeClassName(child.object, r.InnerClassInfoIndex)
								if n == child.object.GetClassName() {
									r.InnerClassAccessFlags |= 8
								}
							}
						}
					}
				case "copied method":
					copy := *method
					method = &copy
				case "duplicate original method":
					obj.Methods = append(obj.Methods, method)
				case "wrong original superclass":
					nativeReplaceOriginalSignature(t, root, root.Attributes, "<T:Ljava/lang/Number;U:TT;>Ljava/util/ArrayList<TT;>;")
				case "static method":
					method.AccessFlags |= 8
				case "missing signature":
					var kept []AttributeInfo
					for _, a := range method.Attributes {
						if _, ok := a.(*SignatureAttribute); !ok {
							kept = append(kept, a)
						}
					}
					method.Attributes = kept
				case "foreign signature prefix":
					nativeReplaceOriginalSignature(t, obj, method.Attributes, strings.Replace(sig, "<TT;TU;>", "<Ljava/lang/Integer;Ljava/lang/Integer;>", 1))
				case "method formal shadows":
					nativeReplaceOriginalSignature(t, obj, method.Attributes, "<T:Ljava/lang/Number;>"+sig)
				case "free signature variable":
					nativeReplaceOriginalSignature(t, obj, method.Attributes, strings.Replace(sig, "Ljava/lang/String;", "TFree;", 1))
				case "malformed signature":
					nativeReplaceOriginalSignature(t, obj, method.Attributes, sig+"x")
				case "mismatched signature throws":
					nativeReplaceOriginalSignature(t, obj, method.Attributes, sig+"^Ljava/io/IOException;")
				case "duplicate signature":
					for _, a := range method.Attributes {
						if _, ok := a.(*SignatureAttribute); ok {
							method.Attributes = append(method.Attributes, a)
							break
						}
					}
				case "nil signature":
					method.Attributes = append(method.Attributes, (*SignatureAttribute)(nil))
				case "missing code":
					var kept []AttributeInfo
					for _, a := range method.Attributes {
						if _, ok := a.(*CodeAttribute); !ok {
							kept = append(kept, a)
						}
					}
					method.Attributes = kept
				case "duplicate code":
					method.Attributes = append(method.Attributes, code)
				case "wrong stack":
					code.MaxStack = 2
				case "foreign capture slot":
					code.Code[4] = byte(core.OP_ALOAD_1)
				case "null capture":
					code.Code[4] = byte(core.OP_ACONST_NULL)
				case "wrong return":
					code.Code[len(code.Code)-1] = byte(core.OP_ATHROW)
				case "extra instruction":
					code.Code = append([]byte{byte(core.OP_ACONST_NULL), byte(core.OP_POP)}, code.Code...)
				case "wrong NEW":
					index := nativeWideningTestPoolClass(t, obj, "OtherReader")
					code.Code[1], code.Code[2] = byte(index>>8), byte(index)
				case "wrong invocation":
					code.Code[5] = byte(core.OP_INVOKEVIRTUAL)
				case "type annotation":
					method.Attributes = append(method.Attributes, &RuntimeVisibleTypeAnnotationsAttribute{})
				case "budget":
					d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "memory":
					d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				case "canceled":
					c, cancel := context.WithCancel(context.Background())
					cancel()
					d.Work = workbudget.New(c, workbudget.Limits{})
				}
				source, known := d.nativeMemberImplicitFactoryResultSource(method, result)
				if known != (variant == "original") {
					t.Fatalf("admitted=%v source=%q", known, source)
				}
				if known && source != "Reader<String>" {
					t.Fatal(source)
				}
			})
		}
	}
}

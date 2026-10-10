package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeLexicalRawInvocationNeedsOriginalBinding(t *testing.T) {
	files := nativeCompileClasses(t, nativeLexicalRawReceiverFixture)
	for _, kind := range []string{"original", "no origin", "wrong PC", "wrong name", "wrong descriptor", "wrong class", "wrong caller", "static invoke", "special invoke", "interface invoke", "foreign receiver", "unknown declaration", "cyclic hierarchy", "interface owner", "nonpublic", "static target", "bridge target", "synthetic target", "own method formal", "foreign formal", "wrong erasure", "duplicate signature", "nil signature", "bad grammar", "generic throws", "parameter rewrite", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			caller, e := Parse(files["ScopedList$Cursor.class"])
			if e != nil {
				t.Fatal(e)
			}
			methodName, methodDesc := "<init>", "(LScopedList;I)V"
			var code *CodeAttribute
			for _, m := range caller.Methods {
				n, _ := sourceBridgeUTF8(caller, m.NameIndex)
				if n == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original constructor")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
			if e := decoder.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			pc := -1
			for _, op := range decoder.Opcodes() {
				if s := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL); s != nil && s.Member == "listIterator" {
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("original invoke")
			}
			call := &values.FunctionCallExpression{ClassName: "ScopedList", FunctionName: "listIterator", Descriptor: "(I)Ljava/util/ListIterator;", Kind: values.InvokeVirtual, OriginPC: pc, HasOriginPC: true}
			read := &nativeMemberLexicalRead{descriptor: "LScopedList;", parameterOwner: "ScopedList"}
			d := NewClassObjectDumper(caller)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			base := d.nativeAnnotationDeclarationResolver()
			resolve := func(name string) (*ClassObject, bool) {
				if kind == "unknown declaration" {
					return nil, false
				}
				obj, known := base(name)
				if !known {
					return nil, false
				}
				if name == "ScopedList" && kind == "cyclic hierarchy" {
					obj.SuperClass = obj.ThisClass
				}
				if name != "java/util/LinkedList" {
					return obj, true
				}
				if kind == "interface owner" {
					obj.AccessFlags |= 0x0200
				}
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
					if n != "listIterator" || desc != call.Descriptor {
						continue
					}
					switch kind {
					case "nonpublic":
						m.AccessFlags &^= 1
					case "static target":
						m.AccessFlags |= 8
					case "bridge target":
						m.AccessFlags |= 0x40
					case "synthetic target":
						m.AccessFlags |= 0x1000
					case "own method formal":
						nativeReplaceOriginalSignature(t, obj, m.Attributes, "<E:Ljava/lang/Object;>(I)Ljava/util/ListIterator<TE;>;")
					case "foreign formal":
						nativeReplaceOriginalSignature(t, obj, m.Attributes, "(I)Ljava/util/ListIterator<TUnknown;>;")
					case "wrong erasure":
						nativeReplaceOriginalSignature(t, obj, m.Attributes, "(I)Ljava/util/Iterator<TE;>;")
					case "duplicate signature":
						for _, a := range m.Attributes {
							if _, ok := a.(*SignatureAttribute); ok {
								m.Attributes = append(m.Attributes, a)
								break
							}
						}
					case "nil signature":
						m.Attributes = append(m.Attributes, (*SignatureAttribute)(nil))
					case "bad grammar":
						nativeReplaceOriginalSignature(t, obj, m.Attributes, "(I)Ljava/util/ListIterator<TE;>;x")
					case "generic throws":
						nativeReplaceOriginalSignature(t, obj, m.Attributes, "(I)Ljava/util/ListIterator<TE;>;^Ljava/io/IOException;")
					}
				}
				return obj, true
			}
			var work *workbudget.Budget
			switch kind {
			case "no origin":
				call.HasOriginPC = false
			case "wrong PC":
				call.OriginPC++
			case "wrong name":
				call.FunctionName = "get"
			case "wrong descriptor":
				call.Descriptor = "()Ljava/util/ListIterator;"
			case "wrong class":
				call.ClassName = "java.util.LinkedList"
			case "wrong caller":
				methodName = "notOriginal"
			case "static invoke":
				call.IsStatic = true
				call.Kind = values.InvokeStatic
			case "special invoke":
				call.IsSpecialInvoke = true
				call.Kind = values.InvokeSpecial
			case "interface invoke":
				call.Kind = values.InvokeInterface
			case "foreign receiver":
				read.descriptor = "Ljava/lang/Object;"
			case "parameter rewrite":
				code.Code[0] = 0x4c
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeLexicalRawInvocation(caller, methodName, methodDesc, call, read, resolve, work); got != (kind == "original") {
				t.Fatalf("%s admitted=%v", strings.ReplaceAll(kind, " ", "_"), got)
			}
		})
	}
}

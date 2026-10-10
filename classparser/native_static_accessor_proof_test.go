package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func TestNativeStaticAccessorReadRequiresOriginalPacket(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticAccessorReadFixture)
	for _, variant := range []string{"original", "not synthetic", "instance method", "wrong ordinal", "receiver argument", "void result", "wrong stack", "wrong locals", "instance opcode", "wrong return", "extra code", "handler", "instance field", "public field", "constant field", "duplicate code", "opaque metadata", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["StaticReadOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			var method *MemberInfo
			var code *CodeAttribute
			var field *MemberInfo
			for _, m := range obj.Methods {
				if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "access$000" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			for _, f := range obj.Fields {
				if n, _ := sourceBridgeUTF8(obj, f.NameIndex); n == "token" {
					field = f
				}
			}
			if method == nil || code == nil || field == nil {
				t.Fatal("original static getter")
			}
			var work *workbudget.Budget
			switch variant {
			case "not synthetic":
				method.AccessFlags &^= 0x1000
			case "instance method":
				method.AccessFlags &^= 8
			case "wrong ordinal":
				obj.ConstantPool[method.NameIndex-1].(*ConstantUtf8Info).Value = "access$001"
			case "receiver argument":
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = "(LStaticReadOwner;)Ljava/lang/Object;"
			case "void result":
				obj.ConstantPool[method.DescriptorIndex-1].(*ConstantUtf8Info).Value = "()V"
			case "wrong stack":
				code.MaxStack++
			case "wrong locals":
				code.MaxLocals++
			case "instance opcode":
				code.Code[0] = core.OP_GETFIELD
			case "wrong return":
				code.Code[len(code.Code)-1] = core.OP_LRETURN
			case "extra code":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "instance field":
				field.AccessFlags &^= 8
			case "public field":
				field.AccessFlags = (field.AccessFlags &^ 2) | 1
			case "constant field":
				field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "opaque metadata":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "Opaque", Info: []byte{0}})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeMemberPrivateGetterProof(obj, method, work)
			if (got != nil) != (variant == "original") {
				t.Fatalf("static packet admitted=%v", got != nil)
			}
			if got != nil && (!got.staticField || got.setter || got.call != nil) {
				t.Fatal("read classified as a receiver/write/call packet")
			}
		})
	}
}

func TestNativeStaticAccessorLexicalFieldRequiresCompleteScope(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticAccessorReadFixture)
	for _, variant := range []string{"original", "unproved bridge", "missing current", "identity mismatch", "nearer field", "missing owner", "ownership cycle", "unknown parent", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["StaticReadOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			c := z.nativeMemberReader(root)
			p := c.planNativeMemberFamily()
			if p == nil {
				t.Fatal("original family")
			}
			getter := p.getters[nativeMemberGetterKey("StaticReadOwner", "access$000", "()Ljava/lang/Object;")]
			if getter == nil {
				t.Fatal("original static bridge")
			}
			current := "StaticReadOwner$Reader"
			object := p.lexicalObjects[current]
			resolve := c.nativeAnnotationDeclarationResolver()
			var work *workbudget.Budget
			switch variant {
			case "unproved bridge":
				clone := *getter
				getter = &clone
			case "missing current":
				delete(p.lexicalObjects, current)
			case "identity mismatch":
				p.lexicalObjects[current] = root
			case "nearer field":
				for _, m := range object.Methods {
					if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == getter.field {
						object.Fields = append(object.Fields, &MemberInfo{NameIndex: m.NameIndex})
					}
				}
			case "missing owner":
				delete(p.lexicalObjects, getter.owner)
			case "ownership cycle":
				p.children[current].owner = current
			case "unknown parent":
				resolve = func(string) (*ClassObject, bool) { return nil, false }
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeStaticAccessorLexicalField(getter, current, p, resolve, work); got != (variant == "original") {
				t.Fatalf("lexical scope accepted=%v", got)
			}
		})
	}
}

func TestNativeStaticAccessorProjectionRequiresCurrentNamePhase(t *testing.T) {
	files := nativeCompileClasses(t, nativeStaticAccessorReadFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	root, e := Parse(files["StaticReadOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	p := z.nativeMemberReader(root).planNativeMemberFamily()
	if p == nil {
		t.Fatal("original family")
	}
	child := p.children["StaticReadOwner$Reader"]
	c := z.nativeMemberReader(child.object)
	c.FuncCtx = &class_context.ClassContext{ClassName: child.object.GetClassName(), FunctionName: "token", CurrentMethodDesc: "()Ljava/lang/Object;", TypeParams: []string{"StaticReadOwner"}}
	c.nativeMemberRoot = p
	c.nativeMemberCurrent = child
	c.wireNativeMemberPrivateGetters(p, c.FuncCtx)
	project := func() (string, bool) {
		return c.FuncCtx.SourcePrivateGetter("StaticReadOwner", "access$000", "()Ljava/lang/Object;", 0, nil, false)
	}
	if _, known := project(); known || p.failed {
		t.Fatal("provisional rendering committed a binding decision")
	}
	c.nativeSourceNamesReady = true
	if source, known := project(); !known || !strings.HasSuffix(source, "*/token)") {
		t.Fatalf("proved lexical field=%q known=%v", source, known)
	}
	c.nativeSourceNamesReady = false
	if _, known := project(); known || p.failed {
		t.Fatal("sibling method reused completed name bindings")
	}
	c.nativeSourceNamesReady = true
	c.nativeCaptureFailed = true
	if _, known := project(); known || !p.failed {
		t.Fatal("failed name graph licensed an unqualified field")
	}
}

package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousLegacySourceReopensInputFeatureDomain(t *testing.T) {
	files := nativeCompileJava56(t, nativeLegacyStaticCaptureFixture, "none", "5")
	for _, variant := range []string{"original", "owner future CP", "child future CP", "both versions changed", "owner version changed", "owner preview", "child preview", "wrong compiler", "wrong output", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			owner, err := Parse(files["LegacyCaptureOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := z.nativeMemberReader(owner)
			d.options.SourceCompiler = NativeJavac8
			d.options.TargetSourceVersion = 8
			p := d.planNativeAnonymousFamily()
			if p == nil {
				t.Fatal("original source plan")
			}
			child := p.children["LegacyCaptureOwner$1"]
			if child == nil || child.legacyInputMajor != 49 {
				t.Fatal("original legacy receipt")
			}
			d.nativeAnonymousRoot = p
			d.FuncCtx = &class_context.ClassContext{ClassName: "LegacyCaptureOwner", FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/String;)LLegacyCaptureBase;"}
			d.wireNativeAnonymousSource()
			switch variant {
			case "owner future CP":
				owner.ConstantPool = append(owner.ConstantPool, &ConstantMethodTypeInfo{})
			case "child future CP":
				child.object.ConstantPool = append(child.object.ConstantPool, &ConstantMethodHandleInfo{})
			case "both versions changed":
				owner.MajorVersion = 51
				child.object.MajorVersion = 51
			case "owner version changed":
				owner.MajorVersion = 50
			case "owner preview":
				owner.MinorVersion = 65535
			case "child preview":
				child.object.MinorVersion = 65535
			case "wrong compiler":
				d.options.SourceCompiler = ModernJavac
			case "wrong output":
				d.options.TargetSourceVersion = 11
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousLegacySourceFeatures(d, child); got != (variant == "original") {
				t.Fatalf("closed=%v", got)
			}
			if variant == "original" {
				return
			}
			args := []class_context.SourceCaptureOperand{{Local: true, Text: "kept"}}
			if _, ok := d.FuncCtx.SourceAnonymousAllocation(child.object.GetClassName(), child.descriptor, child.newPC, child.invokePC, args); ok || !p.failed {
				t.Fatal("changed legacy domain published anonymous source")
			}
		})
	}
}

func TestNativeAnonymousLegacyInputRequiresPhysicalFeatureDomain(t *testing.T) {
	files := nativeCompileJava56(t, nativeLegacyStaticCaptureFixture, "none", "5")
	for _, variant := range []string{"original", "major50", "pre49", "future version", "preview", "future handle CP", "future method type CP", "future dynamic CP", "nil CP hole", "typed nil CP", "missing wide tail", "nil method", "duplicate Code", "abstract Code", "missing Code", "old interface static", "old interface default", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["LegacyCaptureOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "read" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original body")
			}
			var work *workbudget.Budget
			switch variant {
			case "major50":
				obj.MajorVersion = 50
			case "pre49":
				obj.MajorVersion = 48
			case "future version":
				obj.MajorVersion = 51
			case "preview":
				obj.MinorVersion = 65535
			case "future handle CP":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{})
			case "future method type CP":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodTypeInfo{})
			case "future dynamic CP":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantInvokeDynamicInfo{})
			case "nil CP hole":
				obj.ConstantPool = append(obj.ConstantPool, nil)
			case "typed nil CP":
				obj.ConstantPool = append(obj.ConstantPool, (*ConstantClassInfo)(nil))
			case "missing wide tail":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantLongInfo{})
			case "nil method":
				obj.Methods = append(obj.Methods, nil)
			case "duplicate Code":
				for _, a := range method.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						method.Attributes = append(method.Attributes, c)
						break
					}
				}
			case "abstract Code":
				method.AccessFlags |= 0x0400
			case "missing Code":
				method.Attributes = nil
			case "old interface static", "old interface default":
				obj.AccessFlags = 0x0601
				obj.Methods = []*MemberInfo{method}
				method.AccessFlags = 1
				if variant == "old interface static" {
					method.AccessFlags = 9
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousLegacyInputFeatures(obj, work); got != (variant == "original" || variant == "major50") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

// These are CP/opcode admission models, not executable Java positives or a
// substitute for the constructor/body/source-consumption proofs downstream.
func TestNativeAnonymousLegacyCodeKeepsVersionedConstantCategories(t *testing.T) {
	files := nativeCompileJava56(t, nativeLegacyStaticCaptureFixture, "none", "5")
	for _, variant := range []string{"class literal", "wide class literal", "bad ldc category", "bad field category", "bad virtual category", "old interface static", "old interface special", "bad interface category", "bad class operand", "bad wide constant", "invokedynamic", "jsr", "wide jsr", "ret", "wide ret", "bad CP index", "truncated", "empty", "too large", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["LegacyCaptureOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			classIndex := obj.ThisClass
			var methodIndex uint16
			for i, cp := range obj.ConstantPool {
				if _, ok := cp.(*ConstantMethodrefInfo); ok {
					methodIndex = uint16(i + 1)
					break
				}
			}
			if methodIndex == 0 {
				t.Fatal("original method reference")
			}
			ref := obj.ConstantPool[methodIndex-1].(*ConstantMethodrefInfo)
			obj.ConstantPool = append(obj.ConstantPool, &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo})
			interfaceIndex := uint16(len(obj.ConstantPool))
			raw := []byte{core.OP_LDC_W, byte(classIndex >> 8), byte(classIndex), core.OP_POP, core.OP_RETURN}
			index := classIndex
			opcode := byte(core.OP_LDC_W)
			var work *workbudget.Budget
			switch variant {
			case "class literal":
				if classIndex > 255 {
					t.Fatal("small authored CP")
				}
				raw = []byte{core.OP_LDC, byte(classIndex), core.OP_POP, core.OP_RETURN}
			case "bad ldc category":
				index = methodIndex
			case "bad field category":
				opcode = core.OP_GETFIELD
				index = methodIndex
			case "bad virtual category":
				opcode = core.OP_INVOKEVIRTUAL
			case "old interface static":
				opcode = core.OP_INVOKESTATIC
				index = interfaceIndex
			case "old interface special":
				opcode = core.OP_INVOKESPECIAL
				index = interfaceIndex
			case "bad interface category":
				raw = []byte{core.OP_INVOKEINTERFACE, byte(methodIndex >> 8), byte(methodIndex), 1, 0, core.OP_RETURN}
			case "bad class operand":
				opcode = core.OP_NEW
				index = methodIndex
			case "bad wide constant":
				opcode = core.OP_LDC2_W
			case "invokedynamic":
				raw = []byte{core.OP_INVOKEDYNAMIC, 0, 1, 0, 0, core.OP_RETURN}
			case "jsr":
				raw = []byte{core.OP_JSR, 0, 3, core.OP_RETURN}
			case "wide jsr":
				raw = []byte{core.OP_JSR_W, 0, 0, 0, 5, core.OP_RETURN}
			case "ret":
				raw = []byte{core.OP_RET, 0, core.OP_RETURN}
			case "wide ret":
				raw = []byte{core.OP_WIDE, core.OP_RET, 0, 0, core.OP_RETURN}
			case "bad CP index":
				index = 65535
			case "truncated":
				raw = []byte{core.OP_LDC_W}
			case "empty":
				raw = nil
			case "too large":
				raw = make([]byte, 65536)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			switch variant {
			case "bad ldc category", "bad field category", "bad virtual category", "old interface static", "old interface special", "bad class operand", "bad wide constant", "bad CP index":
				raw = []byte{opcode, byte(index >> 8), byte(index), core.OP_RETURN}
			}
			if got := nativeAnonymousLegacyCodeFeatures(obj, &CodeAttribute{Code: raw}, work); got != (variant == "class literal" || variant == "wide class literal") {
				t.Fatalf("admitted=%v", got)
			}
		})
	}
}

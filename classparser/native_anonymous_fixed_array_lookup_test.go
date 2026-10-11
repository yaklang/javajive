package javaclassparser

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousInheritedVarargsRequiresExactArrayDeclaration(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ArrayVisitOwner.java": anonymousFixedArrayVarargsFixture}, "none", "8")
	for _, variant := range []string{"original varargs", "ordinary array", "no parameters", "nonarray last parameter", "array not last", "unknown superclass", "child override", "generic declaration", "return alternative", "bridge", "static", "interface ancestor", "budget"} {
		t.Run(variant, func(t *testing.T) {
			forest := &nativeAnonymousForest{objects: map[string]*ClassObject{}, units: map[string]*nativeAnonymousClass{}}
			for n, raw := range files {
				obj, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				forest.objects[strings.TrimSuffix(n, ".class")] = obj
			}
			caller, child, base := forest.objects["ArrayVisitOwner"], forest.objects["ArrayVisitOwner$1"], forest.objects["ArrayVisitBase"]
			forest.units[child.GetClassName()] = &nativeAnonymousClass{object: child}
			var target *MemberInfo
			for _, m := range base.Methods {
				if n, _ := sourceBridgeUTF8(base, m.NameIndex); n == "visit" {
					target = m
				}
			}
			var invoke *core.OpCode
			for _, m := range caller.Methods {
				if n, _ := sourceBridgeUTF8(caller, m.NameIndex); n != "use" {
					continue
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(caller.ConstantPool, i) })
						if err := d.ParseOpcode(); err != nil {
							t.Fatal(err)
						}
						for _, op := range d.Opcodes() {
							if s := constructorMotionMember(caller, op, core.OP_INVOKEVIRTUAL); s != nil && s.Member == "visit" {
								invoke = op
								if s.Name != child.GetClassName() {
									t.Fatal("authored invocation does not name the anonymous receiver")
								}
							}
						}
					}
				}
			}
			if target == nil || invoke == nil || target.AccessFlags&0x80 == 0 {
				t.Fatal("original varargs declaration/call missing")
			}
			var work *workbudget.Budget
			switch variant {
			case "ordinary array":
				target.AccessFlags &^= 0x80
			case "no parameters", "nonarray last parameter", "array not last":
				desc := map[string]string{"no parameters": "()V", "nonarray last parameter": "(Ljava/lang/Object;)V", "array not last": "([Ljava/lang/Object;I)V"}[variant]
				target.DescriptorIndex = uint16(NewConstantPoolWithConstant(&base.ConstantPool).AddUtf8Info(desc))
				ref := caller.ConstantPool[int(binary.BigEndian.Uint16(invoke.Data))-1].(*ConstantMethodrefInfo)
				nt := caller.ConstantPool[int(ref.NameAndTypeIndex)-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = uint16(NewConstantPoolWithConstant(&caller.ConstantPool).AddUtf8Info(desc))
			case "unknown superclass":
				delete(forest.objects, base.GetClassName())
			case "child override":
				copy := *target
				cp := NewConstantPoolWithConstant(&child.ConstantPool)
				copy.NameIndex = uint16(cp.AddUtf8Info("visit"))
				copy.DescriptorIndex = uint16(cp.AddUtf8Info("([Ljava/lang/Object;)V"))
				child.Methods = append(child.Methods, &copy)
			case "generic declaration":
				target.Attributes = append(target.Attributes, &SignatureAttribute{})
			case "return alternative":
				copy := *target
				copy.DescriptorIndex = uint16(NewConstantPoolWithConstant(&base.ConstantPool).AddUtf8Info("([Ljava/lang/Object;)Ljava/lang/Object;"))
				base.Methods = append(base.Methods, &copy)
			case "bridge":
				target.AccessFlags |= 0x40
			case "static":
				target.AccessFlags |= 8
			case "interface ancestor":
				base.AccessFlags |= 0x200
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			got := nativeAnonymousInheritedCall(forest, caller, invoke, work)
			if want := variant == "original varargs" || variant == "ordinary array"; got != want {
				t.Fatalf("lookup admitted=%t want=%t", got, want)
			}
		})
	}
}

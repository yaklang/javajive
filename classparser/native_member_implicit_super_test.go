package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeImplicitSuperRegenerationRequiresConsumedOriginalCaptureWitness(t *testing.T) {
	files := nativeCompileClasses(t, `class PlainParent{PlainParent(){owner();}Object owner(){return null;}}class PlainOwner{class Child extends PlainParent{Child(long n){super();}Object owner(){return PlainOwner.this;}}}`)
	for _, scenario := range []string{"original", "flat class", "missing family", "foreign family", "foreign object", "static member", "no consumed witness", "different witness", "wrong capture origin", "wrong delegate origin", "wrong delegate owner", "wrong descriptor", "leading instruction", "different capture receiver", "different capture parameter", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["PlainOwner$Child.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			root, err := Parse(append([]byte(nil), files["PlainOwner.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			child := nativeMemberProofWithOwner(obj, root, nil, d.buildInvocationMetadata())
			if child == nil {
				t.Fatal("original complete child proof")
			}
			d.nativeMemberCurrent = child
			d.nativeMemberRoot = &nativeMemberFamily{owner: child.owner, children: map[string]*nativeMemberClass{obj.GetClassName(): child}}
			var method *MemberInfo
			var code *CodeAttribute
			var witness *nativeMemberConstructor
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				if name != "<init>" {
					continue
				}
				method = m
				witness = child.constructors[desc]
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil || witness == nil || witness.capturePC < 0 {
				t.Fatal("original capture witness")
			}
			switch scenario {
			case "flat class":
				d.nativeMemberCurrent = nil
			case "missing family":
				d.nativeMemberRoot = nil
			case "foreign family":
				d.nativeMemberRoot.children = map[string]*nativeMemberClass{}
			case "foreign object":
				child.object = root
			case "static member":
				child.static = true
			case "no consumed witness":
				witness = nil
			case "different witness":
				copy := *witness
				witness = &copy
			case "wrong capture origin":
				witness.capturePC++
			case "wrong delegate origin":
				witness.delegatePC++
			case "wrong delegate owner":
				witness.delegateOwner = "java/lang/Object"
			case "wrong descriptor":
				witness.descriptor = "(LPlainOwner;I)V"
			case "leading instruction":
				code.Code = append([]byte{byte(core.OP_NOP)}, code.Code...)
			case "different capture receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "different capture parameter":
				code.Code[1] = byte(core.OP_ALOAD_0)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			decoder := core.NewDecompiler(code.Code, func(index int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, index) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			if got := d.nativeMemberImplicitSuperRegenerated(method, witness, decoder); got != (scenario == "original") {
				t.Fatalf("regeneration accepted=%v", got)
			}
		})
	}
}

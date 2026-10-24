package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeIndependentStaticHeaderRequiresOwnProvedBoundary(t *testing.T) {
	files := nativeCompileClasses(t, nativeIndependentLeafFixture)
	nativeIndependentLeafInput(t, files)
	for _, variant := range []string{"original", "no family", "ordinary family", "foreign source owner", "foreign object", "missing owned object", "imported physical outer", "nested render", "private original constructor", "assertion protocol", "missing original outer", "budget", "allocation", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for n, raw := range files {
				input[n] = append([]byte(nil), raw...)
			}
			z := nativeArchive(t, input)
			defer z.Close()
			obj, e := Parse(input["LeafNamespace$Leaf.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(obj)
			certificate := d.originalNativeMemberIndependentRoot()
			if certificate == nil {
				t.Fatal("original boundary")
			}
			p := d.planNativeMemberFamilyFromRoot(certificate)
			if p == nil {
				t.Fatal("original leaf plan")
			}
			d.nativeMemberRoot = p
			switch variant {
			case "no family":
				d.nativeMemberRoot = nil
			case "ordinary family":
				p.independentRoot = nil
			case "foreign source owner":
				p.owner = "ForeignScope"
			case "foreign object":
				d.obj, _ = Parse(input["LeafNamespace$Leaf.class"])
			case "missing owned object":
				delete(p.lexicalObjects, p.owner)
			case "imported physical outer":
				p.lexicalObjects[certificate.lexicalOwner], _ = Parse(input["LeafNamespace.class"])
			case "nested render":
				d.nativeMemberCurrent = certificate.declaration
			case "private original constructor":
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == "<init>" {
						m.AccessFlags |= 2
					}
				}
			case "assertion protocol":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				obj.Fields = append(obj.Fields, &MemberInfo{AccessFlags: 0x1018, NameIndex: uint16(cp.AddUtf8Info(nativeAssertionField)), DescriptorIndex: uint16(cp.AddUtf8Info("Z"))})
			case "missing original outer":
				d.foldSiblingResolver = func(string) ([]byte, bool) { return nil, false }
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "allocation":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.nativeMemberIndependentSourceHeader(); got != (variant == "original") {
				t.Fatalf("source header acquired foreign/unproved boundary: %v", got)
			}
		})
	}
}
func TestNativeIndependentStaticLeafDoesNotAdmitOrdinaryEmptyFamily(t *testing.T) {
	files := nativeCompileClasses(t, nativeIndependentLeafFixture)
	z := nativeArchive(t, files)
	defer z.Close()
	root, e := Parse(files["LeafEffects.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := z.nativeMemberReader(root)
	if d.planNativeMemberFamily() != nil || d.originalNativeMemberIndependentRoot() != nil {
		t.Fatal("ordinary empty root gained static-boundary ownership")
	}
}

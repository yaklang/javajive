package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestFlatAncestorAccessorNamesRequireSourceFlagCompatibility(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, nativeProtectedStaticFieldSources("ProtectedFieldOwner"), "none", "8")
	for _, variant := range []string{"distinct name", "different erased type", "different parameter type", "unrelated flat class", "retained invocation", "ordinary ancestor", "synthetic ancestor", "private ancestor", "bridge ancestor", "nil flat", "nil prepared", "nil getter", "nil method", "bad name CP", "bad descriptor CP", "malformed descriptor", "missing owner", "wrong owner identity", "unknown parent", "wrong parent identity", "cyclic hierarchy", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, _ := Parse(files["probe/access/ProtectedFieldOwner.class"])
			middle, _ := Parse(files["probe/base/FieldMiddle.class"])
			base, _ := Parse(files["probe/base/FieldParent.class"])
			archive := nativeArchive(t, files)
			defer archive.Close()
			d := archive.nativeMemberReader(root)
			var packet *MemberInfo
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if n == "access$000" {
					packet = m
				}
			}
			if packet == nil {
				t.Fatal("original static inherited accessor")
			}
			getter := nativeMemberPrivateAccessProofWithDeclarations(root, packet, d.nativeAnnotationDeclarationResolver(), nil, map[string]*ClassObject{root.GetClassName(): root})
			if getter == nil || !getter.inheritedField || !getter.staticField {
				t.Fatal("independent original packet proof")
			}
			family := &nativeMemberFamily{owner: root.GetClassName(), lexicalObjects: map[string]*ClassObject{root.GetClassName(): root}, getters: map[string]*nativeMemberPrivateGetter{"original": getter}}
			prepared := &nativeMemberPrepared{reader: d, root: root, family: family}
			classIndex := func(object *ClassObject, name string) uint16 {
				index := sourceBridgePoolString(t, object, name)
				object.ConstantPool = append(object.ConstantPool, &ConstantClassInfo{NameIndex: index})
				return uint16(len(object.ConstantPool))
			}
			copy := *packet
			copy.NameIndex = sourceBridgePoolString(t, middle, getter.name)
			copy.DescriptorIndex = sourceBridgePoolString(t, middle, getter.descriptor)
			copy.AccessFlags = 8 // the independently flat ordinary source view
			middle.Methods = append(middle.Methods, &copy)
			flat := middle
			switch variant {
			case "distinct name":
				copy.NameIndex = sourceBridgePoolString(t, middle, "otherName")
			case "different erased type":
				copy.DescriptorIndex = sourceBridgePoolString(t, middle, "()Ljava/lang/String;")
			case "different parameter type":
				copy.DescriptorIndex = sourceBridgePoolString(t, middle, "(Ljava/lang/Object;)Ljava/lang/Object;")
			case "unrelated flat class":
				flat = base
				flat.ThisClass = classIndex(flat, "unrelated/Other")
				copy.NameIndex = sourceBridgePoolString(t, flat, getter.name)
				copy.DescriptorIndex = sourceBridgePoolString(t, flat, getter.descriptor)
				flat.Methods = append(flat.Methods, &copy)
			case "retained invocation":
				family.getters = nil
				family.retainedAccessors = map[string]*nativeMemberPrivateGetter{"original": getter}
			case "synthetic ancestor":
				copy.AccessFlags |= 0x1000
			case "private ancestor":
				copy.AccessFlags |= 2
			case "bridge ancestor":
				copy.AccessFlags |= 0x1040
			case "nil flat":
				flat = nil
			case "nil prepared":
				prepared = nil
			case "nil getter":
				family.getters["original"] = nil
			case "nil method":
				middle.Methods = append(middle.Methods, nil)
			case "bad name CP":
				copy.NameIndex = 0
			case "bad descriptor CP":
				copy.DescriptorIndex = 0
			case "malformed descriptor":
				copy.DescriptorIndex = sourceBridgePoolString(t, middle, "()not-a-descriptor")
			case "missing owner":
				delete(family.lexicalObjects, getter.owner)
			case "wrong owner identity":
				family.lexicalObjects[getter.owner] = middle
			case "unknown parent":
				root.SuperClass = classIndex(root, "unknown/Missing")
			case "wrong parent identity":
				root.SuperClass = classIndex(root, "known/Intermediate")
				family.lexicalObjects[root.GetSupperClassName()] = base
			case "cyclic hierarchy":
				root.SuperClass = root.ThisClass
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "distinct name" || variant == "different erased type" || variant == "different parameter type" || variant == "unrelated flat class" || variant == "retained invocation"
			if got := nativeMemberFlatAncestorAccessorNamesClosed(prepared, flat); got != want {
				t.Fatalf("source hierarchy closure=%v want=%v", got, want)
			}
		})
	}
}

package javaclassparser

import "testing"

func TestNativeAccessorAncestorBridgeExemptionKeepsHierarchyEvidence(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"p/Base.java":    `package p;public class Base{protected static Object value;}`,
		"q/Program.java": `package q;interface Value<T>{T access$000();}class Parent extends p.Base implements Value<String>{public String access$000(){return "value";}}class Root extends Parent{public static class Reader{public Object read(){return Root.value;}}}`,
	}, "none", "8")
	for _, variant := range []string{"real bridge", "ordinary erased symbol", "synthetic nonbridge symbol", "bad bridge descriptor", "missing superclass after bridge", "cyclic superclass after bridge"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, raw := range files {
				o, e := Parse(append([]byte(nil), raw...))
				if e != nil {
					t.Fatal(n, e)
				}
				objects[o.GetClassName()] = o
			}
			parent := objects["q/Parent"]
			found := 0
			for _, method := range parent.Methods {
				if method.AccessFlags&0x0040 == 0 {
					continue
				}
				found++
				switch variant {
				case "ordinary erased symbol":
					method.AccessFlags &^= 0x1040
				case "synthetic nonbridge symbol":
					method.AccessFlags &^= 0x0040
				case "bad bridge descriptor":
					method.DescriptorIndex = parent.ThisClass
				}
			}
			if found != 1 {
				t.Fatalf("original generic bridge count=%d", found)
			}
			if variant == "missing superclass after bridge" {
				delete(objects, "p/Base")
			}
			if variant == "cyclic superclass after bridge" {
				parent.SuperClass = parent.ThisClass
			}
			matches, known := nativeMemberOriginalAccessorAncestors(objects["q/Root"], func(n string) (*ClassObject, bool) { o, k := objects[n]; return o, k }, nil)
			switch variant {
			case "real bridge":
				if !known || len(matches) != 0 {
					t.Fatalf("bridge manufactured ordinary-source conflict:%v %v", known, matches)
				}
			case "ordinary erased symbol", "synthetic nonbridge symbol":
				if !known || len(matches) != 1 || matches[0].object != parent || matches[0].descriptor != "()Ljava/lang/Object;" {
					t.Fatalf("nonbridge conflict lost:%v %v", known, matches)
				}
			default:
				if known {
					t.Fatal("bridge flag bypassed original hierarchy/name evidence")
				}
			}
		})
	}
}

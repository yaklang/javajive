package javaclassparser

import "testing"

// Read permission is not permission to synthesize an erased write. The original
// javac packets have identical field erasure but different storage obligations.
func TestNativeFixedFieldPacketsSeparateReadAndWrite(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "fixed"}[fixed], func(t *testing.T) {
			parent, formals, typ := "p.Base<E>", "<E>", "E"
			if fixed {
				parent, formals, typ = "p.Base<String>", "", "String"
			}
			files := nativeCompileSourceReleaseClasses(t, map[string]string{
				"p/Base.java":       `package p;public class Base<T>{protected T payload;}`,
				"q/FieldOwner.java": "package q;public class FieldOwner" + formals + " extends " + parent + "{public class Reader{public " + typ + " get(){return payload;}}public class Writer{public " + typ + " put(" + typ + " value){return payload=value;}}}",
			}, "none", "8")
			objects := map[string]*ClassObject{}
			for name, raw := range files {
				o, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				objects[name[:len(name)-6]] = o
			}
			root, owner := objects["q/FieldOwner"], objects["p/Base"]
			resolve := func(n string) (*ClassObject, bool) { o, ok := objects[n]; return o, ok }
			reads, writes := 0, 0
			for _, m := range root.Methods {
				n, _ := sourceBridgeUTF8(root, m.NameIndex)
				if m.AccessFlags != 0x1008 {
					continue
				}
				p := nativeMemberProtectedFieldProof(root, m, resolve, nil)
				if p != nil {
					reads++
					if p.method != m || p.setter || !p.genericField || !p.inheritedField || p.fieldDescriptor != "Ljava/lang/Object;" {
						t.Fatalf("lost original read packet %s", n)
					}
				}
				p = nativeMemberSetterPacketProof(root, m, resolve, nil)
				if p != nil {
					writes++
					if p.method != m || !p.setter || !p.genericField || !p.inheritedField {
						t.Fatalf("lost original write packet %s", n)
					}
				}
			}
			wantWrite := 1
			if fixed {
				wantWrite = 0
			}
			if reads != 1 || writes != wantWrite {
				t.Fatalf("read/write packets=%d/%d want1/%d", reads, writes, wantWrite)
			}
			// Unknown source uses must refuse even the legacy concrete fast path.
			for _, sig := range []bool{true, false} {
				if !sig {
					owner.Fields[0].Attributes = nil
				}
				for _, use := range []nativeInheritedFieldUse{0, 3, 255} {
					if _, known := nativeMemberInheritedFieldSourceSignature(root, owner, owner.Fields[0], "Ljava/lang/Object;", use, resolve, nil); known {
						t.Fatalf("unknown use %d accepted signature=%v", use, sig)
					}
				}
			}
		})
	}
}

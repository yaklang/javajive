package javaclassparser

import "testing"

// FieldSignature is a ReferenceTypeSignature, even when its erasure is checked
// through a method-parameter grammar. Bare JVM base types are not field Signatures.
func TestNativeInheritedFieldSignatureRequiresReferenceGrammar(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"p/Base.java": `package p;public class Base<T>{protected T payload;}`, "q/FieldOwner.java": `package q;public class FieldOwner<E> extends p.Base<E>{}`}, "none", "8")
	for _, row := range []struct {
		signature, descriptor string
		valid                 bool
	}{
		{"Z", "Z", false}, {"B", "B", false}, {"C", "C", false}, {"S", "S", false}, {"I", "I", false}, {"J", "J", false}, {"F", "F", false}, {"D", "D", false},
		{"[I", "[I", true}, {"Ljava/lang/Object;", "Ljava/lang/Object;", true}, {"TT;", "Ljava/lang/Object;", true},
	} {
		t.Run(row.signature, func(t *testing.T) {
			root, e := Parse(files["q/FieldOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			owner, e := Parse(files["p/Base.class"])
			if e != nil {
				t.Fatal(e)
			}
			field := owner.Fields[0]
			pool := NewConstantPoolWithConstant(&owner.ConstantPool)
			field.DescriptorIndex = uint16(pool.AddUtf8Info(row.descriptor))
			for _, a := range field.Attributes {
				if s, ok := a.(*SignatureAttribute); ok {
					s.SignatureIndex = uint16(pool.AddUtf8Info(row.signature))
				}
			}
			resolve := func(n string) (*ClassObject, bool) { return owner, n == owner.GetClassName() }
			generic, known := nativeMemberInheritedFieldSourceSignature(root, owner, field, row.descriptor, resolve, nil)
			if known != row.valid || known && !generic {
				t.Fatalf("field Signature %q erasure %q admitted=%v generic=%v want=%v", row.signature, row.descriptor, known, generic, row.valid)
			}
		})
	}
}

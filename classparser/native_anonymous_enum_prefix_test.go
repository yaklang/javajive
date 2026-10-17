package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeEnumAnonymousPrefixRequiresOriginalConstantRoles(t *testing.T) {
	fixture := methodLocalTypeConsumerShape("enum-body-prefix")
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MethodTypeOwner.java": fixture}, "none", "8")
	for _, scenario := range []string{"original", "missing parent", "missing synthesis", "missing body", "missing lexical object", "wrong body owner", "wrong descriptor", "wrong allocation ordinal", "wrong allocated type", "bad body flags", "bad constructor", "wrong constant name", "reversed constant order", "failed family", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			root, e := Parse(append([]byte(nil), files["MethodTypeOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil {
				t.Fatal("complete original enum and local ownership")
			}
			name := "MethodTypeOwner$Scope"
			parent := p.children[name]
			body := p.enumConstants[name+"$1"]
			if parent == nil || body == nil {
				t.Fatal("constant body fixture")
			}
			reader := z.nativeMemberReader(parent.object)
			switch scenario {
			case "missing parent":
				delete(p.children, name)
			case "missing synthesis":
				parent.enumSynthesis = nil
			case "missing body":
				delete(p.enumConstants, name+"$1")
			case "missing lexical object":
				delete(p.lexicalObjects, name+"$1")
			case "wrong body owner":
				body.owner = "Foreign"
			case "wrong descriptor":
				body.descriptor = "()V"
			case "wrong allocation ordinal":
				body.plan.ordinal++
			case "wrong allocated type":
				body.plan.allocatedClass = name + "$3"
			case "bad body flags":
				body.object.AccessFlags ^= 1
			case "bad constructor":
				for _, m := range body.object.Methods {
					n, _ := sourceBridgeUTF8(body.object, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								code.Code = []byte{0xb1}
							}
						}
					}
				}
			case "wrong constant name":
				for _, f := range parent.object.Fields {
					if f.AccessFlags&0x4000 != 0 {
						parent.object.ConstantPool = append(parent.object.ConstantPool, &ConstantUtf8Info{Value: "UNKNOWN"})
						f.NameIndex = uint16(len(parent.object.ConstantPool))
						break
					}
				}
			case "reversed constant order":
				parent.object.Fields[0], parent.object.Fields[1] = parent.object.Fields[1], parent.object.Fields[0]
			case "failed family":
				p.failed = true
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			prefix, known := reader.nativeAnonymousEnumConstantPrefix(p)
			if known != (scenario == "original") || known && prefix != 2 {
				t.Fatalf("prefix=%d known=%v", prefix, known)
			}
		})
	}
}
func TestNativeEnumAnonymousSourceOrdinalsKeepConstantPrefix(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"MethodTypeOwner.java": methodLocalTypeConsumerShape("enum-body-prefix")}, "none", "8")
	root, e := Parse(files["MethodTypeOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	d := z.nativeMemberReader(root)
	p := d.planNativeMemberFamily()
	if p == nil || !d.planNativeMemberAnonymousScopes(p) {
		t.Fatal("original enum and expression roles")
	}
	group := p.memberAnonymous["MethodTypeOwner$Scope"]
	if group == nil || group.enumPrefix != 2 || len(group.children) != 1 {
		t.Fatal("two constants and one ordinary anonymous expression")
	}
	for _, ordinal := range []int{1, 2, 3, 4} {
		source := fmt.Sprintf("/*jdec-owned-anonymous-ordinal:%d:%s*/new Token<String>(){}", ordinal, group.owner)
		if group.completeOwnSource(source) != (ordinal == 3) {
			t.Fatalf("source ordinal=%d", ordinal)
		}
	}
	if group.completeOwnSource("") {
		t.Fatal("missing expression must fail")
	}
}

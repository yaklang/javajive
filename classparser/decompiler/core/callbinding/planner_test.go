package callbinding

import (
	"strings"
	"testing"
)

func provider() Provider {
	m := map[string]Class{
		"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true},
		"java/lang/String": {Name: "java/lang/String", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true},
		"Base":             {Name: "Base", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []Method{{Name: "pick", Desc: "(Ljava/lang/Object;)I", Public: true}}},
		"Child":            {Name: "Child", Parents: []string{"Base"}, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []Method{{Name: "pick", Desc: "(Ljava/lang/String;)I", Public: true}}},
	}
	return func(n string) (Class, bool) { c, o := m[n]; return c, o }
}
func TestVirtualBinding(t *testing.T) {
	w := Witness{Owner: "Base", Name: "pick", Desc: "(Ljava/lang/Object;)I", Kind: Virtual}
	p := Build(w, "LChild;", []Argument{{Type: "Ljava/lang/String;"}}, provider())
	if !p.Supported || p.ReceiverType != "LBase;" || p.ArgumentTypes[0] != "Ljava/lang/Object;" {
		t.Fatalf("%+v", p)
	}
}
func TestMissingAncestorNotUnique(t *testing.T) {
	base := provider()
	p := func(n string) (Class, bool) {
		if n == "java/lang/Object" {
			return Class{}, false
		}
		return base(n)
	}
	f, e := FamilyOf(Witness{Owner: "Base", Name: "pick", Desc: "(Ljava/lang/Object;)I"}, p)
	if e != nil || f.Complete || f.Proof == Unique {
		t.Fatalf("%+v %v", f, e)
	}
}
func TestGenericRefused(t *testing.T) {
	base := provider()
	p := func(n string) (Class, bool) {
		c, o := base(n)
		if n == "Base" {
			c.Methods = append([]Method(nil), c.Methods...)
			c.Methods[0].Generic = true
		}
		return c, o
	}
	r := Build(Witness{Owner: "Base", Name: "pick", Desc: "(Ljava/lang/Object;)I"}, "LBase;", []Argument{{Type: "null"}}, p)
	if r.Supported {
		t.Fatal(r)
	}
}
func TestDescriptor(t *testing.T) {
	for _, s := range []string{"(Ljava/lang/Object;)I", "([I[[Ljava/lang/String;)V", "(JD)Ljava/lang/Object;"} {
		if _, _, e := Descriptor(s); e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range []string{"(V)V", "()Vjunk", "(L;)V", "([V)V", "(Ljava.lang.Object;)V", "(" + strings.Repeat("[", 256) + "I)V"} {
		if _, _, e := Descriptor(s); e == nil {
			t.Fatal(s)
		}
	}
}
func TestKindMatrixAndControl(t *testing.T) {
	for _, k := range []Kind{Virtual, Interface} {
		prov := provider()
		if k == Interface {
			base := prov
			prov = func(n string) (Class, bool) {
				c, ok := base(n)
				if n == "Base" {
					c.IsInterface = true
				}
				return c, ok
			}
		}
		p := Build(Witness{Owner: "Base", Name: "pick", Desc: "(Ljava/lang/Object;)I", Kind: k}, "LChild;", []Argument{{Type: "null"}}, prov)
		if !p.Supported {
			t.Fatalf("%d %+v", k, p)
		}
	}
	if Build(Witness{Owner: "Base", Name: "pick", Desc: "(Ljava/lang/Object;)I", Kind: Special}, "LBase;", []Argument{{Type: "null"}}, provider()).Supported {
		t.Fatal("special accepted")
	}
}
func TestHierarchyCycle(t *testing.T) {
	p := func(n string) (Class, bool) {
		return Class{Name: n, Parents: []string{n}, Public: true, MembersComplete: true, ParentsComplete: true}, true
	}
	if _, e := FamilyOf(Witness{Owner: "A", Name: "m", Desc: "()V"}, p); e == nil {
		t.Fatal("cycle accepted")
	}
}

// Package binding implements a deliberately bounded, public/non-generic,
// fixed-arity invocation planner. Unsupported is a result, never a successful proof.
package callbinding

import (
	"fmt"
	"sort"
	"strings"
)

type Kind uint8

const (
	Virtual Kind = iota
	Interface
	Static
	Special
	Dynamic
)

type Method struct {
	Name, Desc                               string
	Public, Static, Generic, Varargs, Bridge bool
}
type Class struct {
	Name                                     string
	Parents                                  []string
	Methods                                  []Method
	Public, MembersComplete, ParentsComplete bool
	IsInterface                              bool
}
type Provider func(string) (Class, bool)
type Witness struct {
	Owner, Name, Desc string
	Kind              Kind
	PC                int
}
type Proof uint8

const (
	Unknown Proof = iota
	Unique
	Compete
)

type Family struct {
	Proof    Proof
	Missing  []string
	Methods  []Method
	Target   *Method
	Complete bool
}

// FamilyOf distinguishes 'some metadata found' from 'the complete family is known'.
// Constructors/special calls and generic families need a separate resolver.
func FamilyOf(w Witness, p Provider) (Family, error) {
	f := Family{}
	if p == nil {
		return f, nil
	}
	want, _, err := Descriptor(w.Desc)
	if err != nil {
		return f, err
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	complete := true
	var walk func(string) error
	walk = func(n string) error {
		if visiting[n] {
			return fmt.Errorf("invalid hierarchy cycle at %s", n)
		}
		if done[n] {
			return nil
		}
		c, ok := p(n)
		if !ok {
			complete = false
			f.Missing = append(f.Missing, n)
			done[n] = true
			return nil
		}
		if c.Name != n {
			return fmt.Errorf("provider identity mismatch %s/%s", n, c.Name)
		}
		visiting[n] = true
		if !c.MembersComplete || !c.ParentsComplete {
			complete = false
			f.Missing = append(f.Missing, n+":incomplete")
		}
		for _, m := range c.Methods {
			if m.Name != w.Name {
				continue
			}
			ps, _, e := Descriptor(m.Desc)
			if e != nil {
				return e
			}
			if len(ps) != len(want) {
				continue
			}
			f.Methods = append(f.Methods, m)
			if m.Desc == w.Desc && f.Target == nil {
				mc := m
				f.Target = &mc
			}
		}
		parents := append([]string(nil), c.Parents...)
		sort.Strings(parents)
		for _, s := range parents {
			if e := walk(s); e != nil {
				return e
			}
		}
		visiting[n] = false
		done[n] = true
		return nil
	}
	if e := walk(w.Owner); e != nil {
		return f, e
	}
	sort.Strings(f.Missing)
	f.Complete = complete
	// Positive evidence of competitors is valid even with incomplete ancestors.
	compete := false
	for _, m := range f.Methods {
		ps, _, _ := Descriptor(m.Desc)
		if strings.Join(ps, ",") != strings.Join(want, ",") {
			compete = true
		}
	}
	if compete {
		f.Proof = Compete
	} else if complete && f.Target != nil {
		f.Proof = Unique
	}
	return f, nil
}

// Descriptor parses ordinary JVM descriptors without accepting trailing junk.
func Descriptor(s string) ([]string, string, error) {
	bad := func() ([]string, string, error) { return nil, "", fmt.Errorf("invalid method descriptor %q", s) }
	if len(s) < 3 || s[0] != '(' {
		return bad()
	}
	i := 1
	var ps []string
	take := func(allowVoid bool) (string, bool) {
		start := i
		dim := 0
		for i < len(s) && s[i] == '[' {
			dim++
			i++
		}
		if dim > 255 || i >= len(s) {
			return "", false
		}
		c := s[i]
		i++
		if c == 'L' {
			k := strings.IndexByte(s[i:], ';')
			if k <= 0 {
				return "", false
			}
			name := s[i : i+k]
			if strings.ContainsAny(name, ".[]();") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
				return "", false
			}
			i += k + 1
		} else if !strings.ContainsRune("BCDFIJSZ", rune(c)) && !(c == 'V' && allowVoid && dim == 0) {
			return "", false
		}
		return s[start:i], true
	}
	for i < len(s) && s[i] != ')' {
		p, ok := take(false)
		if !ok {
			return bad()
		}
		ps = append(ps, p)
	}
	if i >= len(s) || s[i] != ')' {
		return bad()
	}
	i++
	ret, ok := take(true)
	if !ok || i != len(s) {
		return bad()
	}
	slots := 0
	for _, p := range ps {
		slots++
		if p == "J" || p == "D" {
			slots++
		}
	}
	if slots > 255 {
		return bad()
	}
	return ps, ret, nil
}
func Reference(t string) bool { return strings.HasPrefix(t, "L") || strings.HasPrefix(t, "[") }
func Name(t string) string {
	if strings.HasPrefix(t, "L") && strings.HasSuffix(t, ";") {
		return t[1 : len(t)-1]
	}
	return t
}

// Assignable implements only identity, null, array->Object and metadata-proven
// reference widening. Unsupported array covariance is not guessed.
func Assignable(actual, formal string, p Provider) bool {
	if actual == formal {
		return true
	}
	if actual == "null" {
		return Reference(formal)
	}
	if !Reference(actual) || !Reference(formal) {
		return false
	}
	if formal == "Ljava/lang/Object;" {
		return true
	}
	if strings.HasPrefix(actual, "[") || strings.HasPrefix(formal, "[") {
		return false
	}
	if p == nil {
		return false
	}
	seen := map[string]bool{}
	q := []string{Name(actual)}
	for len(q) > 0 {
		n := q[0]
		q = q[1:]
		if n == Name(formal) {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		c, ok := p(n)
		if !ok {
			continue
		}
		q = append(q, c.Parents...)
	}
	return false
}

type Argument struct {
	Type string
	Poly bool
}
type Plan struct {
	Supported     bool
	Reason        string
	Family        Family
	ReceiverType  string
	ArgumentTypes []string
	KeepWitness   Witness
}

func Build(w Witness, receiverType string, args []Argument, p Provider) Plan {
	out := Plan{KeepWitness: w}
	reject := func(s string) Plan { out.Reason = s; return out }
	if w.Kind == Special || w.Kind == Dynamic || w.Name == "<init>" {
		return reject("dedicated special/bootstrap/constructor proof required")
	}
	ps, _, err := Descriptor(w.Desc)
	if err != nil {
		return reject(err.Error())
	}
	if len(ps) != len(args) {
		return reject("arity mismatch")
	}
	f, err := FamilyOf(w, p)
	out.Family = f
	if err != nil {
		return reject(err.Error())
	}
	if !f.Complete || f.Target == nil {
		return reject("overload_family_unknown")
	}
	c, ok := p(w.Owner)
	if !ok || !c.Public {
		return reject("owner not publicly denotable")
	}
	if (w.Kind == Interface && !c.IsInterface) || (w.Kind == Virtual && c.IsInterface) {
		return reject("symbolic owner kind mismatch")
	}
	for _, m := range f.Methods {
		if !m.Public || m.Generic || m.Varargs || m.Bridge {
			return reject("family requires generic/access/varargs/bridge proof")
		}
	}
	if f.Target.Static != (w.Kind == Static) {
		return reject("invoke kind disagrees with metadata")
	}
	if w.Kind != Static {
		recv := "L" + w.Owner + ";"
		if !Assignable(receiverType, recv, p) {
			return reject("receiver cast not proven widening")
		}
		out.ReceiverType = recv
	}
	for i, a := range args {
		if a.Poly {
			return reject("poly expression requires target-typing proof")
		}
		if !Assignable(a.Type, ps[i], p) {
			return reject("conversion not proven non-throwing; primitive narrowing is excluded")
		}
	}
	// All formal types must be source-denotable. Arrays are checked recursively.
	for _, t := range ps {
		for strings.HasPrefix(t, "[") {
			t = t[1:]
		}
		if strings.HasPrefix(t, "L") {
			c, ok := p(Name(t))
			if !ok || !c.Public {
				return reject("formal type not publicly denotable")
			}
		}
	}
	out.ArgumentTypes = append([]string(nil), ps...)
	out.Supported = true
	return out
}

package types

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"reflect"
	"strings"
	"testing"
)

func TestNestedOwnerArgumentsRequireOriginalSourceNesting(t *testing.T) {
	for _, row := range []struct{ signature, legacy, owned string }{
		{"Lp/Outer<TT;>.Inner;", "p.Outer$Inner<T>", "p.Outer<T>.Inner"},
		{"Lp/Outer<TT;>.Inner<TU;>;", "p.Outer$Inner<U>", "p.Outer<T>.Inner<U>"},
		{"Lp/Outer<TT;>.Middle<TU;>.Inner<TV;>;", "p.Outer$Middle$Inner<V>", "p.Outer<T>.Middle<U>.Inner<V>"},
		{"Lp/Outer<+TT;>.Inner<-TU;>;", "p.Outer$Inner<? super U>", "p.Outer<? extends T>.Inner<? super U>"},
		{"[Lp/Outer<TT;>.Inner;", "p.Outer$Inner<T>[]", "p.Outer<T>.Inner[]"},
		{"Lp/Outer$Literal<TT;>.Inner$Literal<TU;>;", "p.Outer$Literal$Inner$Literal<U>", "p.Outer$Literal<T>.Inner$Literal<U>"},
	} {
		t.Run(row.signature, func(t *testing.T) {
			v := ParseSignature(row.signature)
			if v == nil {
				t.Fatal("signature refused")
			}
			for _, proof := range []string{"absent", "original", "wrong parent"} {
				ctx := &class_context.ClassContext{TypeParams: []string{"T", "U", "V"}}
				ctx.DeclarationSourceName = func(n string) (string, bool) {
					if !strings.Contains(n, "$Inner") && !strings.Contains(n, "$Middle") {
						return "", false
					}
					if proof == "absent" {
						return "", false
					}
					source := strings.ReplaceAll(n, "$Middle", ".Middle")
					source = strings.ReplaceAll(source, "$Inner", ".Inner")
					if proof == "wrong parent" {
						source = strings.Replace(source, "p.Outer", "p.Wrong", 1)
					}
					return source, true
				}
				want := row.legacy
				if proof == "original" {
					want = row.owned
				}
				if proof != "wrong parent" {
					want = strings.TrimPrefix(want, "p.")
				}
				if proof == "wrong parent" {
					want = strings.ReplaceAll(strings.ReplaceAll(row.legacy, "$Middle", ".Middle"), "$Inner", ".Inner")
					want = strings.Replace(want, "p.Outer", "p.Wrong", 1)
				}
				if got := v.String(ctx); got != want {
					t.Fatalf("%s: %q != %q", proof, got, want)
				}
			}
		})
	}
}
func TestNestedOwnerSubstitutionKeepsEveryLexicalSegment(t *testing.T) {
	v := ParseSignature("Lp/Outer<TT;>.Inner<TU;>;")
	ctx := &class_context.ClassContext{DeclarationSourceName: func(n string) (string, bool) {
		if n == "p.Outer$Inner" {
			return "p.Outer.Inner", true
		}
		return "", false
	}}
	sub := SubstituteTypeVars(v, map[string]JavaType{"T": NewJavaClass("java.lang.String"), "U": NewJavaClass("java.lang.Integer")})
	if got := sub.String(ctx); got != "Outer<String>.Inner<Integer>" {
		t.Fatal(got)
	}
	if got := v.String(ctx); got != "Outer<T>.Inner<U>" {
		t.Fatalf("original signature mutated: %s", got)
	}
}

func TestOrdinaryParameterizedSignaturesKeepCanonicalRepresentation(t *testing.T) {
	original := ParseSignature("Lp/Box<TT;>;")
	want := NewParameterizedType("p.Box", []JavaType{NewJavaClass("T")})
	if !reflect.DeepEqual(original.RawType(), want.RawType()) {
		t.Fatal("ordinary signature acquired lexical owner metadata")
	}
	substituted := SubstituteTypeVars(original, map[string]JavaType{"T": NewJavaClass("java.lang.String")})
	expected := NewParameterizedType("p.Box", []JavaType{NewJavaClass("java.lang.String")})
	if !reflect.DeepEqual(substituted.RawType(), expected.RawType()) {
		t.Fatal("ordinary substitution changed the canonical type identity")
	}
}

func TestSignatureClassReferencesFollowTypeGrammar(t *testing.T) {
	for _, row := range []struct {
		signature string
		names     []string
	}{
		{"LDefault$Outer.Inner;", []string{"Default$Outer", "Default$Outer$Inner"}},
		{"(Lpkg/Outer$Child;[[I)Ljava/util/List;", []string{"pkg.Outer$Child", "java.util.List"}},
		{"<T$Dollar:Lpkg/Holder<Lpkg/Other<TT$Dollar;>.Child;>;>(TT$Dollar;)Lpkg/Result;^Ljava/io/IOException;", []string{"pkg.Holder", "pkg.Other", "pkg.Other$Child", "pkg.Result", "java.io.IOException"}},
		{"Ljava/util/List<+Lpkg/Outer<TT;>.Middle<-Lpkg/Other$Child;>.Child;>;", []string{"java.util.List", "pkg.Outer", "pkg.Outer$Middle", "pkg.Other$Child", "pkg.Outer$Middle$Child"}},
	} {
		got, ok := SignatureClassReferences(row.signature)
		if !ok || !reflect.DeepEqual(got, row.names) {
			t.Fatalf("%s: %v %v", row.signature, ok, got)
		}
	}
	for _, bad := range []string{"", "L;", "Lpkg/Outer<TT;>", "(I", "Lpkg/Outer..Child;", "<T:Lpkg/Outer;", strings.Repeat("[", 129) + "Lpkg/Outer$Child;", "Lpkg/Outer" + strings.Repeat(".Child", 129) + ";"} {
		if got, ok := SignatureClassReferences(bad); ok {
			t.Fatalf("invalid signature admitted %q: %v", bad, got)
		}
	}
}

func TestSignatureTypeVariablesIncludeBoundsWithoutGuessingClassNames(t *testing.T) {
	for _, row := range []struct {
		signature     string
		formals, refs []string
	}{
		{"<U:TT;:Ljava/lang/Comparable<TU;>;>(TU;)TU;^TX;", []string{"U"}, []string{"T", "U", "U", "U", "X"}},
		{"<T$Dollar:Ljava/lang/Number;>(TT$Dollar;)Ljava/util/List<+TT$Dollar;>;", []string{"T$Dollar"}, []string{"T$Dollar", "T$Dollar"}},
		{"Ljava/lang/Thread;", nil, nil},
		{"Lp/Outer<TT;>.Child<TU;>;", nil, []string{"T", "U"}},
	} {
		own, refs, ok := SignatureTypeVariableReferences(row.signature)
		if !ok || !reflect.DeepEqual(own, row.formals) || !reflect.DeepEqual(refs, row.refs) {
			t.Fatalf("%s: %v %v %v", row.signature, own, refs, ok)
		}
	}
	for _, sig := range []string{"<U:TT;U:TU;>Ljava/lang/Object;", "<U/Bad:Ljava/lang/Object;>Ljava/lang/Object;", "(TT;)TU", strings.Repeat("[", 129) + "TT;"} {
		if _, _, ok := SignatureTypeVariableReferences(sig); ok {
			t.Fatalf("malformed signature admitted %s", sig)
		}
	}
}

func TestNestedSelfTypeRequiresOriginalLexicalDeclaration(t *testing.T) {
	value := ParseSignature("Lp/Outer<TT;>.Child<TT;>;")
	for _, scenario := range []string{"owned self", "no lexical declaration", "different current class", "wrong member name", "wrong source owner"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := &class_context.ClassContext{ClassName: "p.Outer$Child", LexicalClassName: "Child", TypeParams: []string{"T"}, DeclarationSourceName: func(name string) (string, bool) {
				if name == "p.Outer$Child" {
					return "p.Outer.Child", true
				}
				return "", false
			}}
			switch scenario {
			case "no lexical declaration":
				ctx.LexicalClassName = ""
			case "different current class":
				ctx.ClassName = "p.Other$Child"
			case "wrong member name":
				ctx.LexicalClassName = "Other"
			case "wrong source owner":
				ctx.DeclarationSourceName = func(string) (string, bool) { return "p.Other.Child", true }
			}
			got := value.String(ctx)
			if scenario == "owned self" {
				if got != "Child<T>" {
					t.Fatal(got)
				}
			} else if got == "Child<T>" {
				t.Fatal("unproved implicit enclosing scope")
			}
		})
	}
}

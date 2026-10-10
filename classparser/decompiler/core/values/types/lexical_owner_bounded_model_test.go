package types

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// The independent model stores declaration IDs and bound edges, not parsed
// Signature names. It exhausts this finite grammar: two owner scopes followed
// by one method scope, each declaring zero or one of T/U; first bounds are
// Object/Number/CharSequence/T/U; seven parameter lists and six result shapes.
// This checks 55,902 models, not arbitrary Java or JVM semantic equivalence.
func TestLexicalOwnerErasureBoundedDeclarationModel(t *testing.T) {
	type declaration struct {
		concrete string
		edge     int
	}
	type option struct{ name, bound string }
	options := []option{{}}
	for _, n := range []string{"T", "U"} {
		for _, b := range []string{"Object", "Number", "CharSequence", "T", "U"} {
			options = append(options, option{n, b})
		}
	}
	parameters := [][]string{nil, {"T"}, {"U"}, {"Object"}, {"T", "U"}, {"U", "T"}, {"T[]", "long"}}
	results := []string{"T", "U", "Object", "T[]", "U[][]", "void"}
	primitive := map[string]string{"Object": "Ljava/lang/Object;", "Number": "Ljava/lang/Number;", "CharSequence": "Ljava/lang/CharSequence;", "long": "J", "void": "V"}
	spelling := func(n string) string {
		if d, ok := primitive[n]; ok {
			return d
		}
		return "T" + n + ";"
	}
	models, positive, refused := 0, 0, 0
	inventory := sha256.New()
	for _, outer := range options {
		for _, inner := range options {
			for _, method := range options {
				nodes := []declaration{}
				scope := map[string]int{}
				valid := true
				var bound func(int, map[int]bool) (string, bool)
				bound = func(id int, visiting map[int]bool) (string, bool) {
					if id < 0 || id >= len(nodes) || visiting[id] {
						return "", false
					}
					n := nodes[id]
					if n.concrete != "" {
						return n.concrete, true
					}
					visiting[id] = true
					result, ok := bound(n.edge, visiting)
					delete(visiting, id)
					return result, ok
				}
				extend := func(o option) string {
					if o.name == "" {
						return ""
					}
					id := len(nodes)
					nodes = append(nodes, declaration{edge: -1})
					scope[o.name] = id
					if d, ok := primitive[o.bound]; ok {
						nodes[id].concrete = d
					} else {
						target, ok := scope[o.bound]
						if ok {
							nodes[id].edge = target
						}
					}
					// The binding edge freezes the declaration's environment. Later namesakes
					// cannot change an outer dependent bound or repair an unknown earlier one.
					for i := range nodes {
						if _, ok := bound(i, map[int]bool{}); !ok {
							valid = false
						}
					}
					return "<" + o.name + ":" + spelling(o.bound) + ">"
				}
				classes := []string{extend(outer) + "Ljava/lang/Object;", extend(inner) + "Ljava/lang/Object;"}
				formal := extend(method)
				modelType := func(name string) (string, bool) {
					arrays := 0
					for strings.HasSuffix(name, "[]") {
						arrays++
						name = strings.TrimSuffix(name, "[]")
					}
					d, ok := primitive[name]
					if !ok {
						id, found := scope[name]
						if !found {
							return "", false
						}
						d, ok = bound(id, map[int]bool{})
					}
					if !ok || arrays > 0 && d == "V" {
						return "", false
					}
					return strings.Repeat("[", arrays) + d, true
				}
				signatureType := func(name string) string {
					arrays := 0
					for strings.HasSuffix(name, "[]") {
						arrays++
						name = strings.TrimSuffix(name, "[]")
					}
					return strings.Repeat("[", arrays) + spelling(name)
				}
				for _, params := range parameters {
					for _, result := range results {
						input := formal + "("
						expected := "("
						accept := valid
						for _, name := range params {
							input += signatureType(name)
							d, known := modelType(name)
							expected += d
							accept = accept && known && d != "V"
						}
						input += ")" + signatureType(result)
						d, known := modelType(result)
						expected += ")" + d
						accept = accept && known
						got, throws, ok := EraseLexicalOwnerMethodSignatureWithThrows(classes, input)
						if ok != accept || accept && (got != expected || len(throws) != 0) {
							t.Fatalf("model outer=%+v inner=%+v method=%+v params=%v result=%s: got=%q known=%v expected=%q valid=%v", outer, inner, method, params, result, got, ok, expected, accept)
						}
						fmt.Fprintf(inventory, "%q\t%q\t%t\t%q\n", classes, input, accept, expected)
						models++
						if accept {
							positive++
						} else {
							refused++
						}
					}
				}
			}
		}
	}
	if models != 55902 || positive < 1000 || refused < 1000 {
		t.Fatal(models, positive, refused)
	}
	t.Logf("finite grammar: models=%d valid=%d refused=%d; all declaration-ID oracle comparisons pass; inventory-sha256=%x", models, positive, refused, inventory.Sum(nil))
}

package class_context

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Transitive closure is an independent finite oracle for the conservative
// method-namespace proof. Models include joins, unreachable bad tables,
// missing/mismatched declarations and reachable cycles; they are not counted
// as executable Java positives or as additional debug/name scenarios.
func TestStaticImportScopeMatchesIndependentGraphClosureModel(t *testing.T) {
	const count = 9
	for sample := 0; sample < 2048; sample++ {
		f, decls := staticImportModelContext()
		state := uint64(sample+1) * 0x9e3779b97f4a7c15
		next := func() uint64 { state ^= state << 13; state ^= state >> 7; state ^= state << 17; return state }
		var edges, reach [count][count]bool
		var bad [count]bool
		for i := 0; i < count; i++ {
			for j := i + 1; j < count; j++ {
				edges[i][j] = next()%4 == 0
			}
		}
		if sample%5 == 0 {
			edges[count-1][0] = true
		}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("graph/N%d", i)
			c := callbinding.Class{Name: name, MembersComplete: true, ParentsComplete: true, IsInterface: true}
			for j := count - 1; j >= 0; j-- {
				if edges[i][j] {
					c.Parents = append(c.Parents, fmt.Sprintf("graph/N%d", j))
				}
			}
			switch next() % 19 {
			case 0:
				c.Methods = []callbinding.Method{{Name: "compute", Desc: "()V"}}
				bad[i] = true
			case 1:
				c.MembersComplete = false
				bad[i] = true
			case 2:
				c.ParentsComplete = false
				bad[i] = true
			case 3:
				c.Name = "graph/Wrong"
				bad[i] = true
			case 4:
				bad[i] = true
				continue
			default:
				c.Methods = []callbinding.Method{{Name: "unrelated", Desc: "(J)V"}}
			}
			decls[name] = c
		}
		reach = edges
		for k := 0; k < count; k++ {
			for i := 0; i < count; i++ {
				for j := 0; j < count; j++ {
					reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
				}
			}
		}
		want := true
		for i := 0; i < count; i++ {
			if (i == 0 || reach[0][i]) && (bad[i] || reach[i][i]) {
				want = false
			}
		}
		probe := decls["probe/Probe"]
		probe.Parents = []string{"graph/N0"}
		decls[probe.Name] = probe
		prefix := f.StaticInterfaceCallPrefix("a.Api", "compute", "(I)I")
		if got := prefix == "" && f.StaticMethodImports.Error() == nil; got != want {
			t.Fatalf("model=%d got=%t want=%t reachable=%v bad=%v prefix=%q refusal=%v", sample, got, want, reach[0], bad, prefix, f.StaticMethodImports.Error())
		}
	}
}

func staticImportModelContext() (*ClassContext, map[string]callbinding.Class) {
	declarations := map[string]callbinding.Class{
		"a/Api":            {Name: "a/Api", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "compute", Desc: "(I)I", Public: true, Static: true}, {Name: "compute", Desc: "(J)J", Public: true, Static: true}}},
		"probe/Probe":      {Name: "probe/Probe", MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}},
		"java/lang/Object": {Name: "java/lang/Object", MembersComplete: true, ParentsComplete: true},
	}
	f := &ClassContext{ClassName: "probe.Probe", PackageName: "probe", StaticMethodImports: NewStaticMethodImports(), SourceValueNameShadow: func(name string) bool { return name == "Api" || name == "a" }}
	f.InvocationMetadata = func(name string) (callbinding.Class, bool) { c, ok := declarations[name]; return c, ok }
	return f, declarations
}

func TestStaticInterfaceImportsRequireClosedMethodNamespace(t *testing.T) {
	for _, variant := range []string{"original", "ordinary method of other name", "own method different arity", "private own method", "ancestor method", "lexical outer method", "missing ancestor", "incomplete parents", "incomplete methods", "identity mismatch", "cycle", "missing target", "different descriptor", "nonstatic target", "nonpublic target", "duplicate target", "bridge target", "class target", "inaccessible owner", "owned lexical type", "different imported owner", "work budget", "memory budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			f, decls := staticImportModelContext()
			probe, api := decls["probe/Probe"], decls["a/Api"]
			switch variant {
			case "ordinary method of other name":
				probe.Methods = []callbinding.Method{{Name: "elsewhere", Desc: "()V"}}
			case "own method different arity":
				probe.Methods = []callbinding.Method{{Name: "compute", Desc: "()I", Public: true, Static: true}}
			case "private own method":
				probe.Methods = []callbinding.Method{{Name: "compute", Desc: "(I)I", Static: true}}
			case "ancestor method":
				root := decls["java/lang/Object"]
				root.Methods = []callbinding.Method{{Name: "compute", Desc: "(I)I"}}
				decls[root.Name] = root
			case "lexical outer method":
				outer := &ClassContext{ClassName: "probe.Outer", InvocationMetadata: f.InvocationMetadata}
				decls["probe/Outer"] = callbinding.Class{Name: "probe/Outer", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "compute", Desc: "()V"}}}
				f.SourceLexicalParent = outer
			case "missing ancestor":
				delete(decls, "java/lang/Object")
			case "incomplete parents":
				probe.ParentsComplete = false
			case "incomplete methods":
				probe.MembersComplete = false
			case "identity mismatch":
				probe.Name = "probe/Other"
			case "cycle":
				probe.Parents = []string{"probe/Probe"}
			case "missing target":
				api.Methods = nil
			case "different descriptor":
				api.Methods[0].Desc = "(Z)I"
			case "nonstatic target":
				api.Methods[0].Static = false
			case "nonpublic target":
				api.Methods[0].Public = false
			case "duplicate target":
				api.Methods = append(api.Methods, api.Methods[0])
			case "bridge target":
				api.Methods[0].Bridge = true
			case "class target":
				api.IsInterface = false
			case "inaccessible owner":
				api.Public = false
			case "owned lexical type":
				f.DeclarationSourceName = func(name string) (string, bool) { return "Probe.Api", name == "a.Api" }
			case "different imported owner":
				f.StaticMethodImports.owners["compute"] = "b.Api"
			case "work budget":
				f.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory budget":
				f.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				f.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			decls["probe/Probe"], decls["a/Api"] = probe, api
			prefix := f.StaticInterfaceCallPrefix("a.Api", "compute", "(I)I")
			want := variant == "original" || variant == "ordinary method of other name"
			if (prefix == "" && f.StaticMethodImports.Error() == nil) != want {
				t.Fatalf("prefix=%q error=%v wantProof=%t", prefix, f.StaticMethodImports.Error(), want)
			}
			if want {
				if strings.Join(f.StaticMethodImports.Imports(), ",") != "a.Api.compute" {
					t.Fatal(f.StaticMethodImports.Imports())
				}
				if next := f.StaticInterfaceCallPrefix("a.Api", "compute", "(J)J"); next != "" || f.StaticMethodImports.Error() != nil || len(f.StaticMethodImports.Imports()) != 1 {
					t.Fatal("repeated same-owner overload changed import transaction")
				}
			}
		})
	}
}

func TestStaticInterfaceImportRetryDoesNotPoisonOriginalTransaction(t *testing.T) {
	f, _ := staticImportModelContext()
	if f.StaticInterfaceCallPrefix("a.Api", "compute", "(I)I") != "" {
		t.Fatal("missing original proof")
	}
	clone := f.CloneForRetry()
	clone.StaticMethodImports.owners["compute"] = "b.Api"
	clone.StaticInterfaceCallPrefix("a.Api", "compute", "(I)I")
	if clone.StaticMethodImports.Error() == nil || f.StaticMethodImports.Error() != nil || strings.Join(f.StaticMethodImports.Imports(), ",") != "a.Api.compute" {
		t.Fatal("failed retry leaked imports or refusal into original")
	}
}

func TestStaticCallImportChecksBothTypeNamespacesAndOwnerKind(t *testing.T) {
	for _, isInterface := range []bool{false, true} {
		for _, variant := range []string{"method type parameters", "package type", "method conflict", "wrong owner kind", "missing owner", "value-only shadow", "owned type path"} {
			t.Run(fmt.Sprint(isInterface)+"/"+variant, func(t *testing.T) {
				f, declarations := staticImportModelContext()
				api := declarations["a/Api"]
				api.IsInterface = isInterface
				f.TypeParams = []string{"Api", "a"}
				f.FunctionName, f.CurrentMethodDesc = "m", "(I)I"
				switch variant {
				case "package type":
					f.TypeParams = []string{"Api"}
					f.LexicalTypeNames = map[string]bool{"a": true}
				case "method conflict":
					probe := declarations["probe/Probe"]
					probe.Methods = []callbinding.Method{{Name: "compute", Desc: "(J)J"}}
					declarations[probe.Name] = probe
				case "wrong owner kind":
					api.IsInterface = !isInterface
				case "value-only shadow":
					f.TypeParams = nil
				case "owned type path":
					f.TypeParams = nil
					f.SourceValueNameShadow = nil
					f.DeclarationSourceName = func(string) (string, bool) { return "Outer.Api", true }
					f.LexicalTypeNames = map[string]bool{"Outer": true}
				}
				declarations["a/Api"] = api
				if variant == "missing owner" {
					delete(declarations, "a/Api")
				}
				var prefix string
				if isInterface {
					prefix = f.StaticInterfaceCallPrefix("a.Api", "compute", "(I)I")
				} else {
					prefix = f.StaticClassCallPrefix("a.Api", "compute", "(I)I")
				}
				refused := variant == "method conflict" || variant == "wrong owner kind" || variant == "missing owner"
				if (f.StaticMethodImports.Error() != nil) != refused {
					t.Fatalf("prefix=%q refusal=%v", prefix, f.StaticMethodImports.Error())
				}
				if refused {
					if f.StaticMethodImports.FailedMethod() != "probe.Probe.m(I)I" || len(f.StaticMethodImports.Imports()) != 0 {
						t.Fatal("binding refusal must retain original caller and no invented import")
					}
					return
				}
				want := ""
				imports := "a.Api.compute"
				if variant == "owned type path" {
					want, imports = "Outer.Api.", ""
				} else if variant == "value-only shadow" && !isInterface {
					want, imports = "((Api)null).", ""
				}
				if prefix != want || strings.Join(f.StaticMethodImports.Imports(), ",") != imports {
					t.Fatalf("prefix=%q imports=%v want=%q/%q", prefix, f.StaticMethodImports.Imports(), want, imports)
				}
			})
		}
	}
}

func TestStaticOwnerWithoutMemberCannotCertifyAnObscuredCastType(t *testing.T) {
	f, _ := staticImportModelContext()
	f.TypeParams = []string{"Api", "a"}
	f.FunctionName, f.CurrentMethodDesc = "m", "()V"
	f.StaticClassOwner("a.Api")
	if f.StaticMethodImports.Error() == nil || f.StaticMethodImports.FailedMethod() != "probe.Probe.m()V" {
		t.Fatal("a typed-null primary still needs a denotable cast type")
	}
}

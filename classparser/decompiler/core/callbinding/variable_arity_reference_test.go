package callbinding

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func variableArityTestProvider(methods []Method) (Provider, map[string]Class) {
	classes := map[string]Class{}
	add := func(name string, parents ...string) {
		classes[name] = Class{Name: name, Public: true, MembersComplete: true, ParentsComplete: true, Parents: parents}
	}
	add("java/lang/Object")
	add("x/A", "java/lang/Object")
	add("x/B", "x/A")
	add("x/C", "java/lang/Object")
	add("java/lang/Number", "java/lang/Object")
	add("java/lang/Integer", "java/lang/Number")
	add("java/lang/Long", "java/lang/Number")
	add("x/Owner", "java/lang/Object")
	owner := classes["x/Owner"]
	owner.Methods = methods
	classes[owner.Name] = owner
	return func(name string) (Class, bool) { class, found := classes[name]; return class, found }, classes
}

func variableArityTestMethod(desc string, variable bool) Method {
	return Method{Name: "write", Desc: desc, Public: true, Varargs: variable, ExceptionsKnown: true}
}

func TestVariableArityReferenceUsesAllArityAndAllInvocationPhases(t *testing.T) {
	method := variableArityTestMethod
	for _, fixture := range []struct {
		name, target, sam string
		methods           []Method
		want              bool
	}{
		{"single reference pack", "([Lx/A;)V", "(Lx/A;)V", []Method{method("([Lx/A;)V", true)}, true},
		{"other fixed arity", "([Lx/A;)V", "(Lx/A;)V", []Method{method("([Lx/A;)V", true), method("(Lx/A;Lx/A;)V", false)}, true},
		{"unrelated component", "([Lx/A;)V", "(Lx/A;)V", []Method{method("([Lx/A;)V", true), method("([Lx/C;)V", true)}, true},
		{"more specific component", "([Lx/A;)V", "(Lx/B;)V", []Method{method("([Lx/A;)V", true), method("([Ljava/lang/Object;)V", true)}, true},
		{"original loses to narrower component", "([Ljava/lang/Object;)V", "(Lx/B;)V", []Method{method("([Lx/A;)V", true), method("([Ljava/lang/Object;)V", true)}, false},
		{"fixed widening beats pack", "([Lx/A;)V", "(Lx/A;)V", []Method{method("([Lx/A;)V", true), method("(Ljava/lang/Object;)V", false)}, false},
		{"existing array is fixed arity", "([Lx/A;)V", "([Lx/B;)V", []Method{method("([Lx/A;)V", true)}, false},
		{"primitive widening phase", "([I)V", "(I)V", []Method{method("([I)V", true), method("(J)V", false)}, false},
		{"boxing phase", "([I)V", "(I)V", []Method{method("([I)V", true), method("(Ljava/lang/Integer;)V", false)}, false},
		{"unboxing phase", "([Ljava/lang/Integer;)V", "(Ljava/lang/Integer;)V", []Method{method("([Ljava/lang/Integer;)V", true), method("(J)V", false)}, false},
		{"zero pack specificity", "([Lx/A;)V", "()V", []Method{method("([Lx/A;)V", true), method("([Ljava/lang/Object;)V", true)}, true},
		{"zero tail must also be specific", "(Lx/A;[Ljava/lang/Object;)V", "(Lx/A;)V", []Method{method("(Lx/A;[Ljava/lang/Object;)V", true), method("(Ljava/lang/Object;[Lx/A;)V", true)}, false},
		{"different declared varargs arities", "(Lx/A;[Lx/A;)V", "(Lx/A;Lx/A;)V", []Method{method("(Lx/A;[Lx/A;)V", true), method("([Ljava/lang/Object;)V", true)}, true},
		{"boxing within pack", "([Ljava/lang/Object;)V", "(I)V", []Method{method("([Ljava/lang/Object;)V", true)}, true},
		{"unboxing within pack", "([I)V", "(Ljava/lang/Integer;)V", []Method{method("([I)V", true)}, true},
		{"primitive array is one Object", "([Ljava/lang/Object;)V", "([I)V", []Method{method("([Ljava/lang/Object;)V", true)}, true},
		{"primitive arrays do not numerically widen", "([[J)V", "([I)V", []Method{method("([[J)V", true)}, false},
		{"recursive array covariance", "([[Lx/A;)V", "([Lx/B;)V", []Method{method("([[Lx/A;)V", true)}, true},
		{"covariant array fixed phase wins", "([[Lx/A;)V", "([Lx/B;)V", []Method{method("([[Lx/A;)V", true), method("([Ljava/lang/Object;)V", true)}, false},
		{"boolean pack", "([Z)V", "(Z)V", []Method{method("([Z)V", true)}, true},
		{"unknown original target", "([Lx/C;)V", "(Lx/A;)V", []Method{method("([Lx/A;)V", true)}, false},
		{"target is not varargs", "(Lx/A;)V", "(Lx/A;)V", []Method{method("(Lx/A;)V", false)}, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			provider, _ := variableArityTestProvider(fixture.methods)
			plan := SelectVariableArityReference(Witness{Owner: "x/Owner", Name: "write", Desc: fixture.target, Kind: Virtual}, fixture.sam, provider, nil)
			if plan.Supported != fixture.want || plan.Supported && plan.Selected.Desc != fixture.target {
				t.Fatalf("binding=%+v want supported=%v", plan, fixture.want)
			}
		})
	}
}

func TestVariableArityReferenceRequiresCompleteNegativeAndBindingEvidence(t *testing.T) {
	for _, variant := range []string{"original", "unknown ancestry", "partial ancestry", "partial member table", "missing parent methods", "wrong provider identity", "inherited target", "inherited duplicate", "cycle", "generic candidate", "inapplicable generic arity", "static candidate", "inapplicable static arity", "private candidate", "bridge candidate", "wrong invoke kind", "bad descriptor", "bad varargs flag", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			target := "([Lx/A;)V"
			provider, classes := variableArityTestProvider([]Method{variableArityTestMethod(target, true), variableArityTestMethod("([Lx/C;)V", true)})
			owner := classes["x/Owner"]
			w := Witness{Owner: owner.Name, Name: "write", Desc: target, Kind: Virtual}
			var work *workbudget.Budget
			want := variant == "original" || variant == "inherited target" || variant == "inapplicable generic arity" || variant == "inapplicable static arity"
			switch variant {
			case "unknown ancestry":
				delete(classes, "x/A")
			case "partial ancestry":
				a := classes["x/A"]
				a.ParentsComplete = false
				classes[a.Name] = a
			case "partial member table":
				owner.MembersComplete = false
			case "missing parent methods":
				delete(classes, "java/lang/Object")
			case "wrong provider identity":
				owner.Name = "different/Owner"
			case "inherited target", "inherited duplicate":
				classes["x/Base"] = Class{Name: "x/Base", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []Method{owner.Methods[0]}}
				owner.Parents = []string{"x/Base"}
				if variant == "inherited target" {
					owner.Methods = owner.Methods[1:]
				}
			case "cycle":
				owner.Parents = []string{owner.Name}
			case "generic candidate", "static candidate", "private candidate", "bridge candidate", "inapplicable generic arity", "inapplicable static arity":
				other := variableArityTestMethod("(Lx/A;)V", false)
				switch variant {
				case "generic candidate", "inapplicable generic arity":
					other.Generic = true
				case "static candidate", "inapplicable static arity":
					other.Static = true
				case "private candidate":
					other.Public = false
				case "bridge candidate":
					other.Bridge = true
				}
				if variant == "inapplicable generic arity" || variant == "inapplicable static arity" {
					other.Desc = "(Lx/A;Lx/A;)V"
				}
				owner.Methods = append(owner.Methods, other)
			case "wrong invoke kind":
				w.Kind = Interface
			case "bad descriptor":
				owner.Methods[0].Desc = "([V)V"
			case "bad varargs flag":
				owner.Methods[0].Desc = "(Lx/A;)V"
				w.Desc = owner.Methods[0].Desc
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			classes["x/Owner"] = owner
			plan := SelectVariableArityReference(w, "(Lx/A;)V", provider, work)
			if plan.Supported != want {
				t.Fatalf("binding=%+v want supported=%v", plan, want)
			}
			if plan.Supported {
				wantOwner := "x/Owner"
				if variant == "inherited target" {
					wantOwner = "x/Base"
				}
				if plan.DeclaringOwner != wantOwner {
					t.Fatalf("lost selected declaration identity: %s want %s", plan.DeclaringOwner, wantOwner)
				}
			}
		})
	}
}

// A chain has an independent closed-form most-specific member: the greatest
// declared depth not exceeding the argument's depth. Source method/parent
// order is randomized; an arbitrary remembered original invocation must never
// substitute for that selection. These are 1,000 independent generated worlds,
// not 1,000 renames/debug variants of one fixed source program.
func TestVariableArityReferenceMatchesIndependentHierarchyDepthModel(t *testing.T) {
	random := rand.New(rand.NewSource(7336))
	for world := 0; world < 1000; world++ {
		depth := 2 + random.Intn(10)
		arg := random.Intn(depth)
		classes := map[string]Class{"java/lang/Object": {Name: "java/lang/Object", Public: true, ParentsComplete: true, MembersComplete: true}}
		var methods []Method
		best := -1
		for i := 0; i < depth; i++ {
			name, parent := fmt.Sprintf("model/T%d", i), "java/lang/Object"
			if i != 0 {
				parent = fmt.Sprintf("model/T%d", i-1)
			}
			classes[name] = Class{Name: name, Public: true, Parents: []string{parent}, ParentsComplete: true, MembersComplete: true}
			if random.Intn(2) == 0 || i == 0 {
				methods = append(methods, variableArityTestMethod("([L"+name+";)V", true))
				if i <= arg {
					best = i
				}
			}
		}
		random.Shuffle(len(methods), func(i, j int) { methods[i], methods[j] = methods[j], methods[i] })
		classes["model/Owner"] = Class{Name: "model/Owner", Public: true, Parents: []string{"java/lang/Object"}, ParentsComplete: true, MembersComplete: true, Methods: methods}
		provider := func(name string) (Class, bool) { c, known := classes[name]; return c, known }
		if best < 0 {
			t.Fatal("generator lacks a known applicable declaration")
		}
		want := fmt.Sprintf("([Lmodel/T%d;)V", best)
		for _, candidate := range methods {
			plan := SelectVariableArityReference(Witness{Owner: "model/Owner", Name: "write", Desc: candidate.Desc, Kind: Virtual}, fmt.Sprintf("(Lmodel/T%d;)V", arg), provider, nil)
			if plan.Supported != (candidate.Desc == want) {
				t.Fatalf("world=%d depth=%d argument=%d original=%s want=%s plan=%+v", world, depth, arg, candidate.Desc, want, plan)
			}
		}
	}
}

// Expected physical ports come from an independent closed-form invocation
// model, while the real compiler family tests check actual adapter descriptors.
func TestVariableArityReferenceReturnsProvedExpandedInputPorts(t *testing.T) {
	for _, tc := range []struct {
		name, target, sam string
		ports             []string
	}{
		{"reference widening", "([Lx/A;)V", "(Lx/B;)V", []string{"Lx/A;"}},
		{"primitive widening", "([D)V", "(I)V", []string{"D"}},
		{"boxing", "([Ljava/lang/Object;)V", "(I)V", []string{"Ljava/lang/Object;"}},
		{"unboxing", "([I)V", "(Ljava/lang/Integer;)V", []string{"I"}},
		{"unboxing widening", "([J)V", "(Ljava/lang/Byte;)V", []string{"J"}},
		{"fixed and two component ports", "(J[Ljava/lang/Object;)V", "(JLjava/lang/String;I)V", []string{"J", "Ljava/lang/Object;", "Ljava/lang/Object;"}},
		{"empty tail", "(J[I)V", "(J)V", []string{"J"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, _ := variableArityTestProvider([]Method{variableArityTestMethod(tc.target, true)})
			plan := SelectVariableArityReference(Witness{Owner: "x/Owner", Name: "write", Desc: tc.target, Kind: Virtual}, tc.sam, provider, nil)
			if !plan.Supported || !slices.Equal(plan.ExpandedParameters, tc.ports) {
				t.Fatalf("proved ports=%+v want %v", plan, tc.ports)
			}
		})
	}
}

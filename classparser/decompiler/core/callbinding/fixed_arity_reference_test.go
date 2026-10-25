package callbinding

import (
	"context"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestFixedReferenceSelectionPreservesPhaseAndPhysicalPorts(t *testing.T) {
	for _, row := range []struct {
		name, sam, target string
		declarations      []string
		accepted          bool
	}{
		{"unbox int", "(Ljava/lang/Integer;)V", "(I)V", []string{"(I)V"}, true},
		{"unbox wide", "(Ljava/lang/Long;)V", "(J)V", []string{"(J)V", "(I)V"}, true},
		{"no numeric narrowing", "(Ljava/lang/Long;)V", "(I)V", []string{"(I)V"}, false},
		{"unbox then widen", "(Ljava/lang/Integer;)V", "(J)V", []string{"(J)V"}, true},
		{"strict reference wins over unboxing", "(Ljava/lang/Integer;)V", "(I)V", []string{"(I)V", "(Ljava/lang/Number;)V"}, false},
		{"strict chosen", "(Ljava/lang/Integer;)V", "(Ljava/lang/Number;)V", []string{"(I)V", "(Ljava/lang/Number;)V"}, true},
		{"narrower loose port wins", "(Ljava/lang/Integer;)V", "(I)V", []string{"(I)V", "(J)V"}, true},
		{"wider loose loses", "(Ljava/lang/Integer;)V", "(J)V", []string{"(I)V", "(J)V"}, false},
		{"primitive strict beats boxing", "(I)V", "(J)V", []string{"(J)V", "(Ljava/lang/Integer;)V"}, true},
		{"boxing to reference", "(I)V", "(Ljava/lang/Number;)V", []string{"(Ljava/lang/Number;)V"}, true},
		{"reference specificity", "(Lx/B;)V", "(Lx/A;)V", []string{"(Lx/A;)V", "(Ljava/lang/Object;)V"}, true},
		{"array covariance", "([Lx/B;)V", "([Lx/A;)V", []string{"([Lx/A;)V", "(Ljava/lang/Object;)V"}, true},
		{"primitive array does not widen", "([I)V", "([J)V", []string{"([J)V"}, false},
		{"other arity irrelevant", "(Ljava/lang/Integer;)V", "(I)V", []string{"(I)V", "(II)V"}, true},
		{"cross parameter ambiguity", "(Lx/B;Lx/B;)V", "(Lx/A;Ljava/lang/Object;)V", []string{"(Lx/A;Ljava/lang/Object;)V", "(Ljava/lang/Object;Lx/A;)V"}, false},
		{"zero arguments", "()V", "()V", []string{"()V", "(I)V"}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				var methods []Method
				for _, desc := range row.declarations {
					methods = append(methods, variableArityTestMethod(desc, false))
				}
				if reverse {
					slices.Reverse(methods)
				}
				provider, _ := variableArityTestProvider(methods)
				plan := SelectFixedArityReference(Witness{Owner: "x/Owner", Name: "write", Desc: row.target, Kind: Virtual}, row.sam, provider, nil)
				ports, _, _ := Descriptor(row.target)
				if plan.Supported != row.accepted || plan.Supported && (!slices.Equal(plan.ExpandedParameters, ports) || plan.Selected.Desc != row.target) {
					t.Fatalf("selection=%+v, accepted=%v", plan, row.accepted)
				}
			}
		})
	}
}

func TestFixedReferenceRequiresCompleteNegativeEvidence(t *testing.T) {
	for _, variant := range []string{"control", "incomplete table", "missing ancestor", "cycle", "unknown conversion", "duplicate", "generic rival", "static rival", "inaccessible rival", "array varargs", "packing only", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			target, sam := "(I)V", "(Ljava/lang/Integer;)V"
			provider, classes := variableArityTestProvider([]Method{variableArityTestMethod(target, false)})
			owner := classes["x/Owner"]
			var work *workbudget.Budget
			switch variant {
			case "incomplete table":
				owner.MembersComplete = false
			case "missing ancestor":
				delete(classes, "java/lang/Object")
			case "cycle":
				owner.Parents = []string{owner.Name}
			case "unknown conversion":
				owner.Methods = append(owner.Methods, variableArityTestMethod("(Lunknown/Type;)V", false))
				delete(classes, "java/lang/Integer")
			case "duplicate":
				owner.Methods = append(owner.Methods, owner.Methods[0])
			case "generic rival", "static rival", "inaccessible rival":
				rival := variableArityTestMethod("(Ljava/lang/Integer;)V", false)
				rival.Generic = variant == "generic rival"
				rival.Static = variant == "static rival"
				rival.Public = variant != "inaccessible rival"
				owner.Methods = append(owner.Methods, rival)
			case "array varargs", "packing only":
				target = "([I)V"
				owner.Methods = []Method{variableArityTestMethod(target, true)}
				if variant == "array varargs" {
					sam = "([I)V"
				}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			classes[owner.Name] = owner
			plan := SelectFixedArityReference(Witness{Owner: owner.Name, Name: "write", Desc: target, Kind: Virtual}, sam, provider, work)
			if plan.Supported != (variant == "control" || variant == "array varargs") {
				t.Fatalf("unexpected selection: %+v", plan)
			}
		})
	}
}

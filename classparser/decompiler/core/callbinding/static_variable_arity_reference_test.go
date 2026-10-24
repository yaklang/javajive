package callbinding

import (
	"slices"
	"testing"
)

// Class::method performs both a static search with n SAM inputs and an
// unbound-instance search with n-1 inputs. A remembered invokestatic target
// does not permit dropping the latter search or an earlier invocation phase.
func TestClassStaticVariableArityReferenceRequiresBothSourceSearches(t *testing.T) {
	for _, tc := range []struct {
		name, target, sam string
		others            []Method
		ports             []string
		want              bool
	}{
		{"expanded references", "([Lx/A;)V", "(Lx/B;Lx/B;)V", nil, []string{"Lx/A;", "Lx/A;"}, true},
		{"empty pack", "([Lx/A;)V", "()V", nil, nil, true},
		{"widening before pack", "([J)V", "(I)V", nil, []string{"J"}, true},
		{"boxing before pack", "([Ljava/lang/Object;)V", "(I)V", nil, []string{"Ljava/lang/Object;"}, true},
		{"fixed prefix and widening", "(J[D)V", "(IIF)V", nil, []string{"J", "D", "D"}, true},
		{"eligible instance search", "([Lx/A;)V", "(Lx/B;)V", []Method{{Name: "write", Desc: "()V", Public: true}}, nil, false},
		{"eligible instance pack", "([Lx/A;)V", "(Lx/B;)V", []Method{{Name: "write", Desc: "([Lx/C;)V", Public: true, Varargs: true}}, nil, false},
		{"ineligible instance arity", "([Lx/A;)V", "(Lx/B;)V", []Method{{Name: "write", Desc: "(Lx/A;Lx/A;)V", Public: true}}, []string{"Lx/A;"}, true},
		{"earlier static fixed phase", "([Lx/A;)V", "(Lx/B;)V", []Method{{Name: "write", Desc: "(Ljava/lang/Object;)V", Public: true, Static: true}}, nil, false},
		{"earlier static boxed phase", "([J)V", "(I)V", []Method{{Name: "write", Desc: "(Ljava/lang/Integer;)V", Public: true, Static: true}}, nil, false},
		{"generic eligible overload", "([Lx/A;)V", "(Lx/B;)V", []Method{{Name: "write", Desc: "(Ljava/lang/Object;)V", Signature: "<T:Ljava/lang/Object;>(TT;)V", Public: true, Static: true, Generic: true}}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := variableArityTestMethod(tc.target, true)
			target.Static = true
			provider, _ := variableArityTestProvider(append([]Method{target}, tc.others...))
			plan := SelectVariableArityReference(Witness{Owner: "x/Owner", Name: "write", Desc: tc.target, Kind: Static}, tc.sam, provider, nil)
			if plan.Supported != tc.want || plan.Supported && (!slices.Equal(plan.ExpandedParameters, tc.ports) || plan.DeclaringOwner != "x/Owner") {
				t.Fatalf("static source selection=%+v want supported=%v ports=%v", plan, tc.want, tc.ports)
			}
		})
	}
}

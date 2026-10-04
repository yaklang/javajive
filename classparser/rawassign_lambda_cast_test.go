package javaclassparser

// Raw functional reassignments retain their instantiated Collection SAM via
// a typed carrier or an explicit target. Moving the view into a local must keep
// that same local's value feeding the original raw assignment and return.

import (
	"os"
	"strings"
	"testing"
)

// TestRawAssignLambdaCastIsLoadBearing pins the raw-functional-interface reassignment cast fix. The
// seed's local is declared RAW `Function var2 = this.builder;` (the field is a raw java.util.function
// .Function), then reassigned a method reference and an explicitly-typed lambda. With the fix ON each
// reassignment is cast to its recovered parameterized `Function<Collection, Collection>` (the exact form
// the javac-visible source uses), so both bind against the raw SAM. With the kill-switch OFF the bare
// forms reappear, which javac rejects as "invalid method reference" / "incompatible parameter types in
// lambda expression". Real hit: fastjson2 ObjectReaderImplList (builder =
// (Function<Collection, Collection>) Collections::unmodifiableCollection / ((Collection list) -> ...)).
func TestRawAssignLambdaCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/RawAssignLambdaSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedSAMInstantiation(t, data, "(Ljava/lang/Object;)Ljava/lang/Object; => (Ljava/util/Collection;)Ljava/util/Collection;", "(Ljava/lang/Object;)Ljava/lang/Object; => (Ljava/util/Collection;)Ljava/util/Collection;")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_LAMBDA_ASSIGN_CAST_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		compact := compactReviewedGenericSource(source)
		carrier := reviewedFunctionalCarrier(t, source, "Function<Collection, Collection>", `Collections::unmodifiableCollection;`)
		target := reviewedFunctionalCarrier(t, source, "Function", `this.builder;`)
		if !strings.Contains(compact, target+"="+carrier+";") || !strings.Contains(compact, "return"+target+";") || !strings.Contains(compact, "(Function<Collection,Collection>)((") || !strings.Contains(compact, "Collections.singleton(") || !strings.Contains(compact, ".iterator().next()") {
			t.Fatalf("switch=%q: both raw reassignments must retain typed SAM inputs and original body: %s", setting, source)
		}
	}
}

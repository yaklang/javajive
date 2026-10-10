package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The writable Class local can use its original erasure and convert only on
// return. Require the same-local chain rather than a redundant cast per store.
func TestLocalReassignRawCastIsLoadBearing(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/LocalReassignRawSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarMethod(t, raw, "nonSerializableSuper", "(Ljava/lang/Class;)Ljava/lang/Class;", "<T:Ljava/lang/Object;>(Ljava/lang/Class<TT;>;)Ljava/lang/Class<-TT;>;")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF", setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		local := requireReviewedPattern(t, source, `Class\s+(\w+)\s*=\s*\w+\s*;`)[1]
		requireReviewedPattern(t, source, regexp.QuoteMeta(local)+`\s*=\s*`+regexp.QuoteMeta(local)+`\.getSuperclass\(\)\s*;`)
		if !strings.Contains(compactReviewedGenericSource(source), "return(Class<?superT>)(Class)("+local+");") {
			t.Fatalf("erased local lost generic return view:\n%s", source)
		}
		requireReviewedPattern(t, source, `Serializable\.class\.isAssignableFrom\(`+regexp.QuoteMeta(local)+`\)`)
	}
}

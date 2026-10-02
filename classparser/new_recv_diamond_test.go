package javaclassparser

import (
	"strings"
	"testing"
)

// The materialized String callback supplies its own SAM target. A raw new
// receiver therefore preserves constructor/callback binding without a diamond.
// Compare both legacy settings while pinning the original bootstrap and tuple.
func TestNewRecvJDKGenericDiamondIsLoadBearing(t *testing.T) {

	path := "testdata/regression/NewRecvDiamondSeed.class"
	raw, _, _ := reviewedFixtureMethod(t, path, "resolve", "(Ljava/util/function/UnaryOperator;)V")
	assertReviewedTypeVarMethod(t, raw, "resolve", "(Ljava/util/function/UnaryOperator;)V", "(Ljava/util/function/UnaryOperator<Ljava/lang/String;>;)V")
	assertReviewedGenericField(t, raw, "aliasMap", "Ljava/util/Map;", "Ljava/util/Map<Ljava/lang/String;Ljava/lang/String;>;")
	assertReviewedTypeVarInvoke(t, path, "resolve", "(Ljava/util/function/UnaryOperator;)V", 8, 183, "java/util/HashMap", "<init>", "(Ljava/util/Map;)V")
	assertReviewedTypeVarInvoke(t, path, "resolve", "(Ljava/util/function/UnaryOperator;)V", 18, 182, "java/util/HashMap", "forEach", "(Ljava/util/function/BiConsumer;)V")
	assertReviewedSeedSAM(t, raw, "(Ljava/lang/Object;Ljava/lang/Object;)V", "(Ljava/lang/String;Ljava/lang/String;)V")
	reviewedSeedSources(t, path, "JDEC_NEW_RECV_DIAMOND_OFF", false, func(source string) {
		body := reviewedSourceMethod(t, source, `resolve\(UnaryOperator<String> [^)]*\)`)
		carrier := reviewedFunctionalCarrier(t, body, "BiConsumer<String, String>", "(l0, l1) ->")
		if !strings.Contains(compactReviewedGenericSource(body), "newHashMap(this.aliasMap).forEach("+carrier+");") {
			t.Fatal("lost copy-map receiver or independently typed String callback")
		}
		if strings.Count(body, ".apply(") != 2 || !strings.Contains(body, "this.aliasMap.put(") {
			t.Fatal("lost callback resolver effects")
		}
	})
}

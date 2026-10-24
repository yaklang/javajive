package javaclassparser

import "testing"

func TestAdversarialNumericCastSourceTextCannotOwnViews(t *testing.T) {
	for _, source := range []string{
		`class C{String scope="assertj";double run(Object value){return ((Number)(value)).doubleValue();}}`,
		`class C{String scope="assertj";Object run(Object value){return overloaded((Number)(((Integer)(value))));}}`,
		`class C{String data="assertj (Number)(payload)";}`,
		`class C{String scope="assertj";double run(Object var0){return Math.round((var0).doubleValue());}}`,
	} {
		if got := fixAssertjRemainingReconstructs(source); got != source {
			t.Fatalf("source text changed a cast, original overload view or application data:\nbefore %s\nafter %s", source, got)
		}
	}
}

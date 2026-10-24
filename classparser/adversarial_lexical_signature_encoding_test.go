package javaclassparser

import (
	"fmt"
	"regexp"
	"testing"
)

// A type-variable name is decoded text during lexical binding, while a class
// file Signature is stored as MUTF-8 code units. Supplementary identifiers make
// those representations different without changing the binding algorithm.
func TestAdversarialLexicalSignaturesPreserveSupplementaryFormals(t *testing.T) {
	for _, fixture := range []struct {
		name, source, output string
		owned                []string
	}{
		{"enclosing", memberReferenceSignatureFixture, "18:owner:binding:identity:null\n", []string{"SuffixOwner", "SuffixOwner$Node"}},
		{"own-and-outer", ownAndOuterBindingFixture(), "12:own:outer:shadow:fields:identity\n", []string{"SuffixOwner$Member"}},
	} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []struct {
				name    string
				target  int
				profile SourceCompilerProfile
			}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
				t.Run(fmt.Sprintf("%s/%s/%s", fixture.name, debug, compiler.name), func(t *testing.T) {
					t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
					source := regexp.MustCompile(`\bT\b`).ReplaceAllString(fixture.source, "𐐀Bound")
					source = regexp.MustCompile(`\bU\b`).ReplaceAllString(source, "𐐁Bound")
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, fixture.owned, fixture.output, 8, compiler.target, compiler.profile)
				})
			}
		}
	}
}

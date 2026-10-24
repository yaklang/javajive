package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The input bytecode's capture convention and the selected source target are
// different axes. A modern target still has javac's constructor initialization
// cut; a mixed-version archive must retain the older input's early captures.
func TestAdversarialConstructorLoweringKeepsOriginalScheduleAtModernTargets(t *testing.T) {
	for _, target := range []int{8, 9, 11, 16, 17, 21} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("target%d/%s", target, debug), func(t *testing.T) {
				fixture := strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", `long r=seed;for(int k=0;k<5;k++)r=r*31L+mode;result=r;SuffixWitness.visits++;`, 1), "ROUNDS", "5")
				constructorSuffixFamilyAtTargetRoundTrip(t, fixture, debug, []string{"SuffixOwner$1"}, "72:constructor:closed:suffix:oracle\n", 8, target)
			})
		}
	}
}

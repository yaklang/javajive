package javaclassparser

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// A local initialized before a loop stays effectively final when the loop
// only reads it. The initializer is not replicated or moved into the loop;
// the unchanged caller observes exactly three reads before the next capture.
func captureReadonlyLoopFixture(kind string) string {
	body := `if(selected.item()!=token)throw new AssertionError("stable outer read");CapturedParentEffects.trace+="L";`
	loop := ""
	switch kind {
	case "for":
		loop = `for(int step=0;step<3;step++){` + body + `}`
	case "while":
		loop = `int step=0;while(step++<3){` + body + `}`
	case "do":
		loop = `int step=0;do{` + body + `}while(++step<3);`
	case "nested":
		loop = `for(int outer=0;outer<3;outer++){for(int inner=0;inner<1;inner++){` + body + `}}`
	}
	fixture := strings.Replace(anonymousCapturedParentFixture, `if(!CapturedParentEffects.choose(selected)`, loop+`if(!CapturedParentEffects.choose(selected)`, 1)
	return strings.Replace(fixture, `.equals("WP")`, `.equals("LLLWP")`, 1)
}
func TestAdversarialAnonymousCapturedParentReadOnlyLoop(t *testing.T) {
	for _, kind := range []string{"for", "while", "do", "nested"} {
		t.Run(kind, func(t *testing.T) {
			testNativeIndependentFamilyFixture(t, captureReadonlyLoopFixture(kind), []string{"CapturedParentOwner"}, "CapturedParentDriver", "6:anonymous:parent:binding:early:identity:failure\n", nativeLexicalExactSignatures)
		})
	}
}
func TestAdversarialAnonymousCapturedParentReadOnlyLoopOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", err, string(out))
	}
	for _, kind := range []string{"for", "while", "do", "nested"} {
		t.Run(kind, func(t *testing.T) {
			fixture := captureReadonlyLoopFixture(kind)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, fixture, "CapturedParentOwner", debug)
			}, NativeJavac8, javac, []string{"CapturedParentOwner"}, "CapturedParentDriver", "6:anonymous:parent:binding:early:identity:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}

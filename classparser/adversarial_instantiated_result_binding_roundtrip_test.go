package javaclassparser

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// javac removes a widening Object cast when the physical generic result already
// erases to Object. Reopening the receiver's source instantiation must preserve
// the caller's descriptor rather than select the new, narrower source overload.
// The driver observes identity, early constructor callbacks, exception identity
// and the exact number/order of both original overload calls.
func instantiatedResultBindingFixture(shape string, raw bool) (string, string) {
	fixture, owner := anonymousCapturedParentShape(shape)
	local := "selected"
	if shape == "renamed" {
		local = "originalDeclaredParent"
	}
	rival := "CharSequence"
	if shape == "array" {
		rival = "Cloneable"
	}
	fixture = strings.Replace(fixture, `static String trace="";`, `static String trace="";static Object chooseItem(Object item){trace+="O";return item;}static Object chooseItem(`+rival+` item){throw new AssertionError("wrong generic return binding");}`, 1)
	fixture = strings.Replace(fixture, `Object read(){return `+local+`.item();}`, `Object read(){return CapturedParentEffects.chooseItem((Object)`+local+`.item());}`, 1)
	if raw {
		arguments := "<CharSequence>"
		if shape == "two-parameters" {
			arguments = "<CharSequence,Object>"
		}
		fixture = strings.Replace(fixture, `final CapturedParentBox`+arguments+` `+local, `final CapturedParentBox `+local, 1)
		fixture = strings.Replace(fixture, `chooseItem((Object)`+local+`.item())`, `chooseItem(`+local+`.item())`, 1)
	}
	return strings.Replace(fixture, `.equals("WP")`, `.equals("WPOO")`, 1), owner
}

func testInstantiatedResultBindingShapes(t *testing.T, original8 bool, javac string) {
	for _, shape := range []string{"generic", "interface", "two-parameters", "renamed", "bounded", "intersection", "array"} {
		views := []bool{false}
		if shape == "generic" || shape == "interface" || shape == "two-parameters" || shape == "renamed" {
			views = append(views, true)
		}
		for _, raw := range views {
			view := "parameterized"
			if raw {
				view = "raw"
			}
			t.Run(shape+"/"+view, func(t *testing.T) {
				fixture, owner := instantiatedResultBindingFixture(shape, raw)
				const want = "6:anonymous:parent:binding:early:identity:failure\n"
				if original8 {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, "CapturedParentDriver", want, nil, nativeLexicalExactSignatures)
				} else {
					testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "CapturedParentDriver", want, nativeLexicalExactSignatures)
				}
			})
		}
	}
}

func TestAdversarialInstantiatedResultKeepsOriginalOverload(t *testing.T) {
	testInstantiatedResultBindingShapes(t, false, "")
}

func TestAdversarialInstantiatedResultOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", err, string(out))
	}
	testInstantiatedResultBindingShapes(t, true, javac)
}

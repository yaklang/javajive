package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The child loader override deliberately disagrees with its original outer.
// The unchanged caller observes that only the proved outermost status controls
// assertions, including reentrant initialization and failed-class reuse.
func memberAssertionMixedFixture(enabled, fail, renamed, deep bool) (string, string) {
	f := standaloneAssertionMixedFixture
	f = strings.Replace(f, "class MixedAssertionOwner {", "class MixedAssertionOwner { static class Child {", 1)
	f = strings.Replace(f, "class MixedAssertionDriver", "} class MixedAssertionDriver", 1)
	f = strings.ReplaceAll(f, "MixedAssertionOwner.class.getDeclaredField", "MixedAssertionOwner.Child.class.getDeclaredField")
	f = strings.ReplaceAll(f, "MixedAssertionOwner.check", "MixedAssertionOwner.Child.check")
	f = strings.Replace(f, `setClassAssertionStatus("MixedAssertionOwner",ENABLED);`, `setClassAssertionStatus("MixedAssertionOwner",ENABLED);ClassLoader.getSystemClassLoader().setClassAssertionStatus("MixedAssertionOwner$Child",!ENABLED);`, 1)
	if deep {
		f = strings.Replace(f, "static class Child {", "static class Branch { static class Child {", 1)
		f = strings.Replace(f, "} class MixedAssertionDriver", "}} class MixedAssertionDriver", 1)
		f = strings.ReplaceAll(f, "MixedAssertionOwner.Child", "MixedAssertionOwner.Branch.Child")
		f = strings.ReplaceAll(f, "MixedAssertionOwner$Child", "MixedAssertionOwner$Branch$Child")
	}
	f = strings.NewReplacer("ENABLED", fmt.Sprint(enabled), "FAIL", fmt.Sprint(fail)).Replace(f)
	owner := "MixedAssertionOwner"
	if renamed {
		f = strings.ReplaceAll(f, owner, "RenamedMixedMemberScope")
		owner = "RenamedMixedMemberScope"
	}
	return f, owner
}
func TestAdversarialMemberAssertionMixedInitializationRoundTrip(t *testing.T) {
	for _, deep := range []bool{false, true} {
		for _, renamed := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				for _, fail := range []bool{false, true} {
					t.Run(fmt.Sprintf("deep=%t/renamed=%t/enabled=%t/fail=%t", deep, renamed, enabled, fail), func(t *testing.T) {
						f, owner := memberAssertionMixedFixture(enabled, fail, renamed, deep)
						testNativePrivateSetterFixture(t, f, owner, "MixedAssertionDriver", "mixed-assertions:prefix:tail:metadata:effects:failure-retry\n")
					})
				}
			}
		}
	}
}
func TestAdversarialMemberAssertionMixedInitializationNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required for actual native member assertion input")
	}
	for _, deep := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprintf("deep=%t/enabled=%t/fail=%t", deep, enabled, fail), func(t *testing.T) {
					f, owner := memberAssertionMixedFixture(enabled, fail, false, deep)
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, f, owner, debug) }, NativeJavac8, javac, []string{owner}, "MixedAssertionDriver", "mixed-assertions:prefix:tail:metadata:effects:failure-retry\n", nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}

func TestNativeMemberMixedAssertionInitializerClearsFailedRecheck(t *testing.T) {
	fixture, owner := memberAssertionMixedFixture(true, false, false, false)
	files := nativeCompileDebugClasses(t, fixture, "none")
	z := nativeArchive(t, files)
	defer z.Close()
	root, e := Parse(files[owner+".class"])
	if e != nil {
		t.Fatal(e)
	}
	p := z.nativeMemberReader(root).planNativeMemberFamily()
	if p == nil {
		t.Fatal("original lexical family")
	}
	child := p.children[owner+"$Child"]
	if child == nil || child.assertions == nil || child.assertions.pureInitializer {
		t.Fatal("original member mixed assertion packet")
	}
	d := z.nativeMemberReader(child.object)
	d.nativeMemberRoot = p
	d.nativeMemberCurrent = child
	source, e := d.DumpClass()
	if e != nil || strings.Contains(source, DecompileStubMarker) || d.nativeAssertionInitProjection != child.assertions {
		t.Fatal("complete original member initializer", e, source)
	}
	if _, e = d.prepareNativeAssertions("<clinit>", "()V", nil); e == nil {
		t.Fatal("missing original initializer AST admitted")
	}
	if d.nativeAssertionInitProjection != nil {
		t.Fatal("failed initializer projection borrowed earlier occurrence")
	}
	if source, e = d.DumpClass(); e == nil {
		t.Fatal("failed source recheck admitted stale method cache", source)
	}
}

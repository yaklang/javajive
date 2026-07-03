package class_context

import "testing"

// TestKotlinNestedPackagesAreDotted pins that Kotlin runtime/reflect packages are treated as
// external "stdlib-like" packages whose nested types render with the dotted Java source spelling
// (KParameter.Kind) rather than the binary flat name (KParameter$Kind). Kotlin classes are never
// Yak-decompiled units, so the flat form makes javac read `KParameter$Kind` as a missing package --
// the spring-core KotlinReflectionParameterNameDiscoverer / MethodParameter$KotlinDelegate blocker.
func TestKotlinNestedPackagesAreDotted(t *testing.T) {
	dottedPkgs := []string{
		"kotlin",
		"kotlin.reflect",
		"kotlin.jvm.internal",
		"kotlinx",
		"kotlinx.coroutines",
	}
	for _, pkg := range dottedPkgs {
		if !isStdlibNestedDottedPackage(pkg) {
			t.Errorf("expected package %q to render nested types dotted, got flat", pkg)
		}
	}

	// A jar-internal package (a real decompiled unit whose nested classes ARE emitted as flat
	// `Outer$Inner` top-level units) must NOT be dotted, else the reference would no longer match the
	// flat declaration.
	notDotted := []string{
		"org.springframework.util",
		"com.google.common.collect",
		"",
	}
	for _, pkg := range notDotted {
		if isStdlibNestedDottedPackage(pkg) {
			t.Errorf("expected package %q to stay flat (jar-internal nested units), got dotted", pkg)
		}
	}

	// The binary->source conversion itself: KParameter$Kind -> KParameter.Kind.
	if got, ok := binaryNestedNameToSource("KParameter$Kind"); !ok || got != "KParameter.Kind" {
		t.Errorf("binaryNestedNameToSource(KParameter$Kind) = (%q,%v), want (KParameter.Kind,true)", got, ok)
	}
}

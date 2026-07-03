package javaclassparser

// 承重测试: 第三方(非 JDK)嵌套类引用的点号化 (JDEC_EXTERNAL_NESTED_DOT_OFF)。
//
// Yak 把**本 jar 内**的嵌套类摊平成顶层 `Outer$Inner` 单元, 并以该扁平名互相引用, 故本 jar 内引用必须保持
// 扁平。但引用一个**不在本 jar**的第三方嵌套类时(spring-core 的 `reactor.blockhound.BlockHound$Builder`),
// 该类只在 classpath 上以真正嵌套的 `Outer.Inner` 存在, 扁平 `Outer$Inner` 无法解析
// (javac "cannot find symbol: class BlockHound$Builder")。
//
// 判据: SiblingSuperTypes(外层二进制名) —— 它读恒存在的 super_class 项, 对**任何本 jar 内**类都能解析;
// ok=false 即该外层类在本 jar 外, 此时把引用与 import 都点号化成 `Outer.Inner` / `import pkg.Outer`。
// 关键: 依赖跨类 resolver, 故必须用 DecompileWithResolver; 单类 Decompile 无 resolver 时保持扁平(不变)。
// kill-switch JDEC_EXTERNAL_NESTED_DOT_OFF 置位后回退到扁平, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	extNestedDottedRefRe = regexp.MustCompile(`\bHolder\.Inner field;`)
	extNestedFlatRefRe   = regexp.MustCompile(`\bHolder\$Inner field;`)
	extNestedDottedImpRe = regexp.MustCompile(`import q\.Holder;`)
	extNestedFlatImpRe   = regexp.MustCompile(`import q\.Holder\$Inner;`)
)

func TestExternalNestedDotIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/ExternalNestedSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	holder, err := os.ReadFile("testdata/regression/ExtHolder.class")
	if err != nil {
		t.Fatalf("read holder: %v", err)
	}

	// EXTERNAL: the resolver does NOT know q/Holder, so SiblingSuperTypes returns ok=false and the
	// nested reference + import are dotted (`Holder.Inner`, `import q.Holder`) -- the recompilable form
	// for a genuinely-nested third-party type.
	externalResolver := func(internalName string) ([]byte, bool) { return nil, false }
	os.Unsetenv("JDEC_EXTERNAL_NESTED_DOT_OFF")
	ext, err := DecompileWithResolver(seed, externalResolver)
	if err != nil {
		t.Fatalf("decompile (external) failed: %v", err)
	}
	if !extNestedDottedRefRe.MatchString(ext) {
		t.Errorf("external: expected dotted `Holder.Inner field;`, got:\n%s", ext)
	}
	if !extNestedDottedImpRe.MatchString(ext) {
		t.Errorf("external: expected `import q.Holder;`, got:\n%s", ext)
	}
	if extNestedFlatRefRe.MatchString(ext) || extNestedFlatImpRe.MatchString(ext) {
		t.Errorf("external: flat `Holder$Inner` must NOT appear, got:\n%s", ext)
	}

	// IN-JAR: the resolver DOES know q/Holder (it is a sibling unit Yak emits flat), so the reference +
	// import stay flat (`Holder$Inner`), matching the flat `q.Holder$Inner` unit.
	inJarResolver := func(internalName string) ([]byte, bool) {
		if internalName == "q/Holder" {
			return holder, true
		}
		return nil, false
	}
	inJar, err := DecompileWithResolver(seed, inJarResolver)
	if err != nil {
		t.Fatalf("decompile (in-jar) failed: %v", err)
	}
	if !extNestedFlatRefRe.MatchString(inJar) {
		t.Errorf("in-jar: expected flat `Holder$Inner field;`, got:\n%s", inJar)
	}
	if extNestedDottedRefRe.MatchString(inJar) {
		t.Errorf("in-jar: dotted `Holder.Inner` must NOT appear for a same-jar flat unit, got:\n%s", inJar)
	}

	// Kill-switch: even with no resolver knowledge of q/Holder, the external dotting is disabled, so the
	// reference falls back to flat -- proving the extension is load-bearing.
	t.Setenv("JDEC_EXTERNAL_NESTED_DOT_OFF", "1")
	off, err := DecompileWithResolver(seed, externalResolver)
	if err != nil {
		t.Fatalf("decompile (kill-switch) failed: %v", err)
	}
	if !extNestedFlatRefRe.MatchString(off) {
		t.Errorf("kill-switch: expected flat `Holder$Inner field;` fallback, got:\n%s", off)
	}
	if extNestedDottedRefRe.MatchString(off) {
		t.Errorf("kill-switch: dotted `Holder.Inner` must NOT appear (kill-switch not load-bearing), got:\n%s", off)
	}
}

package javaclassparser

// 三个承重测试, 复刻 gson 反编译里被忽视的三类真实缺陷:
//
//  1. TestClassLitReturnCastIsLoadBearing
//     类字面量 `X.class`(静态类型 `Class<X>`)在 `<T> Class<T> wrap()` 里直接 return 时, 必须补回未受检
//     造型 `(Class<T>)`(gson Primitives.wrap)。兄弟字段 `Integer.TYPE` 早已经字段分支补造型, 类字面量这一
//     value-kind 之前漏了。kill-switch JDEC_CLASSLIT_RET_CAST_OFF。
//
//  2. TestDollarTopLevelClassIsPublic
//     名字本身含 '$' 的真·顶层类(gson `$Gson$Preconditions` / `$Gson$Types`): 它不在任何 InnerClasses
//     里, 顶层 ClassFile access_flags 已带 ACC_PUBLIC。旧代码把所有含 '$' 的类都当嵌套类, InnerClasses
//     查不到便误删 `public`, 令其跨包不可见("$X is not public")。kill-switch JDEC_NESTED_PUBLIC_OFF。
//
//  3. TestDollarTopLevelClassImported
//     跨包引用上述 '$' 顶层类时必须产出 `import pkga.$Dollar$Util;`。旧 import 逻辑因名字首段为空
//     (split("$") => ["", ...])把它当匿名类直接丢掉 import, 导致每个跨包使用点 `cannot find symbol`。
//     kill-switch JDEC_DOLLAR_FLAT_IMPORT_OFF。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// classLitCastRe matches the recovered `(Class<T>) (Integer.class)` cast (parens/space flexible).
var classLitCastRe = regexp.MustCompile(`\(\s*Class<T>\s*\)\s*\(?\s*Integer\.class`)

func TestClassLitReturnCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/ClassLitRetSeed.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	os.Unsetenv("JDEC_CLASSLIT_RET_CAST_OFF")
	on, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !classLitCastRe.MatchString(on) {
		t.Errorf("fix ON: expected `(Class<T>) Integer.class` cast, got:\n%s", on)
	}

	t.Setenv("JDEC_CLASSLIT_RET_CAST_OFF", "1")
	off, err := Decompile(seed)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if classLitCastRe.MatchString(off) {
		t.Errorf("fix OFF: expected the `(Class<T>)` cast to be gone, got:\n%s", off)
	}
}

func TestDollarTopLevelClassIsPublic(t *testing.T) {
	util, err := os.ReadFile("testdata/regression/DollarUtil_pkga.class")
	if err != nil {
		t.Fatalf("read util seed: %v", err)
	}

	// Fix ON (default): a top-level '$'-named class keeps its real ClassFile visibility (public).
	os.Unsetenv("JDEC_NESTED_PUBLIC_OFF")
	on, err := Decompile(util)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "public final class $Dollar$Util") {
		t.Errorf("fix ON: expected `public final class $Dollar$Util`, got:\n%s", on)
	}

	// Fix OFF: legacy strips `public` from every '$'-named class, demoting this real public top-level
	// class to package-private (the cross-package "is not public" defect).
	t.Setenv("JDEC_NESTED_PUBLIC_OFF", "1")
	off, err := Decompile(util)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "public final class $Dollar$Util") {
		t.Errorf("fix OFF: expected `public` to be stripped, got:\n%s", off)
	}
}

func TestDollarTopLevelClassImported(t *testing.T) {
	user, err := os.ReadFile("testdata/regression/DollarUser_pkgb.class")
	if err != nil {
		t.Fatalf("read user seed: %v", err)
	}
	utilBytes, err := os.ReadFile("testdata/regression/DollarUtil_pkga.class")
	if err != nil {
		t.Fatalf("read util seed: %v", err)
	}
	// Resolver feeds the '$'-named util class by its binary internal name (package -> slash form).
	resolver := func(internalName string) ([]byte, bool) {
		if internalName == "pkga/$Dollar$Util" {
			return utilBytes, true
		}
		return nil, false
	}

	importRe := regexp.MustCompile(`import\s+pkga\.\$Dollar\$Util;`)

	// Fix ON (default): the cross-package '$'-named class is imported as a flat name.
	os.Unsetenv("JDEC_DOLLAR_FLAT_IMPORT_OFF")
	on, err := DecompileWithResolver(user, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !importRe.MatchString(on) {
		t.Errorf("fix ON: expected `import pkga.$Dollar$Util;`, got:\n%s", on)
	}

	// Fix OFF: legacy drops the import for a '$'-leading name (empty split segment), reproducing the
	// `cannot find symbol` defect.
	t.Setenv("JDEC_DOLLAR_FLAT_IMPORT_OFF", "1")
	off, err := DecompileWithResolver(user, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if importRe.MatchString(off) {
		t.Errorf("fix OFF: expected the `import pkga.$Dollar$Util;` to be gone, got:\n%s", off)
	}
}

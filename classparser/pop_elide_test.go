package javaclassparser

// 承重测试: pop/pop2 丢弃无副作用且非语句的值时, 不得渲染成裸表达式语句 (`var0;`)。
// 种子 = spring-core cglib EmitUtils, javac 在 process_array 等方法里留了死的 `aload x; pop`,
// 旧实现把它发成 `var0;` (javac 报 "not a statement", 是 spring-core 整树重编译的阻断点)。
// kill-switch JDEC_POP_ELIDE_OFF 关掉治本后裸语句应当复现, 证明这条治本是承重的。

import (
	"os"
	"regexp"
	"testing"
)

// bareLocalStmtRe matches a statement line that is just a local-variable reference (`var0;`), which is
// invalid Java ("not a statement"). Anchored per-line so it does not match `var0.foo();` etc.
var bareLocalStmtRe = regexp.MustCompile(`(?m)^\s*var\d+;\s*$`)

func TestPopElideIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/SpringCglibEmitUtils.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): no bare local-only statements survive.
	os.Unsetenv("JDEC_POP_ELIDE_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if m := bareLocalStmtRe.FindAllString(on, -1); len(m) != 0 {
		t.Errorf("fix ON: expected no bare `varN;` statements, found %d: %v", len(m), m)
	}

	// Fix OFF (kill-switch): the legacy bare-statement emission reappears, proving the fix is what
	// removes them (not some unrelated pass).
	t.Setenv("JDEC_POP_ELIDE_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if m := bareLocalStmtRe.FindAllString(off, -1); len(m) == 0 {
		t.Errorf("fix OFF: expected bare `varN;` statements to reappear (kill-switch not load-bearing)")
	}
}

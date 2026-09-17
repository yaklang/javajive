package javaclassparser

// 承重测试: 无 default 的 switch 不得注入伪 default case。
// 种子 = commons-lang3 DateUtils, 其 range-style 内层 switch 只有 case 1..4、没有 default。
// 旧实现把不存在的默认(-1)无条件 re-key 成 math.MaxInt 哨兵, 于是发出空的
// `case 9223372036854775807:`(64 位 math.MaxInt)。该字面量是 javac 词法错("integer number too
// large"), 且位于 attribution 之前, 会遮蔽整树重编译里其余所有类型错(commons-lang3 的整树阻断点)。
// Default now has an independent field. The obsolete kill-switch must not resurrect the defect.

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
)

func TestSwitchSpuriousDefaultRetired(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/CommonsLang3DateUtils.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	// The sentinel is math.MaxInt (platform int width); on every 64-bit CI runner it prints as
	// 9223372036854775807. Build the pattern from math.MaxInt so the test is width-independent.
	spuriousRe := regexp.MustCompile(`case\s+` + strconv.Itoa(math.MaxInt) + `\s*:`)

	// Fix ON (default): no spurious math.MaxInt case survives.
	t.Setenv("JDEC_SWITCH_SPURIOUS_DEFAULT_OFF", "")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if m := spuriousRe.FindAllString(on, -1); len(m) != 0 {
		t.Errorf("fix ON: expected no spurious `case %d:`, found %d", math.MaxInt, len(m))
	}

	// The retired kill-switch is inert; correctness survives patch retirement.
	t.Setenv("JDEC_SWITCH_SPURIOUS_DEFAULT_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if m := spuriousRe.FindAllString(off, -1); len(m) != 0 {
		t.Errorf("retired switch flag reintroduced spurious `case %d:`", math.MaxInt)
	}
}

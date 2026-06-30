package javaclassparser

// 承重测试: 唯一定义是嵌入在条件里的赋值 (`if ((var2 = expr) == null){}else{return var2;}`) 的局部,
// 必须被补上声明, 否则渲染成未声明 -> `cannot find symbol`(fastjson2 JSONReaderJSONB
// readLocalDateTime12/14/16 家族, +37 tree)。两处耦合根因: (1) generatedLocalDeclRe 把 `return varN`
// 误当声明(关键字 `return` 命中类型标识符分支), 导致 addMissingGeneratedLocalDecls 以为已声明而跳过;
// (2) 即便补声明, 因 RHS 是跨类调用(Helper.parse/DateUtils.parse)无法文本推断类型而退化成 Object。
// 治法(kill-switch JDEC_RETURN_DECL_FIX_OFF): 收集声明时跳过关键字开头的伪声明; 对被 `return` 的未声明
// 局部, 以**所在方法的返回类型**声明(JLS 14.17 返回值必可赋给返回类型)。
// 种子 = 合成 `ReturnDeclSeed.readIt()` 返回 LocalDateTime, 经 `(v = Helper.parse(...)) == null`。

import (
	"os"
	"regexp"
	"testing"
)

// returnLocalDeclRe matches the synthesized declaration of the returned-but-otherwise-undeclared
// local, e.g. `LocalDateTime var2 = null;`.
var returnLocalDeclRe = regexp.MustCompile(`LocalDateTime var\d+ = null;`)

func TestReturnLocalDeclSynthesisIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ReturnLocalDeclSynthesis.class")
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	// Fix ON (default): the returned-but-undeclared local gets a `LocalDateTime varN = null;` decl.
	os.Unsetenv("JDEC_RETURN_DECL_FIX_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !returnLocalDeclRe.MatchString(on) {
		t.Errorf("fix ON: expected a synthesized `LocalDateTime varN = null;` declaration, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the declaration disappears (local rendered undeclared), proving this fix
	// is what synthesizes it rather than some unrelated pass.
	t.Setenv("JDEC_RETURN_DECL_FIX_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if returnLocalDeclRe.MatchString(off) {
		t.Errorf("fix OFF: expected the synthesized declaration to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}

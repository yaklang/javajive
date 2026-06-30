package javaclassparser

// 承重测试: 扁平内部类对**未声明外层类型变量**的 raw-erase (JDEC_INNER_RAW_ERASE_OFF)。
//
// `RawEraseSeed$Itr<T>` 是泛型外层 `RawEraseSeed<K, V>` 的非静态内部类, 但它自身又带形参 `<T>`
// (它是 Iterator<T>)。它的字段/返回签名仍提及外层变量 K, V (`Node<K, V>`)。扁平成顶层
// `RawEraseSeed$Itr<T>` 后:
//
//   - 外层形参元数对齐 (enclosing-arity) 帮不上忙: 追加 <K, V> 会改变 `Itr<ElementType>` 引用点的元数。
//   - 原样渲染 `Node<K, V>` 会出现未声明的 K, V -> javac "cannot find symbol: class K"。
//
// 治本: 把这些参数化 raw-erase 成裸类型 (`Node<K, V>` -> `Node`), 合法、运行期等价, 且正是字节码里局部变量
// 已有的形态。该用例镜像 gson LinkedTreeMap$LinkedTreeMapIterator 与 guava 自带形参内部类一族。
//
// 关键: raw-erase 仅由 Itr 自身字节码推导 (字段签名提及非自身形参的类型变量), 不依赖跨类 resolver,
// 故单类 Decompile 即可触发。kill-switch 置位后回退到原样 `Node<K, V>`, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

var (
	rawEraseOnRe  = regexp.MustCompile(`RawEraseSeed\$Node next;`)
	rawEraseOffRe = regexp.MustCompile(`RawEraseSeed\$Node<K, V> next;`)
	// `Iterator<T>` (own param) must survive raw-erase in BOTH states -- only the undeclared K, V go.
	rawEraseOwnParamRe = regexp.MustCompile(`class RawEraseSeed\$Itr<T extends Object> implements Iterator<T>`)
)

func TestInnerRawEraseIsLoadBearing(t *testing.T) {
	itrBytes, err := os.ReadFile("testdata/regression/RawEraseSeed$Itr.class")
	if err != nil {
		t.Fatalf("read Itr seed: %v", err)
	}

	// Fix ON (default): the undeclared enclosing variables K, V are raw-erased, so the `Node<K, V>`
	// field/return types render as raw `Node`, while the own param `T` survives.
	os.Unsetenv("JDEC_INNER_RAW_ERASE_OFF")
	on, err := Decompile(itrBytes)
	if err != nil {
		t.Fatalf("decompile Itr (fix ON) failed: %v", err)
	}
	if !rawEraseOnRe.MatchString(on) {
		t.Errorf("fix ON: expected raw `RawEraseSeed$Node next;`, got:\n%s", on)
	}
	if rawEraseOffRe.MatchString(on) {
		t.Errorf("fix ON: undeclared `RawEraseSeed$Node<K, V> next;` must NOT appear, got:\n%s", on)
	}
	if !rawEraseOwnParamRe.MatchString(on) {
		t.Errorf("fix ON: own param `Itr<T> implements Iterator<T>` must survive raw-erase, got:\n%s", on)
	}

	// Fix OFF (kill-switch): the undeclared K, V are rendered verbatim (`Node<K, V>`), the exact
	// "cannot find symbol: class K" recompile blocker the fix removes -- proving it is load-bearing.
	t.Setenv("JDEC_INNER_RAW_ERASE_OFF", "1")
	off, err := Decompile(itrBytes)
	if err != nil {
		t.Fatalf("decompile Itr (fix OFF) failed: %v", err)
	}
	if !rawEraseOffRe.MatchString(off) {
		t.Errorf("fix OFF: expected verbatim `RawEraseSeed$Node<K, V> next;` fallback, got:\n%s", off)
	}
	if rawEraseOnRe.MatchString(off) {
		t.Errorf("fix OFF: raw `RawEraseSeed$Node next;` must NOT appear (kill-switch not load-bearing), got:\n%s", off)
	}
	if !rawEraseOwnParamRe.MatchString(off) {
		t.Errorf("fix OFF: own param `Itr<T> implements Iterator<T>` must still survive, got:\n%s", off)
	}
}

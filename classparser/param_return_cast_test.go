package javaclassparser

// 承重测试: 方法返回 `Ctor<E>`(E 为无界型变), 而 `return` 的值是一个泛型方法调用, 其返回型变被某 Object
// 实参「钉死」为 Object(target-type 反推救不回来)时, 需把整个返回表达式包一层非受检 `(Ctor<E>)` 造型。
//
// 镜像 guava `Multisets$1$1.computeNext` 等: `return Multisets.immutableEntry(var2, n)`, 其中
// `immutableEntry` 是 `<E> Entry<E> immutableEntry(E, int)`, var2 由 raw 迭代器 `getElement()` 读成 Object
// → javac「inference variable E has incompatible bounds: E, Object」。因为 E 同时出现在返回类型 `Entry<E>`
// 与形参, 而实参 Object 把 E 钉成 Object, 返回目标类型无法再把它推成类作用域 E。治法(kill-switch
// JDEC_PARAM_RETURN_CAST_OFF): 当方法返回类型是「每个顶层型实参都是无界型变」的参数化类型、且返回值擦除到
// 同一 raw 类但携带不同(Object/擦除)型实参时, 渲染 `(Entry<E>)(call)` 非受检造型 —— 它把内层调用还原成独立
// 表达式(E 自由推成 Object)再造型, 同 raw 擦除的 `Entry<Object>`->`Entry<E>` 造型合法(有界型变会 inconvertible,
// 故仅限无界)。仅对「非 target-typing 友好」形状触发: 排除零参工厂 / lambda 实参调用 / 裸 lambda 返回。
// 种子: `ParamRetCastSeed<E>.make` 返回 `box((E) this.raw, 1)`(源码 (E) 造型被字节码擦除), box 为同类 static
// 泛型 `<T> Box<T> box(T, int)`。ON=`(ParamRetCastBox<E>) (box(this.raw,1))` / OFF=裸 `box(this.raw,1)`。

import (
	"os"
	"strings"
	"testing"
)

func TestParamReturnCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/ParamRetCastSeed.class")
	if err != nil {
		t.Fatalf("read ParamRetCastSeed seed: %v", err)
	}

	// Fix ON (default): the unchecked parameterization cast is emitted so the return recompiles.
	// Two shapes: (1) unbounded bare type-var target `ParamRetCastBox<E>` from an Object-pinned call;
	// (2) WILDCARD target `ParamRetCastBox<? super E>` from a `ParamRetCastBox<?>` field (the guava
	// TypeToken `return of(bound)` shape).
	os.Unsetenv("JDEC_PARAM_RETURN_CAST_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "(ParamRetCastBox<E>) (box(this.raw,1))") {
		t.Errorf("fix ON: expected parameterized return cast `(ParamRetCastBox<E>) (box(this.raw,1))`, got:\n%s", on)
	}
	if !strings.Contains(on, "(ParamRetCastBox<? super E>) (rawBox(this.raw))") {
		t.Errorf("fix ON: expected wildcard return cast `(ParamRetCastBox<? super E>) (rawBox(this.raw))`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): both casts disappear -- the exact "inference variable E has incompatible
	// bounds" / "X<?> cannot be converted to X<? super E>" recompile blockers the fix removes -- proving
	// it is load-bearing.
	t.Setenv("JDEC_PARAM_RETURN_CAST_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if strings.Contains(off, "(ParamRetCastBox<E>) (box(this.raw,1))") {
		t.Errorf("fix OFF: expected the unbounded-var cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
	if strings.Contains(off, "(ParamRetCastBox<? super E>) (rawBox(this.raw))") {
		t.Errorf("fix OFF: expected the wildcard cast to disappear (kill-switch not load-bearing), got:\n%s", off)
	}
}

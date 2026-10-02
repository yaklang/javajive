package javaclassparser

// 承重测试: 把"非泛型具体子类型"当作"被类型变量参数化的泛型父类型"返回时, 必须补回未受检造型 `(R<T>)`。
// 复刻 gson TypeAdapterFactory.create 家族: `ObjectTypeAdapter extends TypeAdapter<Object>` 这种
// 非泛型子类, 在 `<T> TypeAdapter<T> create()` 里 `return new ObjectTypeAdapter(...)`。子类钉死了父类的
// 类型实参, 不走 unchecked conversion, javac 报 "GenRetConcrete 无法转换为 GenRetBase<T>"; 源码本身带
// `(GenRetBase<T>)` 造型, 反编译丢了它。治法在 typeVarReturnCast 里对 NewExpression/TernaryExpression
// 调 genericReturnSubtypeCastNeeded (要求: 擦除不同 + 子类自身非泛型) 补回造型。
//
// 关键护栏: `ident()` 里 `new GenRetInner<K,V>()`(泛型子类, 与 `GenRetBase<K>` 同擦除 GenRetBase) 走
// 恒等/unchecked, 绝不能被过度造型 -- 正是历史 InnerNode 过度造型回归的样本。该治本依赖跨类 resolver 读
// 子类自身 Signature, 故用 DecompileWithResolver 喂入 GenRetBase/GenRetConcrete/GenRetInner 三个 sibling。
// kill-switch JDEC_GENERIC_RET_SUBTYPE_CAST_OFF 关掉后造型消失, 证明承重。

import (
	"os"
	"regexp"
	"testing"
)

// genRetCastRe matches the recovered unchecked cast `(GenRetBase<T>) (new GenRetConcrete())`
// (whitespace + optional grouping parens around the constructed value are flexible).
var genRetCastRe = regexp.MustCompile(`\(\s*GenRetBase<T>\s*\)\s*(?:\(\s*GenRetBase\s*\)\s*)?\(?\s*new\s+GenRetConcrete`)

// genRetIdentOverCastRe matches the FORBIDDEN over-cast on the same-erasure generic-subtype identity return.
var genRetIdentOverCastRe = regexp.MustCompile(`\(\s*GenRetBase<[^)]*>\s*\)\s*(?:\(\s*GenRetBase\s*\)\s*)?\(?\s*new\s+GenRetInner`)

func genRetResolver() func(string) ([]byte, bool) {
	table := map[string]string{
		"GenRetBase":     "testdata/regression/GenRetBase.class",
		"GenRetConcrete": "testdata/regression/GenRetConcrete.class",
		"GenRetInner":    "testdata/regression/GenRetInner.class",
	}
	return func(internalName string) ([]byte, bool) {
		p, ok := table[internalName]
		if !ok {
			return nil, false
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, false
		}
		return b, true
	}
}

func TestGenericReturnSubtypeCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/GenRetSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMethod(t, data, "create", "()LGenRetBase;", "<T:Ljava/lang/Object;>()LGenRetBase<TT;>;")
	assertReviewedGenericMethod(t, data, "ident", "()LGenRetBase;", "<K:Ljava/lang/Object;V:Ljava/lang/Object;>()LGenRetBase<TK;>;")
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_GENERIC_RET_SUBTYPE_CAST_OFF", setting)
		source, err := DecompileWithResolver(data, genRetResolver())
		if err != nil {
			t.Fatal(err)
		}
		if genRetCastRe.MatchString(source) != (setting == "") {
			t.Fatalf("switch=%q: missing widening/same-erasure generic return view: %s", setting, source)
		}
		if genRetIdentOverCastRe.MatchString(source) {
			t.Fatalf("switch=%q: already assignable generic identity return must not receive a new narrowing cast: %s", setting, source)
		}
	}
}

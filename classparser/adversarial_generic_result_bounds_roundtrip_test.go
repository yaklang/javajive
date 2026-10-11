package javaclassparser

import (
	"strings"
	"testing"
)

// The untouched driver calls both physical Object-erased identity methods and
// checks source overload binding, payload identity and getter evaluation count.
// Object followed by interface bounds retains Object as its JVM erasure; every
// later interface still participates in Java overload applicability.
const genericResultBoundsFixture = `class ResultBoundEffects{static String trace="";static Object choose(Object v){trace+="O";return v;}static Object choose(CharSequence v){throw new AssertionError("wrong intersection result binding");}}
 class ResultBoundBox<T>{final T value;ResultBoundBox(T v){value=v;}T get(){ResultBoundEffects.trace+="G";return value;}}
 public class ResultBoundOwner<T extends Object & CharSequence>{T identity(T v){return v;}static <U extends Object & CharSequence>U methodIdentity(U v){return v;}Object use(ResultBoundBox<T> box){return ResultBoundEffects.choose((Object)box.get());}static <U extends Object & CharSequence>Object method(ResultBoundBox<U> box){return ResultBoundEffects.choose((Object)box.get());}}
 class ResultBoundDriver{public static void main(String[]args){int rows=0;for(CharSequence value:new CharSequence[]{null,new String("same"),new StringBuilder("mutable")})for(boolean method:new boolean[]{false,true}){ResultBoundEffects.trace="";if(new ResultBoundOwner<CharSequence>().identity(value)!=value||ResultBoundOwner.methodIdentity(value)!=value)throw new AssertionError("original result/erasure identity");ResultBoundBox<CharSequence> box=new ResultBoundBox<CharSequence>(value);Object got=method?ResultBoundOwner.method(box):new ResultBoundOwner<CharSequence>().use(box);if(got!=value||!ResultBoundEffects.trace.equals("GO"))throw new AssertionError("identity/evaluation/binding");rows++;}System.out.println(rows+":intersection:binding:identity:once");}}`

func genericResultBoundsShape(shape, producer string) (string, string, string) {
	fixture, owner, driver := genericResultBoundsFixture, "ResultBoundOwner", "ResultBoundDriver"
	switch shape {
	case "later-interface", "method-shadows-class":
		fixture = strings.Replace(fixture, `for(CharSequence value:new CharSequence[]{null,new String("same"),new StringBuilder("mutable")})`, `for(String value:new String[]{null,new String("same"),new String("different")})`, 1)
		fixture = strings.ReplaceAll(fixture, `<CharSequence>`, `<String>`)
		if shape == "later-interface" {
			fixture = strings.ReplaceAll(fixture, `extends Object & CharSequence`, `extends Object & java.io.Serializable & CharSequence`)
		} else {
			fixture = strings.Replace(fixture, `ResultBoundOwner<T extends Object & CharSequence>`, `ResultBoundOwner<T extends Object & java.io.Serializable>`, 1)
			fixture = strings.NewReplacer(`<U `, `<T `, `>U `, `>T `, `(U `, `(T `, `<U>`, `<T>`).Replace(fixture)
		}
	case "rank-one", "rank-two":
		rank := "[]"
		inputs := `new CharSequence[][]{null,new CharSequence[]{new String("same")},new CharSequence[]{new StringBuilder("mutable"),null}}`
		if shape == "rank-two" {
			rank = "[][]"
			inputs = `new CharSequence[][][]{null,new CharSequence[][]{new CharSequence[]{new String("same")}},new CharSequence[][]{null,new CharSequence[]{new StringBuilder("mutable"),null}}}`
		}
		fixture = strings.Replace(fixture, `for(CharSequence value:new CharSequence[]{null,new String("same"),new StringBuilder("mutable")})`, `for(CharSequence`+rank+` value:`+inputs+`)`, 1)
		fixture = strings.NewReplacer(`choose(CharSequence v)`, `choose(CharSequence`+rank+` v)`, `ResultBoundBox<T> box`, `ResultBoundBox<T`+rank+`> box`, `ResultBoundBox<U> box`, `ResultBoundBox<U`+rank+`> box`, `ResultBoundBox<CharSequence>`, `ResultBoundBox<CharSequence`+rank+`>`, `T identity(T v)`, `T`+rank+` identity(T`+rank+` v)`, `>U methodIdentity(U v)`, `>U`+rank+` methodIdentity(U`+rank+` v)`).Replace(fixture)
	case "renamed":
		fixture = strings.ReplaceAll(fixture, "ResultBound", "ScopedConstraint")
		owner, driver = "ScopedConstraintOwner", "ScopedConstraintDriver"
	}
	if producer == "field" {
		fixture = strings.ReplaceAll(fixture, `box.get()`, `box.value`)
		fixture = strings.Replace(fixture, `trace.equals("GO")`, `trace.equals("O")`, 1)
	}
	return fixture, owner, driver
}

func TestAdversarialGenericResultBoundsKeepErasureAndBinding(t *testing.T) {
	for _, shape := range []string{"intersection", "later-interface", "method-shadows-class", "rank-one", "rank-two", "renamed"} {
		for _, producer := range []string{"getter", "field"} {
			t.Run(shape+"/"+producer, func(t *testing.T) {
				fixture, owner, driver := genericResultBoundsShape(shape, producer)
				testNativePrivateSetterCompiledFixture(t, owner, driver, "6:intersection:binding:identity:once\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
				}, nativeLexicalExactSignatures)
			})
		}
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The original driver deliberately passes heap-polluted arrays through a raw
// receiver. A source view must retain the original descriptor without inserting
// a narrower array check. Both source binding and original ABI/Signature matter.
const genericArrayDeclarationResultFixture = `class ArrayResultEffects{static String trace="";static Object[] choose(Object[] v){trace+="O";return v;}static Object[] choose(CharSequence[] v){throw new AssertionError("wrong array declaration result binding");}}
 class ArrayResultBox<T>{final T[] value;ArrayResultBox(T[] v){value=v;}T[] get(){ArrayResultEffects.trace+="G";return value;}}
 public class ArrayResultOwner<T extends Object & CharSequence>{T[] identity(T[] v){return v;}static <U extends Object & CharSequence>U[] methodIdentity(U[] v){return v;}Object[] use(ArrayResultBox<T> box){return ArrayResultEffects.choose((Object[])box.get());}static <U extends Object & CharSequence>Object[] method(ArrayResultBox<U> box){return ArrayResultEffects.choose((Object[])box.get());}}
 class ArrayResultDriver{public static void main(String[]args){int rows=0;for(Object[] value:new Object[][]{null,new CharSequence[]{new String("same"),new StringBuilder("mutable"),null},new Integer[]{7,null}})for(boolean method:new boolean[]{false,true}){ArrayResultEffects.trace="";ArrayResultBox<CharSequence> box=(ArrayResultBox<CharSequence>)(ArrayResultBox)new ArrayResultBox(value);Object[] got=method?ArrayResultOwner.method(box):new ArrayResultOwner<CharSequence>().use(box);if(got!=value||!ArrayResultEffects.trace.equals("GO"))throw new AssertionError("identity/evaluation/binding/heap pollution");rows++;}System.out.println(rows+":array-declaration:binding:identity:once:pollution");}}`

func genericArrayDeclarationResultShape(shape, producer string) (string, string, string) {
	fixture, owner, driver := genericArrayDeclarationResultFixture, "ArrayResultOwner", "ArrayResultDriver"
	switch shape {
	case "rank-two":
		fixture = strings.ReplaceAll(fixture, "Object[]", "Object[][]")
		fixture = strings.ReplaceAll(fixture, "CharSequence[]", "CharSequence[][]")
		fixture = strings.ReplaceAll(fixture, "T[]", "T[][]")
		fixture = strings.ReplaceAll(fixture, "U[]", "U[][]")
		fixture = strings.Replace(fixture, `new CharSequence[][]{new String("same"),new StringBuilder("mutable"),null}`, `new CharSequence[][]{new CharSequence[]{new String("same"),new StringBuilder("mutable"),null},null}`, 1)
		fixture = strings.Replace(fixture, `new Integer[]{7,null}`, `new Integer[][]{new Integer[]{7,null},null}`, 1)
	case "raw":
		fixture = strings.ReplaceAll(fixture, "ArrayResultBox<T> box", "ArrayResultBox box")
		fixture = strings.ReplaceAll(fixture, "ArrayResultBox<U> box", "ArrayResultBox box")
	case "interface":
		fixture = strings.Replace(fixture, "class ArrayResultBox<T>", "interface ArrayResultProducer<T>{T[] get();}class ArrayResultBox<T> implements ArrayResultProducer<T>", 1)
		fixture = strings.Replace(fixture, "T[] get(){", "public T[] get(){", 1)
		fixture = strings.ReplaceAll(fixture, "ArrayResultBox<T> box", "ArrayResultProducer<T> box")
		fixture = strings.ReplaceAll(fixture, "ArrayResultBox<U> box", "ArrayResultProducer<U> box")
	case "primitive-array":
		fixture = strings.ReplaceAll(fixture, "CharSequence", "Cloneable")
		fixture = strings.Replace(fixture, `new Cloneable[]{new String("same"),new StringBuilder("mutable"),null}`, `new int[][]{new int[]{-1,0,1},null}`, 1)
		fixture = strings.Replace(fixture, `new Integer[]{7,null}`, `new String[][]{new String[]{"polluted",null},null}`, 1)
		fixture = strings.ReplaceAll(fixture, "ArrayResultBox<Cloneable>", "ArrayResultBox<int[]>")
		fixture = strings.ReplaceAll(fixture, "ArrayResultOwner<Cloneable>", "ArrayResultOwner<int[]>")
	case "renamed":
		fixture = strings.ReplaceAll(fixture, "ArrayResult", "RankedPayload")
		owner, driver = "RankedPayloadOwner", "RankedPayloadDriver"
	}
	if producer == "field" {
		fixture = strings.ReplaceAll(fixture, "box.get()", "box.value")
		fixture = strings.Replace(fixture, `trace.equals("GO")`, `trace.equals("O")`, 1)
	}
	return fixture, owner, driver
}

func testGenericArrayDeclarationResults(t *testing.T, original8 bool, javac string) {
	for _, shape := range []string{"rank-one", "rank-two", "raw", "interface", "primitive-array", "renamed"} {
		producers := []string{"getter", "field"}
		if shape == "interface" {
			producers = producers[:1]
		}
		for _, producer := range producers {
			t.Run(shape+"/"+producer, func(t *testing.T) {
				fixture, owner, driver := genericArrayDeclarationResultShape(shape, producer)
				const want = "6:array-declaration:binding:identity:once:pollution\n"
				if original8 {
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, driver, want, nil, nativeLexicalExactSignatures)
				} else {
					testNativePrivateSetterCompiledFixture(t, owner, driver, want, func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
					}, nativeLexicalExactSignatures)
				}
			})
		}
	}
}

func TestAdversarialGenericArrayDeclarationResultKeepsOriginalUse(t *testing.T) {
	testGenericArrayDeclarationResults(t, false, "")
}

func TestAdversarialGenericArrayDeclarationResultOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual original compiler")
	}
	if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
		t.Fatal("original compiler", err, string(out))
	}
	testGenericArrayDeclarationResults(t, true, javac)
}

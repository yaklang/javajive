package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func testOriginalVisibilityBridgeLexicalCall(t *testing.T, native bool) {
	for _, shape := range []string{"lexical-method"} {
		t.Run(shape, func(t *testing.T) {
			owner := "FactoryWideningConsumer"
			source := `import java.util.*; public class FactoryWideningConsumer<T extends Comparable<T>> extends HiddenGenericParent<T> {
 public final Iterator<T> initial; public FactoryWideningConsumer(){initial=FactoryWideningProvider.empty();}
 public Iterator<T> make(){return initial;}
 }
 class HiddenGenericParent<E extends Comparable<E>> {public boolean accept(E value){return value!=null;}}
 abstract class FactoryWideningIterator<E> implements Iterator<E>{}
 class FactoryWideningProvider {
 static int calls;static final RuntimeException fault=new IllegalStateException("identity");
 static <E> FactoryWideningIterator<E> empty(){calls++;return new FactoryWideningIterator<E>(){public boolean hasNext(){return false;}public E next(){throw fault;}public void remove(){throw fault;}};}}
 class FactoryWideningDriver {public static void main(String[]args){
 FactoryWideningConsumer<String> first=new FactoryWideningConsumer<String>();Iterator<String> a=first.make();
 FactoryWideningConsumer<Integer> second=new FactoryWideningConsumer<Integer>();Iterator<Integer> b=second.make();
 if(FactoryWideningProvider.calls!=WANT||(Object)a==(Object)b||a==null||b==null||a.hasNext()||b.hasNext())throw new AssertionError("factory evaluation/identity");
 for(Iterator<?> it:Arrays.<Iterator<?>>asList(a,b)){try{it.next();throw new AssertionError("next did not throw");}catch(RuntimeException e){if(e!=FactoryWideningProvider.fault)throw new AssertionError("next identity");}try{it.remove();throw new AssertionError("remove did not throw");}catch(RuntimeException e){if(e!=FactoryWideningProvider.fault)throw new AssertionError("remove identity");}}
 System.out.println("factory:generic:identity:effects");}}
`
			count := "2"
			if shape != "field" && shape != "lexical-method" {
				count = "4"
				source = strings.Replace(source, "public Iterator<T> make(){return initial;}", `public Iterator<T> make(){return new Iterator<T>(){Iterator<T> cursor=FactoryWideningProvider.empty();public boolean hasNext(){return cursor.hasNext();}public T next(){return cursor.next();}public void remove(){cursor.remove();}};}`, 1)
			}
			if shape == "lexical-method" {
				source = strings.Replace(source, "public Iterator<T> make(){return initial;}", `public Iterator<T> make(){return initial;}public class Gate {public boolean check(Object input){try{T value=(T)input;return FactoryWideningConsumer.this.accept(value);}catch(ClassCastException e){return false;}}}`, 1)
				source = strings.Replace(source, `System.out.println("factory:generic:identity:effects");`, `if(!first.new Gate().check("value")||first.new Gate().check(new Object())||first.new Gate().check(null))throw new AssertionError("qualified enclosing erased formal");System.out.println("factory:generic:identity:effects");`, 1)
			}
			if shape == "unbounded" {
				source = strings.Replace(source, "T extends Comparable<T>", "T", 1)
			}
			if shape == "number-bound" {
				source = strings.Replace(source, "T extends Comparable<T>", "T extends Number", 1)
				source = strings.ReplaceAll(source, "<String>", "<Integer>")
			}
			if shape == "intersection" {
				source = strings.Replace(source, "T extends Comparable<T>", "T extends Object & Comparable<T> & java.io.Serializable", 1)
			}
			if shape == "renamed" {
				source = strings.ReplaceAll(source, "FactoryWideningConsumer", "RenamedFactoryConsumer")
				owner = "RenamedFactoryConsumer"
				source = strings.NewReplacer("<T extends", "<Element extends", "Iterator<T>", "Iterator<Element>", "<T>", "<Element>", "public T ", "public Element ").Replace(source)
			}
			source = strings.ReplaceAll(source, "WANT", count)
			if native {
				compiler := os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, compiler, []string{owner}, "FactoryWideningDriver", "factory:generic:identity:effects\n", nil, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			} else {
				testNativePrivateSetterCompiledFixture(t, owner, "FactoryWideningDriver", "factory:generic:identity:effects\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
				}, nativeLexicalExactSignatures, nativeCovariantBridgeABI)
			}
		})
	}
}
func TestAdversarialVisibilityBridgeLexicalCall(t *testing.T) {
	testOriginalVisibilityBridgeLexicalCall(t, false)
}
func TestAdversarialVisibilityBridgeLexicalCallNativeCompiler(t *testing.T) {
	testOriginalVisibilityBridgeLexicalCall(t, true)
}

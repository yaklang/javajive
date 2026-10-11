package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The untouched driver observes the iterator producer and the nested entry's
// snapshot separately. Re-evaluating entrySet/iterator/next, capturing the wrong
// word, or flattening either anonymous scope changes its result.
const nestedIteratorCaptureFixture = `import java.util.*;
class IteratorCaptureOwner {
 static abstract class Cursor<E> implements Iterator<E>{public void remove(){throw new UnsupportedOperationException();}}
 static class View<K,V>{final Map<K,V> map;View(Map<K,V> map){this.map=map;}
 Iterator<Map.Entry<K,Set<V>>> iterator(){final Iterator<Map.Entry<K,V>> backing=map.entrySet().iterator();return new Cursor<Map.Entry<K,Set<V>>>(){
 public boolean hasNext(){return backing.hasNext();}
 public Map.Entry<K,Set<V>> next(){final Map.Entry<K,V> entry=backing.next();return new Map.Entry<K,Set<V>>(){public K getKey(){return entry.getKey();}public Set<V> getValue(){return Collections.singleton(entry.getValue());}public Set<V> setValue(Set<V> value){throw new UnsupportedOperationException();}};}
 };}}
}
class IteratorCaptureDriver{public static void main(String[]args){int rows=0;for(String word:new String[]{null,"",new String("word"),new String(new char[]{0}),new String(new char[]{(char)0xd800}),new String(new char[]{(char)0xd83d,(char)0xde00})}){Map<String,String> map=new LinkedHashMap<String,String>();String key=new String("key");map.put(key,word);IteratorCaptureOwner.View<String,String> view=new IteratorCaptureOwner.View<String,String>(map);Iterator<Map.Entry<String,Set<String>>> cursor=view.iterator();if(!cursor.hasNext())throw new AssertionError("start");Map.Entry<String,Set<String>> entry=cursor.next();if(entry.getKey()!=key||entry.getValue().iterator().next()!=word||cursor.hasNext())throw new AssertionError("identity and exhaustion");String replacement=new String("replacement");map.put(key,replacement);if(entry.getValue().iterator().next()!=replacement)throw new AssertionError("live entry");try{cursor.next();throw new AssertionError("missing exhaustion");}catch(NoSuchElementException expected){}try{cursor.remove();throw new AssertionError("mutable iterator");}catch(UnsupportedOperationException expected){}try{entry.setValue(Collections.<String>emptySet());throw new AssertionError("mutable entry");}catch(UnsupportedOperationException expected){}if(!cursor.getClass().isAnonymousClass()||!entry.getClass().isAnonymousClass()||cursor.getClass().getEnclosingMethod()==null||entry.getClass().getEnclosingMethod()==null||!entry.getClass().getEnclosingMethod().getName().equals("next"))throw new AssertionError("anonymous scopes");rows++;}System.out.println(rows+":iterator:capture:identity:scope");}}`

func nestedIteratorCaptureShape(shape string) string {
	f := nestedIteratorCaptureFixture
	if shape != "plain" {
		f = strings.Replace(f, " static class View", ` static class DelegatingCursor<E> extends Cursor<E>{final Iterator<E> delegate;DelegatingCursor(Iterator<E> delegate){this.delegate=delegate;}public boolean hasNext(){return delegate.hasNext();}public E next(){return delegate.next();}}
 static int productions;static final RuntimeException failure=new RuntimeException("factory");static <E>Cursor<E> narrow(Iterator<E> delegate){productions++;if(delegate==null)throw failure;return new DelegatingCursor<E>(delegate);}
 static class View`, 1)
		f = strings.Replace(f, "backing=map.entrySet().iterator()", "backing=narrow(map.entrySet().iterator())", 1)
	}
	if shape != "plain" {
		f = strings.Replace(f, "Iterator<Map.Entry<String,Set<String>>> cursor=view.iterator();", `IteratorCaptureOwner.productions=0;Iterator<Map.Entry<String,Set<String>>> cursor=view.iterator();if(IteratorCaptureOwner.productions!=1)throw new AssertionError("one producer");try{IteratorCaptureOwner.narrow(null);throw new AssertionError("missing factory failure");}catch(RuntimeException expected){if(expected!=IteratorCaptureOwner.failure||IteratorCaptureOwner.productions!=2)throw new AssertionError("factory failure identity");}`, 1)
	}
	switch shape {
	case "narrow-two-edge":
		f = strings.Replace(f, "static class DelegatingCursor<E> extends Cursor<E>", "static abstract class Middle<E> extends Cursor<E>{}static class DelegatingCursor<E> extends Middle<E>", 1)
		f = strings.Replace(f, "static <E>Cursor<E> narrow", "static <E>Middle<E> narrow", 1)
	case "narrow-array-key":
		f = strings.ReplaceAll(f, "Map<String,String>", "Map<String[],String>")
		f = strings.ReplaceAll(f, "View<String,String>", "View<String[],String>")
		f = strings.ReplaceAll(f, "Map.Entry<String,Set<String>>", "Map.Entry<String[],Set<String>>")
		f = strings.Replace(f, `String key=new String("key");`, `String[] key=new String[]{new String("key"),null};`, 1)
	case "narrow-member-factory":
		f = strings.Replace(f, "Iterator<Map.Entry<K,Set<V>>> iterator()", `Cursor<Map.Entry<K,V>> source(){return narrow(map.entrySet().iterator());}Iterator<Map.Entry<K,Set<V>>> iterator()`, 1)
		f = strings.Replace(f, "backing=narrow(map.entrySet().iterator())", "backing=source()", 1)
	case "renamed":
		f = strings.ReplaceAll(f, "IteratorCaptureOwner", "SequenceCaptureOwner")
		f = strings.ReplaceAll(f, "DelegatingCursor", "ForwardingSequence")
	}

	return f
}
func TestAdversarialNestedIteratorCaptureProducer(t *testing.T) {
	for _, shape := range []string{"plain", "narrow-factory", "narrow-two-edge", "narrow-array-key", "narrow-member-factory", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner := "IteratorCaptureOwner"
			if shape == "renamed" {
				owner = "SequenceCaptureOwner"
			}
			testNativePrivateSetterCompiledFixture(t, owner, "IteratorCaptureDriver", "6:iterator:capture:identity:scope\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": nestedIteratorCaptureShape(shape)}, debug, "8")
			})
		})
	}
}
func TestAdversarialNestedIteratorCaptureOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"plain", "narrow-factory", "narrow-two-edge", "narrow-array-key", "narrow-member-factory", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner := "IteratorCaptureOwner"
			if shape == "renamed" {
				owner = "SequenceCaptureOwner"
			}
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, nestedIteratorCaptureShape(shape), owner, debug)
			}, NativeJavac8, javac, []string{owner}, "IteratorCaptureDriver", "6:iterator:capture:identity:scope\n", nil, nativeLexicalExactSignatures)
		})
	}
}

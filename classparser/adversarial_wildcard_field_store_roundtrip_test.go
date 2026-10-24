package javaclassparser

import (
	"strings"
	"testing"
)

// The producer, generic factory, callbacks and driver remain original bytes.
// Only the consuming family is rebuilt. Nested invariant arguments, contravariant
// inputs, failure-atomic stores and overloads must retain the original values,
// object/throwable identities, null result and single evaluation of each input.
const invariantWildcardFieldFixture = `interface VarianceLoader<K,V>{ V load(K key); }
class VarianceFactory{static int calls;static <K,V> java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>> make(final VarianceLoader<? super K,V> loader){calls++;if(loader==null)return null;return new java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>>(){public java.util.Map<K,V> apply(Iterable<? extends K> keys){java.util.Map<K,V> map=new java.util.LinkedHashMap<K,V>();for(K key:keys)map.put(key,loader.load(key));return map;}};}}
class VarianceOwner{public static final class Holder<K,V>{final java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>> function;public Holder(VarianceLoader<? super K,V> loader){function=(java.util.function.Function)VarianceFactory.make(loader);}public java.util.Map<K,V> run(Iterable<? extends K> keys){return function.apply(keys);}}}
class VarianceDriver{public static void main(String[]args){final Object token=new Object();VarianceLoader<Object,Object> loader=new VarianceLoader<Object,Object>(){public Object load(Object key){return token;}};VarianceOwner.Holder<String,Object> holder=new VarianceOwner.Holder<String,Object>(loader);java.util.Map<String,Object> map=holder.run(java.util.Arrays.asList("a","b"));if(map.size()!=2||map.get("a")!=token||map.get("b")!=token||holder.getClass().getDeclaringClass()!=VarianceOwner.class)throw new AssertionError("binding/identity/ownership");if(new VarianceOwner.Holder<String,Object>(null).function!=null||VarianceFactory.calls!=2)throw new AssertionError("null/order");System.out.println("variance:2:identity:ownership:null:effects");}}`

func TestAdversarialWildcardFieldStorePreservesOriginalErasedFactory(t *testing.T) {
	for _, shape := range []string{"constructor", "assignment result and failure", "argument effects", "bounded overload"} {
		t.Run(shape, func(t *testing.T) {
			fixture := invariantWildcardFieldFixture
			switch shape {
			case "assignment result and failure":
				fixture = strings.Replace(fixture, "final java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>> function;", "java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>> function;", 1)
				fixture = strings.Replace(fixture, "class VarianceFactory{static int calls;", "class VarianceFactory{static int calls;static VarianceLoader<?,?> bad;static final RuntimeException failure=new RuntimeException(\"original factory failure\");", 1)
				fixture = strings.Replace(fixture, "calls++;if(loader==null)", "calls++;if(bad!=null&&loader==bad)throw failure;if(loader==null)", 1)
				method := `public java.util.function.Function<Iterable<? extends K>,java.util.Map<K,V>> replace(VarianceLoader<? super K,V> loader){return function=(java.util.function.Function)VarianceFactory.make(loader);}`
				fixture = strings.Replace(fixture, "public java.util.Map<K,V> run", method+"public java.util.Map<K,V> run", 1)
				check := `java.util.function.Function<Iterable<? extends String>,java.util.Map<String,Object>> before=holder.function;java.util.function.Function<Iterable<? extends String>,java.util.Map<String,Object>> replaced=holder.replace(loader);if(replaced==before||replaced!=holder.function||holder.run(java.util.Arrays.asList("a")).get("a")!=token)throw new AssertionError("assignment result/single producer");VarianceFactory.bad=loader;try{holder.replace(loader);throw new AssertionError("missing original failure");}catch(RuntimeException e){if(e!=VarianceFactory.failure||holder.function!=replaced)throw new AssertionError("throwable identity/failure-atomic field store");}VarianceFactory.bad=null;`
				fixture = strings.Replace(fixture, "if(new VarianceOwner.Holder<String,Object>(null)", check+"if(new VarianceOwner.Holder<String,Object>(null)", 1)
				fixture = strings.Replace(fixture, "VarianceFactory.calls!=2", "VarianceFactory.calls!=4", 1)
			case "argument effects":
				fixture = strings.Replace(fixture, "class VarianceFactory{static int calls;", `class VarianceFactory{static int calls,inputs;static <K,V> VarianceLoader<K,V> input(VarianceLoader<K,V> value){inputs++;return value;}`, 1)
				fixture = strings.Replace(fixture, "VarianceFactory.make(loader)", "VarianceFactory.make(VarianceFactory.input(loader))", 1)
				fixture = strings.Replace(fixture, "VarianceFactory.calls!=2", "VarianceFactory.calls!=2||VarianceFactory.inputs!=2", 1)
			case "bounded overload":
				fixture = strings.Replace(fixture, "class Holder<K,V>", "class Holder<K extends Number,V>", 1)
				fixture = strings.Replace(fixture, "class VarianceFactory{static int calls;", `class VarianceFactory{static int calls;static java.util.function.Function<Object,Object> make(Object wrong){throw new AssertionError("wrong original overload");}`, 1)
				fixture = strings.ReplaceAll(fixture, "Holder<String,Object>", "Holder<Integer,Object>")
				fixture = strings.ReplaceAll(fixture, "Map<String,Object>", "Map<Integer,Object>")
				fixture = strings.ReplaceAll(fixture, `java.util.Arrays.asList("a","b")`, `java.util.Arrays.asList(7,9)`)
				fixture = strings.ReplaceAll(strings.ReplaceAll(fixture, `map.get("a")`, `map.get(7)`), `map.get("b")`, `map.get(9)`)
			}
			for _, prefix := range []string{"Variance", "Erased"} {
				t.Run(prefix, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, strings.ReplaceAll(fixture, "Variance", prefix), []string{prefix + "Owner"}, prefix+"Driver", "variance:2:identity:ownership:null:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}

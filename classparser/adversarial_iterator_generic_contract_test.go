package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// CHECKCAST describes the use of an erased next() result, not the declaration
// of the iterator. In particular Iterator<Object> permits a checked String use,
// and ? extends Number must not lose its public generic contract because the
// bytecode explicitly casts its erased result to Number.
func TestAdversarialIteratorUseCastsPreserveGenericDeclarationAndEffects(t *testing.T) {
	const source = `public class IteratorContract {
public static long sum(java.util.Iterator<? extends Number> input){long result=0;while(input.hasNext()){result=result*31+input.next().longValue();}return result;}
public static <N extends Number> long generic(java.util.Iterator<N> input){return input.next().longValue();}
public static String text(java.util.Iterator<Object> input){return (String)input.next();}
public static int mixed(java.util.Iterator<Object> input){String a=(String)input.next();Integer b=(Integer)input.next();return a.length()+b.intValue();}
public static long reuse(java.util.List<String> left,java.util.List<Integer> right){long n=0;{java.util.Iterator<String> cursor=left.iterator();while(cursor.hasNext()){n+=cursor.next().length();}}{java.util.Iterator<Integer> cursor=right.iterator();while(cursor.hasNext()){n=n*31+cursor.next().intValue();}}return n;}
}
class CountingIterator<T> implements java.util.Iterator<T>{final T[] values;int position,calls;CountingIterator(T[]values){this.values=values;}public boolean hasNext(){return position<values.length;}public T next(){calls++;return values[position++];}public void remove(){throw new UnsupportedOperationException();}}
class IteratorDriver {public static void main(String[]args)throws Exception{
java.lang.reflect.Type sum=IteratorContract.class.getDeclaredMethod("sum",java.util.Iterator.class).getGenericParameterTypes()[0];if(!(sum instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("wildcard parameter erased");java.lang.reflect.Type wildcard=((java.lang.reflect.ParameterizedType)sum).getActualTypeArguments()[0];if(!(wildcard instanceof java.lang.reflect.WildcardType)||((java.lang.reflect.WildcardType)wildcard).getUpperBounds()[0]!=Number.class)throw new AssertionError("wildcard bound");
java.lang.reflect.Method generic=IteratorContract.class.getDeclaredMethod("generic",java.util.Iterator.class);java.lang.reflect.Type gp=generic.getGenericParameterTypes()[0];if(!(gp instanceof java.lang.reflect.ParameterizedType)||!((java.lang.reflect.ParameterizedType)gp).getActualTypeArguments()[0].equals(generic.getTypeParameters()[0])||generic.getTypeParameters()[0].getBounds()[0]!=Number.class)throw new AssertionError("generic binder");
for(String name:new String[]{"text","mixed"}){java.lang.reflect.Type p=IteratorContract.class.getDeclaredMethod(name,java.util.Iterator.class).getGenericParameterTypes()[0];if(!(p instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)p).getActualTypeArguments()[0]!=Object.class)throw new AssertionError("exact Object parameter");}
int rows=0;for(long a:new long[]{Long.MIN_VALUE,-7,0,Long.MAX_VALUE})for(long b:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){Number[] values=new Number[]{Long.valueOf(a),Long.valueOf(b),Integer.valueOf(7)};CountingIterator<Number> input=new CountingIterator<Number>(values);java.math.BigInteger expected=java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(b)).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7));if(IteratorContract.sum(input)!=expected.longValue()||input.calls!=3||input.position!=3)throw new AssertionError("sum/overflow/consumption");CountingIterator<Number> one=new CountingIterator<Number>(new Number[]{Long.valueOf(a)});if(IteratorContract.generic(one)!=a||one.calls!=1)throw new AssertionError("generic");rows++;}
String token=new String("identity");CountingIterator<Object> strings=new CountingIterator<Object>(new Object[]{token});if(IteratorContract.text(strings)!=token||strings.calls!=1)throw new AssertionError("identity");CountingIterator<Object> bad=new CountingIterator<Object>(new Object[]{new Object()});try{IteratorContract.text(bad);throw new AssertionError("cast missing");}catch(ClassCastException expected){}if(bad.calls!=1)throw new AssertionError("cast timing");CountingIterator<Object> mixed=new CountingIterator<Object>(new Object[]{"abc",Integer.valueOf(-9)});if(IteratorContract.mixed(mixed)!=-6||mixed.calls!=2)throw new AssertionError("mixed casts");if(IteratorContract.reuse(java.util.Arrays.asList("a","bb"),java.util.Arrays.asList(4,-9))!=((3L*31+4)*31-9))throw new AssertionError("distinct slot lifetimes");System.out.println(rows+":iterator:generic:casts:effects:overflow:lifetimes");}}
`
	for _, owner := range []string{"IteratorContract", "RenamedIteratorContract"} {
		for _, target := range []int{8, 16} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/target%d/%s", owner, target, debug), func(t *testing.T) {
					roundTripConstructorBridgeArtifact(t, strings.ReplaceAll(source, "IteratorContract", owner), owner, "IteratorDriver", "12:iterator:generic:casts:effects:overflow:lifetimes\n", debug, target)
				})
			}
		}
	}
}

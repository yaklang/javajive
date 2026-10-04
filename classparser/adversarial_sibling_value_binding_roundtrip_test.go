package javaclassparser

import "testing"

// Different subclasses and erased carriers share locals without changing
// their call receivers, payload checks, evaluation order or selected overloads.
func TestAdversarialSiblingValueBindingRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SiblingValueBinding", `import java.util.*;import java.lang.invoke.*;import java.nio.*;
abstract class BindingNode {abstract int value();}
class BindingFirst extends BindingNode {final int number;BindingFirst(int n){number=n;}int value(){return number;}}
class BindingSecond extends BindingNode {final BindingNode parent;BindingSecond(BindingNode p){parent=p;}int value(){return parent.value()*3;}}
interface BindingMatcher<T>{boolean matches(T value);}
class BindingOutputs<T>{String output(T value){return value==null?"nil":value.toString();}}
class SiblingBindingWitness {static String trace="";static final RuntimeException failure=new RuntimeException("same");static BindingFirst first(int n){trace+="F";if(n==7)throw failure;return new BindingFirst(n);}static BindingSecond second(BindingNode n){trace+="S";return new BindingSecond(n);}static int take(BindingNode n){trace+="B";return n.value();}static int take(BindingFirst n){trace+="X";return -999;}static void buffer(ByteBuffer b){trace+="H";if(b==null)throw failure;b.put(0,(byte)11);}}
public class SiblingValueBinding {
 static int node(int n,boolean wrap){BindingNode current=SiblingBindingWitness.first(n);if(wrap)current=SiblingBindingWitness.second(current);return SiblingBindingWitness.take(current);}
 static boolean collection(BindingMatcher<? super Iterable<? extends Number>> matcher,Collection<? extends Number> input){List<Number> copy=new ArrayList<>();for(Number n:input)copy.add(n);return matcher.matches(copy);}
 static <T> String generic(BindingOutputs<T> output,T first,T second,boolean choose){T current=choose?first:second;return output.output(current);}
 static int scoped(Collection<String> names,Collection<Integer> numbers){int total=0;for(String name:names)total+=name.length();for(Integer number:numbers)total+=number.intValue();return total;}
 static void nestedHandle(MethodHandle handle,ByteBuffer buffer)throws Throwable{final MethodHandle chosen=handle;final ByteBuffer target=buffer;Runnable invoke=()->{try{chosen.invokeExact(target);}catch(Throwable failure){SiblingValueBinding.<RuntimeException>raise(failure);}};invoke.run();}
 static <E extends Throwable> void raise(Throwable e)throws E{throw (E)e;}
 public static void main(String[]args)throws Throwable {
  for(int n:new int[]{-3,0,2,7})for(boolean wrap:new boolean[]{false,true}){SiblingBindingWitness.trace="";try{System.out.println(node(n,wrap)+":"+SiblingBindingWitness.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e==SiblingBindingWitness.failure)+":"+SiblingBindingWitness.trace);}}
  BindingMatcher<Iterable<? extends Number>> matcher=value->{int count=0;for(Number n:value)count+=n==null?0:n.intValue();return count==5;};for(Collection input:new Collection[]{Arrays.asList(2,3),Arrays.asList(2,null,3),Arrays.asList("bad"),null}){try{System.out.println(collection(matcher,input));}catch(Throwable e){System.out.println(e.getClass().getName());}}
  BindingOutputs<Object> outputs=new BindingOutputs<>();for(boolean choose:new boolean[]{false,true})System.out.println(generic(outputs,"a",Integer.valueOf(5),choose));System.out.println(generic(outputs,null,null,true));
  for(Collection names:new Collection[]{Arrays.asList("a","bc"),Arrays.asList(2),null}){try{System.out.println(scoped(names,Arrays.asList(3,4)));}catch(Throwable e){System.out.println(e.getClass().getName());}}
  MethodHandle handle=MethodHandles.lookup().findStatic(SiblingBindingWitness.class,"buffer",MethodType.methodType(void.class,ByteBuffer.class));ByteBuffer buffer=ByteBuffer.allocate(1);for(int mode=0;mode<3;mode++){SiblingBindingWitness.trace="";try{nestedHandle(mode==2?null:handle,mode==1?null:buffer);System.out.println(buffer.get(0)+":"+SiblingBindingWitness.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e==SiblingBindingWitness.failure)+":"+SiblingBindingWitness.trace);}}
 }
}`, Precision, Compatibility, "legacy")
}

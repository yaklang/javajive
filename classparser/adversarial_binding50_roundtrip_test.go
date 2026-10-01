package javaclassparser

import "testing"

func TestAdversarialCheckedDeclarationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "org.assertj.probe.CheckedDeclaration", `package org.assertj.probe;
import java.util.*;
public class CheckedDeclaration {
 private static <K,V> Map<K,V> clone(Map<K,V> value) throws NoSuchMethodException {
  if(isMultiValueMapAdapterInstance(value)) throw new NoSuchMethodException("missing");
  return value;
 }
 static boolean isMultiValueMapAdapterInstance(Map<?,?> value) { return value==null; }
 static String run(int mode) {
  Map<String,Integer> value=mode==0?null:new LinkedHashMap<String,Integer>();
  try { return "ok:"+clone(value).size(); }
  catch(NoSuchMethodException failure) { return "caught:"+failure.getMessage(); }
 }
 public static void main(String[]args) {for(int i=0;i<4;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericMethodDeclarationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericMethodDeclaration", `public class GenericMethodDeclaration {
 public static <T extends Number> T convertNumberToTargetClass(Number value,Class<T> target) { return target.cast(value); }
 static String run(int mode) {
  Number value=mode==0?null:mode==1?Integer.valueOf(3):Double.valueOf(2.5);
  Class<? extends Number> target=mode==3?null:mode==2?Double.class:Integer.class;
  try{return "ok:"+convertNumberToTargetClass(value,target);}catch(Throwable failure){return failure.getClass().getName();}
 }
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCtorNullCheckLengthRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CtorNullCheckLength", `import java.util.*;
public class CtorNullCheckLength {
 static int trace;final int length;
 private CtorNullCheckLength(int length) { trace=trace*10+2;this.length=length; }
 public CtorNullCheckLength(String... values) { this(Objects.requireNonNull(values,"values").length);trace=trace*10+3; }
 static String[] input(int mode){trace=trace*10+1;return mode==0?null:mode==1?new String[0]:new String[]{"a","b"};}
 static String run(int mode){trace=0;try{return "ok:"+new CtorNullCheckLength(input(mode)).length+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialInvariantReturnCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InvariantReturnCast", `import java.util.*;
public class InvariantReturnCast {
 static int trace;
 static <T> Iterator<T> combine(Iterator<? extends T>[] values){trace=trace*10+2;return values[0]==null?null:(Iterator<T>)values[0];}
 static <T> Iterator<T> join(Iterator<? extends T> first,Iterator<? extends T> second){return (Iterator<T>)(Iterator)combine(new Iterator[]{first,second});}
 static <T> Set<T> view(Collection<T> values){trace=trace*10+3;return (Set<T>)(Set)Collections.unmodifiableSet(new HashSet<Object>(values));}
 static Iterator<Integer> input(int mode){trace=trace*10+1;return mode==0?null:Arrays.asList(mode).iterator();}
 static String run(int mode){trace=0;try{Iterator<Integer>it=join(input(mode),null);return "ok:"+(it==null?null:it.next())+":"+view(Arrays.asList(mode)).size()+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialErasedMethodInputRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedMethodInput", `import java.util.*;import java.util.function.*;
public class ErasedMethodInput {
 static int trace;
 static <E> Optional<String> apply(Consumer<? super E> action,E value){trace=trace*10+2;action.accept(value);return Optional.of("generic");}
 static Optional<String> apply(Consumer action,String value){trace=trace*10+8;return Optional.of("overload");}
 static <E> String run(Consumer<? super E> action,Object value){Function<Object,Optional> f=x->apply((Consumer)action,x);return f.apply(value)+":"+trace;}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"text":Integer.valueOf(7);}
 public static void main(String[]args){for(int i=0;i<4;i++){trace=0;try{System.out.println(i+":"+run((String x)->{trace=trace*10+3;},input(i)));}catch(Throwable failure){System.out.println(i+":"+failure.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGuardedMethodReferenceRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GuardedMethodReference", `import java.util.*;import java.util.stream.*;
public class GuardedMethodReference {
 static int trace;
 static String text(int mode){trace=trace*10+1;return mode==0?null:mode==1?"A":"z";}
 static boolean match(String text){return text==null || Stream.of("a").anyMatch(text::equalsIgnoreCase);}
 static boolean mapped(List<String> values){return values!=null && values.stream().map(String::isEmpty).anyMatch(Boolean::booleanValue);}
 static String run(int mode){trace=0;try{return "ok:"+match(text(mode))+":"+mapped(mode==3?null:Collections.singletonList(mode==2?"a":""))+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedFactoryArrayTypeRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedFactoryArrayType", `class FactoryRoot$Part {}
class FactoryRoot$Part$Cell {
 int value;FactoryRoot$Part$Cell(int value){NestedFactoryArrayType.trace=NestedFactoryArrayType.trace*10+1;this.value=value;}
 FactoryRoot$Part$Cell[] children(){NestedFactoryArrayType.trace=NestedFactoryArrayType.trace*10+2;return value==0?null:new FactoryRoot$Part$Cell[]{this};}
}
public class NestedFactoryArrayType {
 static int trace;
 static String run(int mode){trace=0;try{FactoryRoot$Part$Cell[] cells=new FactoryRoot$Part$Cell(mode).children();return "ok:"+cells.length+":"+cells[0].value+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialDiscardedGenericMapResultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DiscardedGenericMapResult", `import java.util.*;
enum DiscardedKey {A,B}
public class DiscardedGenericMapResult {
 static int trace;
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?DiscardedKey.A:"wrong";}
 static String run(int mode){trace=0;Map<String,Integer> m=new LinkedHashMap<String,Integer>();m.put(null,4);((Map)m).put(null,7);EnumMap<DiscardedKey,Integer> e=new EnumMap<DiscardedKey,Integer>(DiscardedKey.class);
 try {((EnumMap)e).put((Enum)input(mode),9);trace=trace*10+2;return "ok:"+m.get(null)+":"+e.size()+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+m.get(null)+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericSelfMethodReferenceRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericSelfMethodReference", `import java.util.*;
public class GenericSelfMethodReference<K,V> {
 static int trace; V last;
 V put(K key,V value){trace=trace*10+2;last=value;return value;}
 void copy(Map<? extends K,? extends V> values){values.forEach(this::put);}
 static String run(int mode){trace=0;GenericSelfMethodReference<String,String> view=new GenericSelfMethodReference<String,String>();Map values=new LinkedHashMap();values.put(mode==0?null:"key",mode==1?Integer.valueOf(3):"value");
 try{view.copy(values);String result=view.last;trace=trace*10+3;return "ok:"+result+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoxedSpecialLambdaInputRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoxedSpecialLambdaInput", `import java.io.*;
interface BoxAction<T>{void apply(T value)throws IOException;}
public class BoxedSpecialLambdaInput extends StringWriter {
 BoxAction<Integer> action(){return super::write;}
 static String run(int mode){BoxedSpecialLambdaInput writer=new BoxedSpecialLambdaInput();BoxAction raw=writer.action();Object value=mode==0?null:mode==1?"wrong":Integer.valueOf(65+mode);
 try{raw.apply(value);return "ok:"+writer.toString();}catch(Throwable failure){return failure.getClass().getName()+":"+writer.toString();}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGuardedCapturedLambdaRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GuardedCapturedLambda", `import java.util.*;import java.util.function.*;
public class GuardedCapturedLambda {
 static int trace;
 static boolean accepted(List<Predicate<String>> filters,String value){return !filters.isEmpty() && filters.stream().allMatch(filter->filter.test(value));}
 static String run(int mode){trace=0;List<Predicate<String>> filters=mode==0?Collections.emptyList():Collections.singletonList(x->{trace=trace*10+2;return x.length()>0;});
 try{return "ok:"+accepted(filters,mode==1?null:mode==2?"":"text")+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialInvariantConstructorArgumentRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InvariantConstructorArgument", `import java.util.*;
public class InvariantConstructorArgument<T> {
 static int trace; final List<List<T>> values;
 InvariantConstructorArgument(List<List<T>> values){trace=trace*10+2;this.values=values;}
 InvariantConstructorArgument(List<T>[] values){this((List<List<T>>)(List)Arrays.asList(values));trace=trace*10+3;}
 static List[] input(int mode){trace=trace*10+1;return mode==0?null:mode==1?new List[0]:new List[]{Arrays.asList(mode)};}
 static String run(int mode){trace=0;try{return "ok:"+new InvariantConstructorArgument(input(mode)).values.size()+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundedBinaryMethodReferenceRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundedBinaryMethodReference", `import java.util.*;import java.util.function.*;
public class BoundedBinaryMethodReference {
 static int trace;
 static <E,C extends Collection<E>> C merge(C left,C right){trace=trace*10+2;left.addAll(right);return left;}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"wrong":new ArrayList(mode==3?Collections.singletonList(Integer.valueOf(7)):Collections.singletonList("a"));}
 static String run(int mode){trace=0;BinaryOperator<Collection<String>> operation=BoundedBinaryMethodReference::merge;
 try{Collection result=(Collection)((BinaryOperator)operation).apply(input(mode),Collections.singletonList("b"));String first=(String)result.iterator().next();trace=trace*10+3;return "ok:"+first+":"+result.size()+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSelfPackedGenericArrayRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SelfPackedGenericArray", `import java.util.*;
public class SelfPackedGenericArray<K,SELF extends SelfPackedGenericArray<K,SELF>> {
 static int trace;int count;
 SELF use(K... values){trace=trace*10+2;count=values.length;return (SELF)this;}
 SELF from(List<K> values){return use((K[])values.toArray());}
 static List input(int mode){trace=trace*10+1;return mode==0?null:mode==1?Collections.emptyList():Collections.singletonList(mode==2?"text":Integer.valueOf(3));}
 static String run(int mode){trace=0;SelfPackedGenericArray view=new SelfPackedGenericArray();try{return "ok:"+((SelfPackedGenericArray)view.from(input(mode))).count+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialErasedFunctionResultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedFunctionResult", `import java.util.*;import java.util.function.*;
public class ErasedFunctionResult {
 static int trace;
 static <T,R> R apply(Function<? super T,? extends R> action,T value){trace=trace*10+2;return action.apply(value);}
 static <T,R> Function<Object,Object> bridge(Function<? super T,? extends R> action){return x->apply((Function)action,x);}
 static <T> Supplier<T> identity(Supplier<T> action){return ()->action.get();}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"s":Integer.valueOf(9);}
 static String run(int mode){trace=0;try{Object result=bridge((String s)->{trace=trace*10+3;return s==null?"null":s.toUpperCase();}).apply(input(mode));return "ok:"+result+":"+identity(()->"kept").get()+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialMixedGenericReceiverRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "MixedGenericReceiver", `import java.util.*;import java.util.function.*;
public class MixedGenericReceiver<T> {
 static int trace; final Object value;
 public MixedGenericReceiver(Object value){this.value=value;}
 public <R> List<R> collect(Function<? super T,? extends List<? extends R>> action){trace=trace*10+2;return (List<R>)action.apply((T)value);}
 static <T,R> List<R> bridge(MixedGenericReceiver<T> receiver,Function<? super T,? extends List<? extends R>> action){return (List<R>)(List)((MixedGenericReceiver)receiver).collect((Function)action);}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"s":Integer.valueOf(9);}
 static String run(int mode){trace=0;try{List<String> result=bridge(new MixedGenericReceiver<String>(input(mode)),(String s)->{trace=trace*10+3;return Collections.singletonList(s==null?"null":s.toUpperCase());});return "ok:"+result+":"+trace;}catch(Throwable failure){return failure.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundMethodReferenceResultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundMethodReferenceResult", `import java.util.*;import java.util.function.*;
class ResultFactory<T>{static int trace;public T build(Object value){trace=trace*10+2;return (T)value;}}
public class BoundMethodReferenceResult {
 static Object input(int mode){ResultFactory.trace=ResultFactory.trace*10+1;return mode==0?null:mode==1?Collections.singletonMap("a",1):"bad";}
 static String run(int mode){ResultFactory.trace=0;try{ResultFactory<Map> factory=mode==3?null:new ResultFactory<Map>();Function<Object,Map> action=factory::build;Map value=action.apply(input(mode));ResultFactory.trace=ResultFactory.trace*10+3;return "ok:"+(value==null?null:value.size())+":"+ResultFactory.trace;}catch(Throwable failure){return failure.getClass().getName()+":"+ResultFactory.trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericBoundFieldRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericBoundField", `import java.util.*;
class BoundSink<E extends CharSequence>{static int trace;String last;public E create(Object value){trace=trace*10+2;return (E)value;}public void accept(E value){trace=trace*10+3;last=value==null?"null":value.toString();}}
public class GenericBoundField<O extends BoundSink<E>,E extends CharSequence> {
 final O target;GenericBoundField(O target){this.target=target;}
 void accept(Object value){CharSequence entry=target.create(value);target.accept((E)entry);}
 static Object input(int mode){BoundSink.trace=BoundSink.trace*10+1;return mode==0?null:mode==1?"text":Integer.valueOf(9);}
 static String run(int mode){BoundSink.trace=0;BoundSink<String> sink=new BoundSink<String>();try{new GenericBoundField<BoundSink<String>,String>(sink).accept(input(mode));return "ok:"+sink.last+":"+BoundSink.trace;}catch(Throwable failure){return failure.getClass().getName()+":"+BoundSink.trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialFixedGenericFieldReceiverRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FixedGenericFieldReceiver", `import java.util.*;
class FixedFieldBase {final Map<Class<?>,Integer> values=new LinkedHashMap<Class<?>,Integer>();}
class FixedFieldOwner extends FixedFieldBase {}
public class FixedGenericFieldReceiver extends FixedFieldOwner {
 static String run(int mode){FixedGenericFieldReceiver owner=new FixedGenericFieldReceiver();FixedFieldOwner alias=owner;Class<?> key=mode==0?null:String.class;owner.values.put(null,2);alias.values.put(key,7);return owner.values.size()+":"+alias.values.get(key);}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialEmbeddedCounterAfterCheckedCallRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "EmbeddedCounterAfterCheckedCall", `import java.io.*;
public class EmbeddedCounterAfterCheckedCall {
 static int trace;
 static void prepare(int mode)throws IOException {trace=trace*10+1;if(mode==3)throw new IOException("bad");}
 static String run(int mode){trace=0;int[] a=mode==0?new int[0]:mode==1?new int[]{0,1,2}:new int[]{2,3,4};
 try{prepare(mode);int n=0,i=0,v;while(i<a.length){if((v=a[i])!=0&&v!=7){if(v==1||v==6)n++;i++;}else throw new IOException("zero");}
 boolean[] out=new boolean[a.length*3-n];int p=0;for(i=0;i<a.length;i++){v=a[i];out[p++]=v>1;}
 return "ok:"+p+":"+n+":"+out.length+":"+trace;}
 catch(IOException e){return "caught:"+e.getMessage()+":"+trace;}}
 public static void main(String[] args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBreakArmValueJoinRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BreakArmValueJoin", `public class BreakArmValueJoin {
 static int trace;
 static double input(int mode,int round){trace=trace*10+1;return mode==0?0.2:mode==1?0.7:0.9;}
 static String run(int mode){trace=0;double v, u=0;int round=0;
 do{double a=input(mode,round++);if(a<0.5){if(round<2)continue;}else{if(a<0.8){u=a+3;v=a*7;break;}else if(round<2)continue;}
 u=a+4;v=a*9;if(round>=2)break;}while(true);
 v=Math.min(v,20);return "ok:"+v+":"+u+":"+trace;}
 public static void main(String[]args){for(int i=0;i<3;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialFixedNestedWildcardConstructorRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FixedNestedWildcardConstructor", `import java.util.function.*;
class FixedWildcardConsumer {
 final Predicate<Class<?>> action;
 FixedWildcardConsumer(Predicate<Class<?>> action){this.action=action;FixedNestedWildcardConstructor.trace=FixedNestedWildcardConstructor.trace*10+2;}
 boolean run(Class<?> value){return action.test(value);}
}
public class FixedNestedWildcardConstructor {
 static int trace;
 static String run(int mode){trace=0;Predicate<Class<?>> action=x->{trace=trace*10+3;return x==null||x==String.class;};
 try {FixedWildcardConsumer c=new FixedWildcardConsumer(action);return "ok:"+c.run(mode==0?null:mode==1?String.class:Integer.class)+":"+trace;}catch(Throwable t){return t.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundedReferenceLambdaEntryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundedReferenceLambdaEntry", `class BoundItem {final int value;BoundItem(int n){value=n;}}
class BoundLeaf extends BoundItem {BoundLeaf(int n){super(n);}String detail(){return "leaf:"+value;}}
interface BoundAction<E extends BoundItem>{String apply(E value);}
public class BoundedReferenceLambdaEntry {
 static int trace;
 static BoundAction<BoundLeaf> action(int n){return x->{trace=trace*10+2;return x.detail()+":"+n;};}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?new BoundItem(3):new BoundLeaf(mode);}
 static String run(int mode){trace=0;BoundAction<BoundLeaf> value=x->{trace=trace*10+2;return x.detail()+":"+mode;};BoundAction raw=value;try{return "ok:"+raw.apply((BoundItem)input(mode))+":"+trace;}catch(Throwable t){return t.getClass().getName()+":"+trace;}}
 public static void main(String[] args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedNumericWebRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedNumericWeb", `public class NestedNumericWeb {
 static long run(int mode) {
  long value;
  if(mode<2) { if(mode==0) value=11L; else value=13L; }
  else { if(mode==2) value=17L; else value=19L; }
  return value;
 }
 static long checked(int mode) {
  long value;
  try { if(mode<0) throw new IllegalArgumentException(); value=mode+23L; }
  catch(IllegalArgumentException e) { value=29L; }
  return value;
 }
 public static void main(String[]args) {for(int i=-1;i<5;i++)System.out.println(i+":"+run(i)+":"+checked(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialErasedContainerLambdaResultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedContainerLambdaResult", `import java.util.*;import java.util.function.*;
class ErasedCell<E>{E value;ErasedCell(E value){this.value=value;}}
public class ErasedContainerLambdaResult {
 static int trace;
 static <E> Queue<ErasedCell<E>> put(Queue<ErasedCell<E>> queue,E value){trace=trace*10+2;queue.add(new ErasedCell<E>(value));return queue;}
 static <E> Queue apply(Queue<ErasedCell<E>> queue,Object value){Function<Object,Queue> action=x->put((Queue)queue,x);return action.apply(value);}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"text":Integer.valueOf(mode);}
 static String run(int mode){trace=0;Queue<ErasedCell<String>> queue=new LinkedList<ErasedCell<String>>();try{Queue result=apply(queue,input(mode));return "ok:"+(result==queue)+":"+((ErasedCell)queue.remove()).value+":"+trace;}catch(Throwable t){return t.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialInvariantCachedFieldRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InvariantCachedField", `import java.util.*;
class CacheEntry<E> {E value;CacheEntry(E value){this.value=value;}public String toString(){return String.valueOf(value);}}
class CacheSource<E> {final Set<CacheEntry<E>> entries;CacheSource(Set<CacheEntry<E>> entries){this.entries=entries;}Set<CacheEntry<E>> entries(){return entries;}}
public class InvariantCachedField<E> {
 static int trace;Set<CacheEntry<E>> cached;final CacheSource<? extends E> source;
 InvariantCachedField(CacheSource<? extends E> source){this.source=source;}
 Set<CacheEntry<E>> values(){Set<CacheEntry<E>> value=cached;return value==null?(cached=(Set)Collections.unmodifiableSet(source.entries())):value;}
 static String run(int mode){trace=0;Set<CacheEntry<String>> entries=new LinkedHashSet<CacheEntry<String>>();entries.add(new CacheEntry<String>("a"));CacheSource<String> source=mode==0?null:new CacheSource<String>(entries);InvariantCachedField<String> instance=new InvariantCachedField<String>(source);
 try{Set<CacheEntry<String>> first=instance.values();boolean same=first==instance.values();trace=trace*10+1;try{first.clear();}catch(UnsupportedOperationException e){trace=trace*10+2;}return "ok:"+first+":"+same+":"+trace;}
 catch(Throwable t){return t.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialErasedConstructorMethodFormalRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedConstructorMethodFormal", `import java.util.function.*;
class MethodFormalBox {final Object value;final Predicate action;<T> MethodFormalBox(T value,Predicate<T> action){this.value=value;this.action=action;}MethodFormalBox(String value,Object action){this.value="overload";this.action=x->false;}boolean run(){return action.test(value);}}
public class ErasedConstructorMethodFormal {
 static int trace;
 static <T> String create(T value,Predicate<T> action){MethodFormalBox box=new MethodFormalBox(value,action);return "ok:"+box.run()+":"+trace;}
 public static void main(String[]args){for(int i=0;i<4;i++){trace=0;String value=i==0?null:i==1?"x":"";try{System.out.println(i+":"+create(value,x->{trace=trace*10+1;return x==null||x.length()>0;}));}catch(Throwable t){System.out.println(t.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialExternalErasedFunctionChainRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "ExternalErasedFunctionChain", `import java.util.function.*;
interface NarrowAction extends Function<Object,ChainBox>{}
class ChainBox<T>{final T value;ChainBox(T value){this.value=value;}}
class ChainStream<T>{final T value;ChainStream(T value){this.value=value;}static <T> ChainStream<T> from(T value){return new ChainStream<T>(value);}<R> ChainStream<R> flat(Function<? super T,? extends ChainBox<? extends R>> action){return new ChainStream<R>(action.apply(value).value);}ChainStream<String> flat(Object action){throw new AssertionError("wrong Object overload");}ChainStream<String> flat(NarrowAction action){throw new AssertionError("wrong narrow overload");}ChainStream<T> mark(Consumer<? super T> action){action.accept(value);return this;}}
public class ExternalErasedFunctionChain {
 static int trace;
 static ChainStream<String> convert(Object value){Function<Object,ChainBox> action=x->{trace=trace*10+2;return new ChainBox<String>((String)x);};Consumer<String> mark=x->{trace=trace*10+3;};return (ChainStream)ChainStream.from(value).flat((Function)action).mark((Consumer)mark);}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"x":Integer.valueOf(mode);}
 public static void main(String[]args){for(int i=0;i<4;i++){trace=0;try{System.out.println(i+":"+convert(input(i)).value+":"+trace);}catch(Throwable t){System.out.println(i+":"+t.getClass().getName()+":"+trace);}}}
}`, func(name string) bool { return name != "ChainStream" }, Precision, Compatibility, "legacy")
}

func TestAdversarialNarrowArrayInitializerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NarrowArrayInitializer", `import java.util.*;
public class NarrowArrayInitializer {
 static int trace;
 static char[] choose(int mode){char value;if(mode<0)value='x';else if(mode==0)value='a';else value='\uffff';return new char[]{value};}
 static int next(int value){trace=trace*10+1;return value;}
 static String run(int value){trace=0;byte[] bytes=new byte[]{(byte)next(value)};short[] shorts=new short[]{(short)next(value)};char[] chars=new char[]{(char)next(value)};return Arrays.toString(bytes)+":"+Arrays.toString(shorts)+":"+(int)chars[0]+":"+trace;}
 public static void main(String[] args){for(int value:new int[]{-65537,-129,-1,0,127,128,32768,65535,65536})System.out.println(value+":"+(int)choose(value)[0]+":"+run(value));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNullOnlyLocalFlowRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NullOnlyLocalFlow", `import java.util.function.*;
class NullFlowToken {final String value;NullFlowToken(String value){this.value=value;}public String toString(){return value;}}
public class NullOnlyLocalFlow {
 static int trace;
 static String consume(NullFlowToken token){trace=trace*10+2;return "token:"+token;}
 static String consume(Object token){throw new AssertionError("wrong overload");}
 static NullFlowToken token(){trace=trace*10+1;return new NullFlowToken("fallback");}
 static String run(int mode){trace=0;NullFlowToken empty=null;NullFlowToken copy=empty;Supplier<String> action=()->consume(copy!=null?copy:token());String result=action.get();return result+":"+trace;}
 public static void main(String[] args){for(int i=0;i<3;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLongSwitchFallthroughRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LongSwitchFallthrough", `public class LongSwitchFallthrough {
 static long mix(byte[] data,int seed){long value=seed;int blocks=data.length/4;for(int i=0;i<blocks;i++){long chunk=data[i*4]&255L;chunk*=17L;chunk^=chunk>>>5;chunk*=17L;value^=chunk;value*=17L;}int tail=blocks*4;switch(data.length-tail){case 3:value^=(data[tail+2]&255L)<<16;case 2:value^=(data[tail+1]&255L)<<8;case 1:value^=data[tail]&255L;value*=17L;}value^=value>>>7;value*=17L;value^=value>>>7;return value;}
 public static void main(String[] args){for(int length=0;length<18;length++){byte[] data=new byte[length];for(int i=0;i<length;i++)data[i]=(byte)(i*73-11);for(int seed:new int[]{0,1,-1,19,Integer.MIN_VALUE})System.out.println(length+":"+seed+":"+mix(data,seed));}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialPinnedFluentArgumentsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "PinnedFluentArguments", `import java.util.function.*;
class PinnedBox<T>{final T value;PinnedBox(T value){this.value=value;}}
class PinnedStream<T>{final T value;PinnedStream(T value){this.value=value;}static <T>PinnedStream<T> from(T value){return new PinnedStream<T>(value);}<R>PinnedStream<R> flat(Function<? super T,? extends PinnedBox<? extends R>> fn){return new PinnedStream<R>(fn.apply(value).value);}PinnedStream<String> flat(Object fn){throw new AssertionError("wrong overload");}PinnedStream<T> mark(Class<?> type,Consumer<? super T> action){action.accept(value);return this;}PinnedStream<T> tail(Object... values){if(values.length!=1)throw new AssertionError("unpacked array");return this;}}
public class PinnedFluentArguments {
 static int trace;
 static void inspect(String value){trace=trace*10+3;if(value!=null)trace=trace*10+value.length();}
 static PinnedStream<String> convert(Object value){Function<Object,PinnedBox> action=x->{trace=trace*10+2;return new PinnedBox<String>((String)x);};return ((PinnedStream<String>)(PinnedStream)PinnedStream.from(value).flat((Function)action)).mark(String.class,(Consumer<String>)PinnedFluentArguments::inspect).tail(new Object[]{"tail"});}
 static Object input(int mode){trace=trace*10+1;return mode==0?null:mode==1?"ab":Integer.valueOf(mode);}
 public static void main(String[]args){for(int i=0;i<4;i++){trace=0;try{System.out.println(i+":"+convert(input(i)).value+":"+trace);}catch(Throwable t){System.out.println(i+":"+t.getClass().getName()+":"+trace);}}}
}`, func(name string) bool { return name != "PinnedStream" }, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundedResultIntoCallerFormalRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "BoundedResultIntoCallerFormal", `class BoundApi {
 static int trace;
 static <N extends Number>N choose(Number value,Class<N> target){trace=trace*10+2;return value==null?null:target.cast(value);}
 static Object choose(Object value,Object target){throw new AssertionError("wrong overload");}
}
public class BoundedResultIntoCallerFormal {
 static <T>T store(Object input,Class<T>target){T value=(T)input;value=(T)BoundApi.choose((Number)value,(Class)target);return value;}
 static <T>T direct(Object value,Class<T>target){return (T)BoundApi.choose((Number)value,(Class)target);}
 static Object input(int mode){BoundApi.trace=BoundApi.trace*10+1;return mode==0?null:mode==1?Integer.valueOf(7):mode==2?Long.valueOf(9):"bad";}
 static String run(int mode,boolean reassignment){BoundApi.trace=0;try{Object result=reassignment?store(input(mode),(Class)Integer.class):direct(input(mode),(Class)Integer.class);return "ok:"+result+":"+BoundApi.trace;}catch(Throwable t){return t.getClass().getName()+":"+BoundApi.trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(i+":"+run(i,true)+":"+run(i,false));}
}`, func(name string) bool { return name != "BoundApi" }, Precision, Compatibility, "legacy")
}

package javaclassparser

import "testing"

func TestAdversarialCheckedArrayShortCircuitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CheckedArrayShortCircuit", `import java.lang.reflect.*;
public class CheckedArrayShortCircuit {
 static String trace="";static Method field;
 public static Boolean check(Object o){trace+="call,";if(o==null)throw new IllegalStateException();return true;}
 static String run(Method target,Object payload){trace="";try{return (target!=null&&((Boolean)target.invoke(null,new Object[]{payload})).booleanValue())+":"+trace;}catch(IllegalAccessException e){return "access:"+trace;}catch(InvocationTargetException e){return "target:"+e.getCause().getClass().getName()+":"+trace;}}
 static boolean fieldRun(Object payload){try{return field!=null&&((Boolean)field.invoke(null,new Object[]{payload})).booleanValue();}catch(IllegalAccessException e){return false;}catch(InvocationTargetException e){throw new RuntimeException(e);}}
 public static void main(String[] args)throws Exception{Method target=CheckedArrayShortCircuit.class.getMethod("check",Object.class);System.out.println(run(null,"value"));System.out.println(run(target,"value"));System.out.println(run(target,null));field=null;System.out.println(fieldRun("value"));field=target;System.out.println(fieldRun("value"));try{System.out.println(fieldRun(null));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getCause().getClass().getName()+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundedSelfCallChainRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundedSelfCallChain", `interface SelfNumber<N extends SelfNumber<N>> {N divide(N other);}
class SelfValue implements SelfNumber<SelfValue>{static int trace;final int value;SelfValue(int n){value=n;}public SelfValue divide(SelfValue other){trace=trace*10+3;return new SelfValue(value/other.value);}public String toString(){return ""+value;}}
interface SelfVector<N extends SelfNumber<N>> {N dot(SelfVector<N> other);SelfVector<N> scale(N value);}
public class BoundedSelfCallChain<N extends SelfNumber<N>> implements SelfVector<N>{
 N value;BoundedSelfCallChain(N n){value=n;}public N dot(SelfVector<N> other){SelfValue.trace=SelfValue.trace*10+1;return value;}public SelfVector<N> scale(N n){SelfValue.trace=SelfValue.trace*10+2;return new BoundedSelfCallChain<N>(n);}SelfVector<N> projection(SelfVector<N> other){return other.scale(dot(other).divide(other.dot(other)));}
 static String run(int mode){SelfValue.trace=0;try{BoundedSelfCallChain<SelfValue> a=new BoundedSelfCallChain<SelfValue>(new SelfValue(8));SelfVector<SelfValue> b=new BoundedSelfCallChain<SelfValue>(mode==0?new SelfValue(2):null);SelfVector<SelfValue> c=a.projection(b);return ((BoundedSelfCallChain<SelfValue>)c).value+":"+SelfValue.trace;}catch(Throwable e){return e.getClass().getName()+":"+SelfValue.trace;}}
 public static void main(String[] args){for(int i=0;i<3;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialIntegerMaskZeroBranchRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "IntegerMaskZeroBranch", `public class IntegerMaskZeroBranch {
 static String scan(String text){StringBuilder result=new StringBuilder();int count=0;for(int i=0;i<text.length();i++){char c=text.charAt(i);switch(c){case '\\':count++;break;case '"':if(count==0)return "bare";count=0;break;default:count=0;break;}if((count&1)==0)result.append(c);}return result.toString();}
 public static void main(String[] args){for(String s:new String[]{"", "a", "\\", "\\\\", "\\\\\\", "a\\\\b", "\\\"", "\""})System.out.println(scan(s));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBoundedGenericStoreViewsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundedGenericStoreViews", `import java.util.*;
class GenericStoreCell<E extends Number> {E current;Iterator<E> values;GenericStoreCell(List<E> v){values=v.iterator();}}
public class BoundedGenericStoreViews<T extends Number> {
 GenericStoreCell<T>[] cells;T local;
 BoundedGenericStoreViews(List<T> values){cells=(GenericStoreCell<T>[])new GenericStoreCell[]{new GenericStoreCell<T>(values)};local=values.get(0);}
 void step(){Number next=(T)cells[0].values.next();cells[0].current=(T)next;local=(T)next;}
 void reassign(List<Number> updates){T current=local;for(Number next:updates){current=(T)next;}local=current;}
 static String run(List values){try{BoundedGenericStoreViews<Integer> b=new BoundedGenericStoreViews<Integer>(values);b.step();String before=b.cells[0].current+":"+b.local;b.reassign(Arrays.<Number>asList(9,null,Double.valueOf(2.5)));return before+":"+b.local;}catch(Throwable e){return e.getClass().getName();}}
 public static void main(String[] args){System.out.println(run(Arrays.asList(7)));System.out.println(run(Arrays.asList((Object)null)));System.out.println(run(Arrays.asList("bad")));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBranchArrayInitializerEffectsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BranchArrayInitializerEffects", `enum EffectToken { FIRST }
class EffectTokenApi {
 static int trace;
 static EffectToken element(int mode){trace=trace*10+2;if(mode==1)throw new IllegalStateException();return mode==2?null:EffectToken.FIRST;}
 static String accept(EffectToken... tokens){trace=trace*10+3;return tokens[0].name();}
 static String accept(Object token){trace=trace*10+9;return "wrong";}
}

public class BranchArrayInitializerEffects {
 static String run(boolean selected,int mode){EffectTokenApi.trace=0;try{String result=selected?EffectTokenApi.accept(new EffectToken[]{EffectTokenApi.element(mode)}):"skip";return result+":"+EffectTokenApi.trace;}catch(Throwable e){return e.getClass().getName()+":"+EffectTokenApi.trace;}}
 public static void main(String[] args){for(int mode=0;mode<4;mode++){System.out.println(run(false,mode));System.out.println(run(true,mode));}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBranchInitializedArrayCallRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BranchInitializedArrayCall", `enum BranchToken { FIRST, SECOND }
class BranchTokenApi {
 static int trace;
 static String accept(BranchToken... tokens){trace=trace*10+1;return tokens[0].name();}
 static String accept(Object token){trace=trace*10+9;return "wrong";}
}
public class BranchInitializedArrayCall {
 static String run(boolean selected){BranchTokenApi.trace=0;String result=selected?BranchTokenApi.accept(new BranchToken[]{BranchToken.FIRST}):"skip";return result+":"+BranchTokenApi.trace;}
 public static void main(String[] args){System.out.println(run(false));System.out.println(run(true));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialConditionalArrayOperandsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalArrayOperands", `import java.util.*;
class TraceArraySource<E> extends ArrayList<E> {
 static String trace="";final int id;final boolean fail;
 TraceArraySource(List<E> values,int id,boolean fail){super(values);this.id=id;this.fail=fail;}
 public boolean isEmpty(){trace+=id+"e,";return super.isEmpty();}
 public int size(){trace+=id+"s,";return super.size();}
 public <T>T[] toArray(T[] target){trace+=id+"a,";if(fail)throw new IllegalStateException();return super.toArray(target);}
}
public class ConditionalArrayOperands {
 static String accept(String[] a,Integer[] b){TraceArraySource.trace+="use,";return Arrays.toString(a)+":"+Arrays.toString(b);}
 static String run(int mode){TraceArraySource.trace="";List<String>a=new TraceArraySource<String>(mode==0?Collections.emptyList():Arrays.asList("x",null),1,mode==3);List<Integer>b=new TraceArraySource<Integer>(mode==1?Collections.emptyList():Arrays.asList(3,4),2,mode==4);
 try{return accept(a.isEmpty()?null:a.toArray(new String[a.size()]),b.isEmpty()?null:b.toArray(new Integer[b.size()]))+":"+TraceArraySource.trace;}catch(Throwable e){return e.getClass().getName()+":"+TraceArraySource.trace;}}
 public static void main(String[]args){for(int i=0;i<6;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialReferenceDefinitionJoinRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ReferenceDefinitionJoin", `public class ReferenceDefinitionJoin {
 static int trace;static String field="base";
 static String next(int mode){trace=trace*10+1;return mode==0?null:"value"+mode;}
 static String run(int mode){trace=0;String value=null;Object marker=mode==0?null:new Object();if(marker!=null)value=next(mode);
 if(value==null)value=field;value=transform(value);if(mode==1)value=null;
 return (value==field?"same":"different")+":"+value+":"+trace;}
 static String transform(String value){trace=trace*10+2;return value;}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCheckedErasedContainerUseRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "CheckedErasedContainerUse", `import java.util.*;import java.util.function.*;
class ContainerMaker {
 static int trace;
 static <T> Map<String,T> create(Function<Class<T>,T> f,Class<T> type){trace=trace*10+2;Map<String,T>result=new LinkedHashMap<String,T>();result.put("key",f.apply(type));return result;}
 static Map create(Object f,String type){trace=trace*10+9;return null;}
}

public class CheckedErasedContainerUse {
 static <T> LinkedHashMap<String,T> recover(Function<Class,Object> action,Class<T> type){return (LinkedHashMap<String,T>)(Map)ContainerMaker.create((Function)action,(Class)type);}
 static String run(int mode){ContainerMaker.trace=0;Function<Class,Object> f=c->{ContainerMaker.trace=ContainerMaker.trace*10+3;return mode==0?null:mode==1?Integer.valueOf(7):"text";};
 try{String result=recover(f,String.class).get("key");ContainerMaker.trace=ContainerMaker.trace*10+4;return "ok:"+result+":"+ContainerMaker.trace;}catch(Throwable e){return e.getClass().getName()+":"+ContainerMaker.trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(run(i));}
}`, func(name string) bool { return name != "ContainerMaker" }, Precision, Compatibility, "legacy")
}

func TestAdversarialBooleanCompoundAccessorRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BooleanCompoundAccessor", `public class BooleanCompoundAccessor {
 private boolean flag;
 static class Update {static boolean or(BooleanCompoundAccessor v,boolean b){return v.flag|=b;}static boolean and(BooleanCompoundAccessor v,boolean b){return v.flag&=b;}static boolean xor(BooleanCompoundAccessor v,boolean b){return v.flag^=b;}}
 public static void main(String[]args)throws Exception{for(String operation:new String[]{"or","and","xor"}){java.lang.reflect.Method m=Class.forName("BooleanCompoundAccessor$Update").getDeclaredMethod(operation,BooleanCompoundAccessor.class,boolean.class);m.setAccessible(true);for(int a=0;a<2;a++)for(int b=0;b<2;b++){BooleanCompoundAccessor v=new BooleanCompoundAccessor();v.flag=a!=0;System.out.println(operation+":"+a+":"+b+":"+m.invoke(null,v,b!=0)+":"+v.flag);}}java.lang.reflect.Method[] methods=BooleanCompoundAccessor.class.getDeclaredMethods();java.util.Arrays.sort(methods,java.util.Comparator.comparing(java.lang.reflect.Method::getName));for(java.lang.reflect.Method m:methods){Class<?>[] p=m.getParameterTypes();if(p.length==2&&p[0]==BooleanCompoundAccessor.class&&p[1]==int.class&&m.getReturnType()==boolean.class){m.setAccessible(true);for(int a=0;a<2;a++)for(int b:new int[]{-257,-256,-2,-1,0,1,2,3,127,128,255,256,257}){BooleanCompoundAccessor v=new BooleanCompoundAccessor();v.flag=a!=0;System.out.println(m.getName()+":"+a+":"+b+":"+m.invoke(null,v,b)+":"+v.flag);}}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialConditionalArrayReadCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalArrayReadCast", `class ReadNode {static int trace;static int mode;Object copy(){trace=trace*10+2;return mode==3?"wrong":mode==4?null:new ReadNode();}}
public class ConditionalArrayReadCast {
 static ReadNode[] nodes;
 static int index(){ReadNode.trace=ReadNode.trace*10+1;return ReadNode.mode==2?2:0;}
 static String run(int mode){ReadNode.trace=0;ReadNode.mode=mode;nodes=mode==1?null:new ReadNode[]{new ReadNode()};
 try{ReadNode result=mode==0?null:(ReadNode)nodes[index()].copy();ReadNode.trace=ReadNode.trace*10+3;return (result==null?"null":"node")+":"+ReadNode.trace;}catch(Throwable e){return e.getClass().getName()+":"+ReadNode.trace;}}
 public static void main(String[]args){for(int mode=0;mode<6;mode++)System.out.println(run(mode));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialProtectedRetryBranchesRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedRetryBranches", `import java.io.*;
class RetryTransport {
 static int trace,attempt,mode; static Object socket;
 static boolean tunnel(){trace=trace*10+1;return (mode&1)==0;}
 static void tunnelConnect()throws IOException {trace=trace*10+2;if(attempt==0&&mode==2)throw new IOException("tunnel");socket=mode==4?null:new Object();}
 static void plainConnect()throws IOException {trace=trace*10+3;if(mode==3)throw new IOException("plain");socket=new Object();}
 static void establish()throws IOException {trace=trace*10+4;if(attempt==0&&mode==6)throw new IOException("protocol");}
 static boolean retry(IOException e){trace=trace*10+5;return attempt++==0;}
 static void cleanup(){trace=trace*10+6;socket=null;}
}
public class ProtectedRetryBranches {
 static String connect(int mode){RetryTransport.mode=mode;RetryTransport.trace=0;RetryTransport.attempt=0;RetryTransport.socket=null;
 try{while(true){try{if(RetryTransport.tunnel()){RetryTransport.tunnelConnect();if(RetryTransport.socket==null)break;}else{RetryTransport.plainConnect();}RetryTransport.establish();break;}catch(IOException e){RetryTransport.cleanup();if(!RetryTransport.retry(e))throw new IllegalStateException(e);}}return "ok:"+(RetryTransport.socket!=null)+":"+RetryTransport.trace;}catch(Throwable e){return e.getClass().getName()+":"+RetryTransport.trace;}}
 public static void main(String[]args){for(int i=0;i<8;i++)System.out.println(connect(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedErasedInvocationTupleRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "NestedErasedInvocationTuple", `import java.util.*;import java.util.function.*;
class NestedFunctionApi {
 static int trace;
 static <X,Y>Function<X,Y> constant(Y value){trace=trace*10+1;return x->{trace=trace*10+3;return value;};}
}
public class NestedErasedInvocationTuple<T> {
 T value;NestedErasedInvocationTuple(T value){this.value=value;}
 NestedErasedInvocationTuple<T> choose(Function<? super Throwable,? extends T> action){value=action.apply(null);return this;}
 NestedErasedInvocationTuple<T> choose(T value){return choose(NestedFunctionApi.constant(value));}
 static String run(int mode){NestedFunctionApi.trace=0;NestedErasedInvocationTuple<String> v=new NestedErasedInvocationTuple<String>("base");try{return v.choose(mode==0?null:"new").value+":"+NestedFunctionApi.trace;}catch(Throwable e){return e.getClass().getName()+":"+NestedFunctionApi.trace;}}
 public static void main(String[]args){for(int i=0;i<3;i++)System.out.println(run(i));}
}`, func(name string) bool { return name != "NestedFunctionApi" }, Precision, Compatibility, "legacy")
}

func TestAdversarialDuplicatedValueDifferentStoreViewsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DuplicatedValueDifferentStoreViews", `class ViewBase {String text(){return "base";}}
class ViewChild extends ViewBase {String text(){return "child";}}
class ViewFactory {static int trace;static ViewChild make(int mode){trace++;return mode==0?null:new ViewChild();}}
public class DuplicatedValueDifferentStoreViews {
 static String run(int mode){ViewFactory.trace=0;ViewBase parent=null;ViewChild child=null;try{if(mode!=2){parent=child=ViewFactory.make(mode);}else{parent=new ViewBase();}return (child==parent)+":"+(parent==null?"null":parent.text())+":"+ViewFactory.trace;}catch(Throwable e){return e.getClass().getName()+":"+ViewFactory.trace;}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(run(i));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialDiscardedCheckedValidationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DiscardedCheckedValidation", `class CheckedValidationTarget {public CheckedValidationTarget(Object value){}}
public class DiscardedCheckedValidation {
 final Class<?> type;
 DiscardedCheckedValidation(Class<?> type){try{type.getConstructor(new Class<?>[]{Object.class});}catch(Exception e){throw new IllegalArgumentException("validate",e);}this.type=type;}
 static String run(Class<?> type){try{return new DiscardedCheckedValidation(type).type.getName();}catch(Throwable e){return e.getClass().getName()+":"+(e.getCause()==null?"null":e.getCause().getClass().getName());}}
 public static void main(String[]args){System.out.println(run(CheckedValidationTarget.class));System.out.println(run(String.class));System.out.println(run(null));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialPartialArrayHandlerObservationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "PartialArrayHandlerObservation", `public class PartialArrayHandlerObservation {
 static int trace;static String element(int mode){trace=trace*10+2;if(mode==1)throw new IllegalStateException();return mode==2?null:"item";}
 static String run(int mode){trace=0;String[] array=null;try{array=new String[2];array[0]="first";array[1]=element(mode);return array[0]+":"+array[1]+":"+trace;}catch(Throwable e){return (array==null?"null":array[0]+":"+array[1])+":"+trace+":"+e.getClass().getName();}}
 public static void main(String[]args){for(int mode=0;mode<4;mode++)System.out.println(run(mode));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericArrayWebStoreRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericArrayWebStore", `import java.util.*;
public class GenericArrayWebStore<T> {
 T[] row;static int trace;GenericArrayWebStore(T[] r){row=r;}
 static int index(int mode){trace=trace*10+1;return mode==3?4:0;}
 static Object item(Queue values){trace=trace*10+2;return values.poll();}
 String drain(Queue values,int mode){trace=0;Object[] local=row;try{local[index(mode)]=item(values);return Arrays.toString(local)+":"+trace;}catch(Throwable e){return e.getClass().getName()+":"+trace;}}
 public static void main(String[]args){for(int m=0;m<4;m++){GenericArrayWebStore<String> a=new GenericArrayWebStore<String>(m==2?null:new String[2]);System.out.println(a.drain(new LinkedList(Arrays.asList(m==1?7:"ok")),m));}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialDisjointReferenceSlotDomainsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DisjointReferenceSlotDomains", `import java.util.*;
public class DisjointReferenceSlotDomains {
 static String run(Set<Class<?>> types){Set<Object> modules=new HashSet<Object>();String trace="";try{for(Class<?> c:types){trace+=c.getName()+",";modules.add(c.getName());}for(Object m:modules){trace+=m+",";}return trace;}catch(Throwable e){return e.getClass().getName()+":"+trace;}}
 public static void main(String[]args){System.out.println(run(new LinkedHashSet<Class<?>>(Arrays.<Class<?>>asList(String.class,Integer.class))));System.out.println(run(new LinkedHashSet<Class<?>>(Arrays.<Class<?>>asList((Class<?>)null))));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialParameterizedReturnRawArrayFactoryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ParameterizedReturnRawArrayFactory", `import java.util.*;
class FactoryEntry<E> {final E value;FactoryEntry(E e){value=e;}public String toString(){return ""+value;}}
class FactoryBag<E> {static int trace;final Collection entries;FactoryBag(Collection c){entries=c;}static <E>FactoryBag<E> copy(Collection<? extends FactoryEntry<? extends E>> c){trace=trace*10+2;return new FactoryBag<E>(c);}static FactoryBag copy(Object o){trace=trace*10+9;return null;}public String toString(){return entries.toString();}}
public class ParameterizedReturnRawArrayFactory {
 static <E>FactoryBag<E> copy(List<FactoryEntry<E>> values){FactoryEntry[] entries=values.toArray(new FactoryEntry[0]);return FactoryBag.copy((Collection)Arrays.asList(entries));}
 public static void main(String[]args){FactoryBag.trace=0;System.out.println(copy(Arrays.asList(new FactoryEntry<String>("x"),new FactoryEntry<String>(null)))+":"+FactoryBag.trace);System.out.println(copy(Collections.<FactoryEntry<String>>emptyList())+":"+FactoryBag.trace);}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialParameterizedOverloadAfterReturnPlanningRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ParameterizedOverloadAfterReturnPlanning", `import java.util.*;import java.lang.reflect.*;
class PlannedBox<E>{final String text;PlannedBox(String t){text=t;}public String toString(){return text;}}
public class ParameterizedOverloadAfterReturnPlanning {
 static int trace;
 PlannedBox<?> build(List<? extends Type> values){trace=trace*10+2;return new PlannedBox<Object>(values.toString());}
 PlannedBox<?> build(Collection<? extends Runnable> values){trace=trace*10+9;return new PlannedBox<Object>("wrong");}
 <E>PlannedBox<E> build(Class<E> type){return (PlannedBox<E>)(PlannedBox)build(Collections.singletonList(type));}
 public static void main(String[]args){ParameterizedOverloadAfterReturnPlanning p=new ParameterizedOverloadAfterReturnPlanning();trace=0;System.out.println(p.build(String.class)+":"+trace);System.out.println(p.build((Class<?>)null)+":"+trace);}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSameNamedReflectionWrapperCatchRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SameNamedReflectionWrapperCatch", `import java.lang.reflect.*;
public class SameNamedReflectionWrapperCatch {
 static Class cached;static Class lookup(){return String.class;}
 private Method getMethod(Class owner,String name,Class[] params,boolean unused){try{return owner.getMethod(name,params);}catch(NoSuchMethodException e){throw new IllegalArgumentException(e);}}
 String read(Object target,String name){try{Class[] params=new Class[1];params[0]=cached==null?(cached=lookup()):cached;return "ok:"+getMethod(target.getClass(),name,params,false).invoke(target,new Object[]{"value"});}catch(IllegalAccessException e){return "access";}catch(InvocationTargetException e){return "target:"+e.getTargetException().getClass().getName();}}
 public static void main(String[]args){SameNamedReflectionWrapperCatch c=new SameNamedReflectionWrapperCatch();for(String s:new String[]{"echo","fail","missing"}){try{System.out.println(c.read(new ReflectionWrapperTarget(),s));}catch(Throwable e){System.out.println("escaped:"+e.getClass().getName());}}}
}
class ReflectionWrapperTarget {public String toString(){return "value";}public String echo(String s){return s;}public String fail(String s){throw new ArithmeticException("bad");}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialSharedBranchQualifiedInnerOwnerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowUnits(t, "SharedBranchQualifiedInnerOwner", `public class SharedBranchQualifiedInnerOwner {
 static SharedBranchQualifiedInnerOwner active;static int trace;
 class Item {final int value;Item(int n){trace=trace*10+2;value=n;}String read(){trace=trace*10+3;return "item:"+value;}}
 static int arg(){trace=trace*10+1;return 7;}
 static String run(int n,boolean a,boolean b,boolean c){return n!=1||!a||!b||!c?"bad":active.new Item(arg()).read();}
 public static void main(String[]args){for(int mask=0;mask<32;mask++){trace=0;active=(mask&16)==0?new SharedBranchQualifiedInnerOwner():null;String result;try{result=run((mask&8)==0?1:2,(mask&1)==0,(mask&2)==0,(mask&4)==0);}catch(Throwable e){result=e.getClass().getName();}System.out.println(mask+":"+result+":"+trace);}}
}`, nil, []string{"SharedBranchQualifiedInnerOwner$Item"}, Precision, Compatibility, "legacy")
}

func TestAdversarialFixedSubclassGenericReturnInputRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FixedSubclassGenericReturnInput", `import java.util.*;
class FixedInputBase<R>{R pass(R value){return value;}}
public class FixedSubclassGenericReturnInput extends FixedInputBase<List<Object>>{
 List<Object> run(boolean filled){return pass(filled?Arrays.<Object>asList("x",7):Collections.<Object>emptyList());}
 public static void main(String[]args){FixedSubclassGenericReturnInput c=new FixedSubclassGenericReturnInput();System.out.println(c.run(true));System.out.println(c.run(false));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialJavaLangExceptionNameShadowRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "shadow.JavaLangExceptionNameShadow", `package shadow;
class InstantiationException extends RuntimeException{InstantiationException(String s){super(s);}}
class ShadowFactory {static String create(boolean fail) throws java.lang.InstantiationException{if(fail)throw new java.lang.InstantiationException("checked");return "ok";}}
public class JavaLangExceptionNameShadow {
 static String run(boolean fail) throws java.lang.InstantiationException {if(fail)throw new InstantiationException("local");return ShadowFactory.create(fail);}
 public static void main(String[]args){for(boolean b:new boolean[]{false,true}){try{System.out.println(run(b));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCastedReflectionReceiverRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CastedReflectionReceiver", `import java.lang.reflect.*;
class ReflectionCtorHolder {final Constructor<?> ctor;ReflectionCtorHolder(Constructor<?> c){ctor=c;}Object make()throws Exception{return ctor.newInstance("trace");}}
public class CastedReflectionReceiver {
 static Object lookup(boolean missing)throws Exception{return new ReflectionCtorHolder(((Class<?>)Class.forName(missing?"no.such.Type":"java.lang.String")).getDeclaredConstructor(String.class)).make();}
 public static void main(String[]args){for(boolean missing:new boolean[]{false,true}){try{System.out.println(lookup(missing));}catch(Exception e){System.out.println(e.getClass().getName());}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNullOnlyLambdaCaptureDescriptorRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NullOnlyLambdaCaptureDescriptor", `import java.util.*;import java.util.function.*;
class CaptureLoaderOracle {static String choose(String label,Locale locale,ClassLoader loader){return label+":"+locale.getLanguage()+":"+(loader!=null);}static String choose(String label,Locale locale,Object other){return "WRONG";}}
public class NullOnlyLambdaCaptureDescriptor {
 static Supplier<String> build(String label,Locale locale){final ClassLoader loader=null;return ()->CaptureLoaderOracle.choose(label,locale,loader!=null?loader:ClassLoader.getSystemClassLoader());}
 public static void main(String[]args){System.out.println(build("ok",Locale.ENGLISH).get());System.out.println(build("other",Locale.FRENCH).get());}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCovariantDualInterfaceOverloadRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CovariantDualInterfaceOverload", `interface DualLeft {int value();DualLeft retain();}interface DualRight {int value();DualRight retain();}
class DualResource implements DualLeft,DualRight {int count;public int value(){return count;}public DualResource retain(){count++;return this;}}
class DualOverloads {static String choose(Object allocator,DualLeft left){return "left:"+left.value();}static String choose(Object allocator,DualRight right){return "WRONG:"+right.value();}}
public class CovariantDualInterfaceOverload {
 static String run(DualResource resource){return DualOverloads.choose(new Object(),((DualLeft)resource).retain());}
 public static void main(String[]args){DualResource resource=new DualResource();System.out.println(run(resource));System.out.println(run(resource));try{run(null);}catch(Throwable e){System.out.println(e.getClass().getName());}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialInterruptedPollRetryFinallyRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InterruptedPollRetryFinally", `import java.util.*;import java.util.concurrent.*;
class RetryingPollQueue extends LinkedBlockingQueue<String>{int calls;final int mode;RetryingPollQueue(int m){mode=m;}public String poll(long time,TimeUnit unit)throws InterruptedException{calls++;if(calls==1)throw new InterruptedException("retry");if(mode==2)throw new IllegalStateException("terminal");return mode==1||calls>3?null:"value";}}
public class InterruptedPollRetryFinally {
 static int drain(BlockingQueue<String> queue,Collection<String> output,int max){int count=0;boolean interrupted=false;try{while(count<max){count+=queue.drainTo(output,max-count);if(count<max){String value;while(true){try{value=queue.poll(1,TimeUnit.NANOSECONDS);break;}catch(InterruptedException e){interrupted=true;}}if(value==null)break;output.add(value);count++;}}}finally{if(interrupted)Thread.currentThread().interrupt();}return count;}
 public static void main(String[]args){for(int mode=0;mode<3;mode++){Thread.interrupted();RetryingPollQueue queue=new RetryingPollQueue(mode);List<String> output=new ArrayList<>();String result;try{result="n="+drain(queue,output,2);}catch(Throwable e){result=e.getClass().getName();}System.out.println(mode+":"+result+":"+queue.calls+":"+output+":"+Thread.interrupted());}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialReflectiveWrapperExceptionPropagationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ReflectiveWrapperExceptionPropagation", `public class ReflectiveWrapperExceptionPropagation {
 Object build(String name)throws Exception{return Class.forName(name).getConstructor(new Class[0]).newInstance(new Object[0]);}
 Object make(String name)throws Exception{try{return this.build(name);}catch(ClassNotFoundException e){throw new RuntimeException(e);}}
 public static void main(String[]args){for(String name:new String[]{"java.lang.String","java.lang.Integer","no.such.Class"}){try{Object value=new ReflectiveWrapperExceptionPropagation().make(name);System.out.println("ok:"+value.getClass().getName());}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e.getCause()==null?"no-cause":e.getCause().getClass().getName()));}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialDiscardedGenericStackResultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DiscardedGenericStackResult", `import java.util.*;
public class DiscardedGenericStackResult {
 static String run(Stack<String> sink,Object payload){Stack source=new Stack();source.push(payload);((Stack)sink).push(source.pop());String result=String.valueOf(((Stack)sink).peek());try{return result+":"+sink.pop();}catch(Throwable e){return result+":"+e.getClass().getName();}}
 public static void main(String[] args){System.out.println(run(new Stack<String>(),"value"));System.out.println(run(new Stack<String>(),Integer.valueOf(19)));System.out.println(run(new Stack<String>(),null));}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialGenericFactoryArrayLengthTupleRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GenericFactoryArrayLengthTuple", `import java.util.function.*;
interface InputCell<E>{E read();}
class ArrayFlow<E>{final E[] values;ArrayFlow(E[] v){values=v;}static <X>ArrayFlow<X> of(X... values){return new ArrayFlow<X>(values);} <R>ArrayFlow<R> flat(Function<? super E,? extends R> map,boolean enabled,int count){Object[] out=new Object[count];for(int i=0;i<count;i++)out[i]=map.apply(values[i]);return new ArrayFlow<R>((R[])out);} Object first(){return values[0];}}
class ArrayFunctions{static <X>Function<InputCell<X>,X> mapper(){return InputCell::read;}}
class ArrayScenario { static String run(int mode){try{InputCell[] cells=mode==0?new InputCell[]{()->"ok"}:mode==1?new InputCell[]{()->19}:mode==2?new InputCell[]{null}:null;return String.valueOf(GenericFactoryArrayLengthTuple.transform(cells).first());}catch(Throwable e){return e.getClass().getName();}}
}
public class GenericFactoryArrayLengthTuple {
 static <T>ArrayFlow<T> transform(InputCell<? extends T>[] input){return ((ArrayFlow)ArrayFlow.of(input)).flat(ArrayFunctions.mapper(),true,input.length);}

 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(ArrayScenario.run(i));}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialProtectedLoopFutureContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedLoopFutureContinuation", `import java.util.concurrent.*;
class AwaitGate {static String trace="";int count;final int mode;AwaitGate(int m){mode=m;}boolean pending(){trace+="p,";return count==0;}void await()throws InterruptedException{trace+="w,";count++;if(mode==1)throw new InterruptedException("wait");}}
class ContinuationFuture implements Future<String>{final int mode;ContinuationFuture(int m){mode=m;}public String get()throws ExecutionException,InterruptedException{AwaitGate.trace+="g,";if(mode==2)throw new ExecutionException(new IllegalArgumentException("future"));if(mode==3)throw new InterruptedException("get");return "value";}public String get(long n,TimeUnit u)throws ExecutionException,InterruptedException{return get();}public boolean cancel(boolean b){return false;}public boolean isDone(){return true;}public boolean isCancelled(){return false;}}
public class ProtectedLoopFutureContinuation {
 static String load(AwaitGate gate,Future<String> future,boolean wait){try{while(wait&&gate.pending()){gate.await();}return future.get();}catch(ExecutionException e){throw new IllegalStateException("exec",e.getCause());}catch(Exception e){throw new IllegalStateException("other",e);}}
 static String run(int mode,boolean wait){AwaitGate.trace="";try{return load(new AwaitGate(mode),mode==4?null:new ContinuationFuture(mode),wait)+":"+AwaitGate.trace;}catch(Throwable e){return e.getMessage()+":"+e.getCause().getClass().getName()+":"+AwaitGate.trace;}}
 public static void main(String[]args){for(int m=0;m<5;m++){System.out.println(run(m,false));System.out.println(run(m,true));}}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedThrowableCatchFinallyRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedThrowableCatchFinally", `import java.io.*;
class TransformGate {static String trace="";final int mode;int count;TransformGate(int m){mode=m;}int start(){trace+="s,";return mode==1?0:1;}void body()throws Throwable{trace+="b,";if(mode==2||mode==3||mode==4)throw new IllegalArgumentException("body");}int after(){trace+="a,";return count++==0?0:1;}void error(Throwable e)throws Throwable{trace+="e,";if(mode==3)throw new IOException("handler");if(mode==4)throw new AssertionError("handler");}void close()throws IOException{trace+="c,";if(mode==5)throw new IOException("close");}}
public class NestedThrowableCatchFinally {
 static void transform(TransformGate gate)throws IOException{try{if(gate.start()!=0){do{gate.body();}while(gate.after()==0);}}catch(Throwable error){try{gate.error(error);}catch(IOException|Error e){throw e;}catch(Throwable e){throw new IllegalStateException(e);}}finally{gate.close();}}
 static String run(int mode){TransformGate.trace="";try{transform(new TransformGate(mode));return "ok:"+TransformGate.trace;}catch(Throwable e){return e.getClass().getName()+":"+e.getMessage()+":"+TransformGate.trace;}}
 public static void main(String[]args){for(int i=0;i<7;i++)System.out.println(run(i));}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialConditionalFieldFinallyRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalFieldFinally", `import java.io.*;
class ConditionalResource {static String trace="";final int mode;int count;ConditionalResource(int m){mode=m;}int start(){trace+="s,";return mode==1?0:1;}void body()throws Throwable{trace+="b,";if(mode==2||mode==3||mode==4||mode==6)throw new IllegalArgumentException("body");}int after(){trace+="a,";return count++==0?0:1;}void error(Throwable e)throws Throwable{trace+="e,";if(mode==3)throw new IOException("handler");if(mode==4)throw new AssertionError("handler");}void close()throws IOException{trace+="c,";if(mode==5)throw new IOException("close");}}
public class ConditionalFieldFinally {
 ConditionalResource current;ConditionalFieldFinally(ConditionalResource r){current=r;}
 void transform(ConditionalResource next)throws IOException{ConditionalResource saved=current;current=next;try{if(next.start()!=0){do{next.body();}while(next.after()==0);}}catch(Throwable error){try{if(next!=null && !(error instanceof IllegalArgumentException && next.mode>=6)){next.error(error);}else{throw error;}}catch(IOException|Error e){throw e;}catch(Throwable e){throw new IllegalStateException(e);}}finally{current=saved;if(saved!=next)next.close();}}
 static String run(int mode,boolean shared){ConditionalResource next=new ConditionalResource(mode);ConditionalResource saved=shared?next:new ConditionalResource(0);ConditionalFieldFinally owner=new ConditionalFieldFinally(saved);ConditionalResource.trace="";String result;try{owner.transform(next);result="ok";}catch(Throwable e){result=e.getClass().getName()+":"+e.getMessage();}return result+":"+(owner.current==saved)+":"+ConditionalResource.trace;}
 public static void main(String[]args){for(int i=0;i<7;i++){System.out.println(run(i,false));System.out.println(run(i,true));}}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialReleasedMonitorLoopJoinRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ReleasedMonitorLoopJoin", `class MonitorTrace {static String trace="";static void mark(Object lock,String label){trace+=label+":"+Thread.holdsLock(lock)+",";}static void retry(Object lock){mark(lock,"retry");}}
public class ReleasedMonitorLoopJoin {
 static int select(Object lock,int mode){int selected;synchronized(lock){for(;;){MonitorTrace.mark(lock,"inside");if(mode==0)return 17;if(mode==1){selected=1;break;}if(mode==2){selected=2;break;}if(mode==3){MonitorTrace.retry(lock);mode=2;continue;}throw new IllegalArgumentException("inside");}}MonitorTrace.mark(lock,"outside");return selected;}
 static String run(int mode){Object lock=mode==5?null:new Object();MonitorTrace.trace="";String result;try{result=""+select(lock,mode);}catch(Throwable e){result=e.getClass().getName();}return result+":"+MonitorTrace.trace+":"+(lock!=null&&Thread.holdsLock(lock));}
 public static void main(String[]args){for(int i=0;i<6;i++)System.out.println(run(i));}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialWaitFutureCatchIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "WaitFutureCatchIdentity", `import java.util.concurrent.*;
class WaitFuture implements Future<String> {final int mode;WaitFuture(int m){mode=m;}public String get()throws ExecutionException,InterruptedException{if(mode==1)throw new ExecutionException(new IllegalArgumentException("future"));if(mode==2)throw new InterruptedException("get");return "value";}public String get(long n,TimeUnit u)throws ExecutionException,InterruptedException{return get();}public boolean cancel(boolean b){return false;}public boolean isDone(){return true;}public boolean isCancelled(){return false;}}
public class WaitFutureCatchIdentity {
 static String load(Object lock,Future<String>future,boolean wait){try{while(wait){synchronized(lock){lock.wait();}}return future.get();}catch(ExecutionException e){return "exec:"+e.getCause().getClass().getName();}catch(Exception e){return "other:"+e.getClass().getName();}}
 public static void main(String[]args){for(int i=0;i<4;i++)System.out.println(load(new Object(),i==3?null:new WaitFuture(i),false));Thread.currentThread().interrupt();try{System.out.println(load(new Object(),new WaitFuture(0),true));}finally{Thread.interrupted();}}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialErasedNestedSAMArgumentsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "ErasedNestedSAMArguments", `import java.util.*;import java.util.function.*;
class NestedInput {final int mode;NestedInput(int m){mode=m;}List<String> values(){if(mode==1)return (List)Arrays.asList(17,null);if(mode==2)return null;return Arrays.asList("a","b");}}
class NestedAPI {static <T>int count(Function<? super NestedInput,? extends Iterable<? extends T>> f,NestedInput input){int n=0;for(Object value:f.apply(input)){n+=value==null?1:value.toString().length();}return n;}}
public class ErasedNestedSAMArguments {
 static int count(NestedInput input){Function<NestedInput,Iterable<String>> mapper=x->x.values();return NestedAPI.count(mapper,input);}
 static String checkRaw(Object input){Function<NestedInput,Iterable<String>> mapper=x->x.values();try{return String.valueOf(((Function)mapper).apply(input));}catch(Throwable e){return e.getClass().getName();}}
 public static void main(String[]args){for(int i=0;i<4;i++){try{System.out.println(count(i==3?null:new NestedInput(i)));}catch(Throwable e){System.out.println(e.getClass().getName());}}System.out.println(checkRaw(19));System.out.println(checkRaw(null));}
}
`, func(name string) bool { return name != "NestedAPI" }, Precision, Compatibility, "legacy")
}

func TestAdversarialPutRetryInterruptFinallyRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "PutRetryInterruptFinally", `import java.util.concurrent.*;
class RetryingPutQueue extends LinkedBlockingQueue<String>{int calls;final int mode;RetryingPutQueue(int m){mode=m;}public void put(String v)throws InterruptedException{calls++;if(mode==1&&calls==1)throw new InterruptedException("retry");if(mode==2){if(calls==1)throw new InterruptedException("retry");throw new IllegalArgumentException("terminal");}if(v==null)throw new NullPointerException("null");}}
public class PutRetryInterruptFinally {
 static void put(BlockingQueue<String>queue,String value){boolean interrupted=false;try{while(true){try{queue.put(value);break;}catch(InterruptedException e){interrupted=true;}}}finally{if(interrupted)Thread.currentThread().interrupt();}}
 static String run(int mode,boolean initial){Thread.interrupted();if(initial)Thread.currentThread().interrupt();RetryingPutQueue queue=new RetryingPutQueue(mode);String result;try{put(queue,mode==3?null:"value");result="ok";}catch(Throwable e){result=e.getClass().getName();}boolean interrupted=Thread.interrupted();return result+":"+queue.calls+":"+interrupted;}
 public static void main(String[]args){for(int m=0;m<4;m++){System.out.println(run(m,false));System.out.println(run(m,true));}}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedTraversalReturnExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedTraversalReturnExit", `import java.util.*;
interface TraversalPart {int size();}
class TraversalLeaf implements TraversalPart {final int n;TraversalLeaf(int v){n=v;}public int size(){return n;}}
class TraversalBranch implements TraversalPart {final TraversalPart left,right;TraversalBranch(TraversalPart a,TraversalPart b){left=a;right=b;}public int size(){return left.size()+right.size();}}
class TraversalCases {static void run(){TraversalPart a=new TraversalLeaf(3),b=new TraversalLeaf(7),c=new TraversalLeaf(11);TraversalBranch[] trees={new TraversalBranch(a,b),new TraversalBranch(new TraversalBranch(a,b),c),new TraversalBranch(a,new TraversalBranch(b,c)),new TraversalBranch(new TraversalBranch(a,b),new TraversalBranch(b,c))};for(TraversalBranch tree:trees)System.out.println(tree.size()+":"+NestedTraversalReturnExit.sum(tree));}}
public class NestedTraversalReturnExit {
 static int sum(TraversalBranch tree){int n=0;TraversalBranch[] stack=new TraversalBranch[2];int depth=0;while(true){TraversalPart left=tree.left;if(left instanceof TraversalBranch){if(depth==stack.length)stack=Arrays.copyOf(stack,depth*2);stack[depth++]=tree;tree=(TraversalBranch)left;continue;}n+=left.size();while(true){TraversalPart right=tree.right;if(right instanceof TraversalBranch){tree=(TraversalBranch)right;break;}n+=right.size();if(depth==0)return n;tree=stack[--depth];}}}
 public static void main(String[]args){TraversalCases.run();}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedFallbackProtectedArrayRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedFallbackProtectedArray", `import java.lang.reflect.*;
interface FallbackChoice {int value();}
class PrimaryChoice implements FallbackChoice {public int value(){return 10;}}
class LegacyChoice implements FallbackChoice {public int value(){return -1;}}
class ReflectedChoice implements FallbackChoice {final Method method;ReflectedChoice(Method m){method=m;}public int value(){return method.getParameterTypes().length;}}
public class NestedFallbackProtectedArray {
 static FallbackChoice choose(String name,String method,boolean legacy,boolean second){try{try{Class.forName(name);return new PrimaryChoice();}catch(Exception first){return legacy&&second?new LegacyChoice():new ReflectedChoice(String.class.getDeclaredMethod(method,new Class[]{int.class}));}}catch(Exception terminal){return new LegacyChoice();}}
 public static void main(String[]args){for(boolean a:new boolean[]{false,true})for(boolean b:new boolean[]{false,true}){System.out.println(choose("java.lang.String","substring",a,b).value());System.out.println(choose("missing.Type","substring",a,b).value());System.out.println(choose("missing.Type","missing",a,b).value());}}
}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialZeroArgumentFactoryAssignmentRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ZeroArgumentFactoryAssignment", `import java.util.*;
class FactoryBase<T>{static String trace="";T value;FactoryBase(T v){value=v;}FactoryBase<T> finish(){trace+="f,";return this;}T read(){trace+="r,";return value;}}
class FactoryLeaf<T> extends FactoryBase<T>{FactoryLeaf(T v){super(v);}static int mode;static <X>FactoryLeaf<X> create(){trace+="c,";return mode==2?null:new FactoryLeaf<X>((X)(mode==1?Integer.valueOf(17):"ok"));}}
public class ZeroArgumentFactoryAssignment {
 final FactoryBase<List<String>> stored=(FactoryBase)FactoryLeaf.create().finish();
 static FactoryBase<String> make(){return (FactoryBase)FactoryLeaf.create().finish();}
 static String run(int mode){FactoryLeaf.mode=mode;FactoryBase.trace="";try{ZeroArgumentFactoryAssignment owner=new ZeroArgumentFactoryAssignment();FactoryBase<String> v=make();String result=v.read();return result+":"+(owner.stored!=null)+":"+FactoryBase.trace;}catch(Throwable e){return e.getClass().getName()+":"+FactoryBase.trace;}}
 public static void main(String[]args){for(int m=0;m<3;m++)System.out.println(run(m));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSwitchExternalContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SwitchExternalContinuation", `class SharedSwitchTrace {static String trace="";static int mark(int n){trace+=n+",";return n;}}
public class SwitchExternalContinuation {
 final int choice,tail;
 SwitchExternalContinuation(int mode,int n){if(mode==0){choice=SharedSwitchTrace.mark(99);}else{switch(n){case -7:choice=SharedSwitchTrace.mark(11);break;case 2:case 3:choice=SharedSwitchTrace.mark(22);break;case 8:choice=SharedSwitchTrace.mark(33);break;default:throw new IllegalArgumentException("unknown");}}tail=SharedSwitchTrace.mark(44);}
 static int nested(int mode,int n){int result;if(mode==0)result=77;else{switch(n){case 2:result=1;break;case 3:result=2;case 8:result=SharedSwitchTrace.mark(9);break;default:result=4;}}return result+SharedSwitchTrace.mark(5);}
 public static void main(String[]args){for(int mode=0;mode<2;mode++)for(int n:new int[]{Integer.MIN_VALUE,-7,2,3,8,19,Integer.MAX_VALUE}){SharedSwitchTrace.trace="";try{SwitchExternalContinuation c=new SwitchExternalContinuation(mode,n);System.out.println(c.choice+":"+c.tail+":"+SharedSwitchTrace.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+SharedSwitchTrace.trace);}SharedSwitchTrace.trace="";System.out.println(nested(mode,n)+":"+SharedSwitchTrace.trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedDrainCompletionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedDrainCompletion", `class DrainState {static String trace="";int items,turn;final int mode;DrainState(int m){mode=m;items=m==0?0:2;}boolean cancelled(){trace+="c,";return mode==1;}boolean empty(){trace+="e,";return items==0;}int poll(){trace+="p,";if(mode==2)throw new IllegalArgumentException("poll");return items--;}boolean retry(){trace+="r,";return turn++==0;}void next(int n){trace+="n"+n+",";}void failed(Throwable e){trace+="x,";}}
public class NestedDrainCompletion {
 static void drain(DrainState state,String[]listeners){outer:while(true){int consumed=0;while(true){if(state.cancelled())return;if(state.empty())break outer;try{int value=state.poll();state.next(value);}catch(Throwable e){state.failed(e);return;}consumed++;if(consumed==1)break;}if(!state.retry())return;}for(String listener:listeners)DrainState.trace+=listener+",";}
 public static void main(String[]args){for(int m=0;m<4;m++){DrainState.trace="";drain(new DrainState(m),new String[]{"complete-a","complete-b"});System.out.println(m+":"+DrainState.trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLateInstanceConstantStoreRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LateInstanceConstantStore", `class ConstantBase {ConstantBase(){LateInstanceConstantStore.trace+="b,";}}
public class LateInstanceConstantStore extends ConstantBase {
 static String trace=""; final Object before=event(); final boolean enabled=true; final String text="ready";
 Object event(){trace+="e:"+read()+",";return new Object();} boolean read(){try{return LateInstanceConstantStore.class.getDeclaredField("enabled").getBoolean(this);}catch(Exception e){throw new IllegalStateException(e);}}
 LateInstanceConstantStore(){trace+="c:"+enabled+",";}
 public static void main(String[]a){for(int i=0;i<3;i++){trace="";LateInstanceConstantStore x=new LateInstanceConstantStore();System.out.println(trace+":"+x.enabled+":"+x.text);}}
}`, Precision, Compatibility, "legacy")
}
func TestAdversarialSynchronizedAbruptCompletionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SynchronizedAbruptCompletion", `public class SynchronizedAbruptCompletion {
 static Object lock=new Object();static String trace;
 static int run(int mode){int turn=0;while(true){synchronized(lock){trace+="m,";if(turn++==0)continue;if(mode==0)return turn;if(mode==1)throw new IllegalArgumentException("bad");return -turn;}}}
 public static void main(String[]a){for(int mode=0;mode<3;mode++){trace="";try{System.out.println(run(mode)+":"+trace);}catch(Exception e){System.out.println(e.getClass().getName()+":"+trace);}System.out.println(Thread.holdsLock(lock));}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialProtectedStaticInitializerOrderRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedStaticInitializerOrder", `public class ProtectedStaticInitializerOrder {
 static String trace=""; static final Object first; static final Object second;
 static Object event(String name){trace+=name+",";return new Object();}
 static {try{first=event("first");second=event("second");}catch(Exception e){throw new IllegalStateException(e);}}
 public static void main(String[]a){System.out.println(trace+":"+(first!=second));}
}`, Precision, Compatibility, "legacy")
}
func TestAdversarialParameterizedFactoryAssignmentCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ParameterizedFactoryAssignmentCast", `class FactoryBox<T>{T value;static <T> FactoryBox<T> create(){return new FactoryBox<T>();}void put(T v){value=v;}T get(){return value;}}
public class ParameterizedFactoryAssignmentCast<B> {
 final FactoryBox<java.util.Map<Class<? extends B>,B>> values=FactoryBox.create();
 ParameterizedFactoryAssignmentCast(){values.put(new java.util.HashMap<Class<? extends B>,B>());}
 public static void main(String[]a){ParameterizedFactoryAssignmentCast<Number> box=new ParameterizedFactoryAssignmentCast<>();box.values.get().put(Integer.class,17);System.out.println(box.values.get().get(Integer.class));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNoArgumentConstructorDelegationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NoArgumentConstructorDelegation", `public class NoArgumentConstructorDelegation {
 static String trace="";final java.util.List<String> entries;NoArgumentConstructorDelegation(){trace+="a,";entries=new java.util.ArrayList<String>();}
 NoArgumentConstructorDelegation(String input){this();trace+="b,";entries.add(input);}
 public static void main(String[]args){NoArgumentConstructorDelegation value=new NoArgumentConstructorDelegation("x");System.out.println(trace+value.entries);}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialRepeatedCheckedReceiverInConditionalRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "RepeatedCheckedReceiverInConditional", `public class RepeatedCheckedReceiverInConditional {
 static String trace;static Object key(java.util.Map.Entry<?,?> e){trace+="k,";return e.getKey();}
 static int run(java.util.Map<?,Integer> map,String prefix){int sum=0;for(java.util.Map.Entry<?,Integer> e:map.entrySet()){int dot=((String)key(e)).lastIndexOf('.');if(!prefix.equals(dot==-1?"":((String)key(e)).substring(0,dot)))throw new IllegalArgumentException("package");try{sum+=e.getValue();}catch(Exception ex){throw new IllegalStateException(ex);}}return sum;}
 public static void main(String[]a){for(String name:new String[]{"Bare","good.Item","bad.Item"}){java.util.Map<Object,Integer> map=new java.util.LinkedHashMap<>();map.put(name,7);trace="";try{System.out.println(run(map,"good")+":"+trace);}catch(Exception e){System.out.println(e.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLoopSharedObjectReturnRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LoopSharedObjectReturn", `public class LoopSharedObjectReturn {
 static String trace;
 static Object run(boolean report,java.util.Map<String,Integer> data){Object result=new Object();for(int i=0;i<2;i++)trace+="a,";if(report){for(java.util.Map.Entry<String,Integer> item:data.entrySet())trace+=item.getKey()+"="+item.getValue()+",";}return result;}
 public static void main(String[]args){for(boolean report:new boolean[]{false,true}){trace="";java.util.Map<String,Integer> data=new java.util.LinkedHashMap<>();data.put("x",3);System.out.println((run(report,data)!=null)+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLoopCandidateSelectionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LoopCandidateSelection", `public class LoopCandidateSelection {
 static String trace;static int[] sizes={1,2,3,4,5,6,7,8};
 static int choose(int need,int excess){int size=0;Integer chosen=null;for(int attempt=0;;attempt++){if(attempt>7)throw new IllegalArgumentException("too large");int capacity=(attempt+1)*3;trace+="c"+attempt+",";if(need>capacity)continue;if(chosen==null||size!=sizes[attempt]){size=sizes[attempt];chosen=need+size;trace+="s"+attempt+",";}if(chosen+excess>capacity)continue;break;}if(chosen==null||chosen+excess>24)throw new IllegalArgumentException("too large");return chosen;}
 public static void main(String[]args){for(int need:new int[]{0,1,5,9,22,25})for(int extra:new int[]{0,2,10}){trace="";try{System.out.println(choose(need,extra)+":"+trace);}catch(Exception e){System.out.println(e.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialAutomaticLayerSelectionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "AutomaticLayerSelection", `public class AutomaticLayerSelection {
 static final int[] WIDTH={1,2,3,4,5,6,7,8,9};static String trace;
 static int capacity(int layer,boolean compact){trace+="c"+layer+":"+compact+",";return layer*8+4;}
 static int[] pad(int count,int width){trace+="p"+width+",";return new int[count+width];}
 static String run(int input,int extra,int requested){boolean compact;int layer,capacity,width;int[] bits;int usable;
 if(requested!=0){compact=requested<0;layer=Math.abs(requested);if(layer>8)throw new IllegalArgumentException("layer");capacity=capacity(layer,compact);width=WIDTH[layer];usable=capacity-capacity%width;bits=pad(input,width);if(bits.length+extra>usable)throw new IllegalArgumentException("size");}
 else{width=0;bits=null;for(int i=0;;i++){if(i>7)throw new IllegalArgumentException("exhausted");capacity=capacity(layer=(compact=i<=2)?i+1:i,compact);if(input+extra>capacity)continue;if(bits==null||width!=WIDTH[layer])bits=pad(input,width=WIDTH[layer]);usable=capacity-capacity%width;if(compact&&bits.length>(width<<2)||bits.length+extra>usable)continue;break;}}
 return compact+":"+layer+":"+capacity+":"+width+":"+bits.length;}
 public static void main(String[]args){for(int input:new int[]{0,3,10,30,70})for(int extra:new int[]{0,7})for(int req:new int[]{0,2,-2}){trace="";try{System.out.println(run(input,extra,req)+":"+trace);}catch(Exception e){System.out.println(e.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialProtectedLookupLoopContinuationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedLookupLoopContinuation", `public class ProtectedLookupLoopContinuation {
 static String trace;static String read(String s)throws javax.naming.NamingException{trace+="r"+s+",";if(s.equals("bad"))throw new javax.naming.NamingException();return s.isEmpty()?null:s;}
 static String find(java.util.List<String> values){for(int i=values.size()-1;i>=0;i--){String attribute=values.get(i);if(attribute!=null){try{String value=read(attribute);if(value!=null)return value;}catch(java.util.NoSuchElementException e){}catch(javax.naming.NamingException e){}}}return null;}
 public static void main(String[]args){String[][] cases={{},{null},{"first",null},{"first","bad",""},{"first","last"},{"bad","",null}};for(String[] c:cases){trace="";System.out.println(find(java.util.Arrays.asList(c))+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialConditionalFieldStoreValueRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalFieldStoreValue", `interface FieldStoreHandler {} class FieldStoreVisitor implements FieldStoreHandler {final int code;FieldStoreVisitor(int code){this.code=code;}}
public class ConditionalFieldStoreValue {
 static String trace;FieldStoreHandler handler;
 static FieldStoreHandler create(boolean enabled,FieldStoreVisitor value,boolean first,boolean second){trace+="make:"+enabled+":"+first+":"+second+",";return new FieldStoreVisitor(value.code+1);}
 FieldStoreVisitor visit(boolean missing,int flags){FieldStoreVisitor visitor=missing?null:new FieldStoreVisitor(flags);return visitor==null?null:(FieldStoreVisitor)(handler=create(true,visitor,(flags&2)==0&&flags>=3,(flags&8)!=0));}
 public static void main(String[]args){for(boolean missing:new boolean[]{false,true})for(int flags:new int[]{0,2,3,8,11}){trace="";ConditionalFieldStoreValue owner=new ConditionalFieldStoreValue();FieldStoreVisitor result=owner.visit(missing,flags);System.out.println((result==null?"null":String.valueOf(result.code))+":"+(owner.handler==result)+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialWrappingLoopThenProtectedCallRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "WrappingLoopThenProtectedCall", `class WrappingFailure extends Exception {}
public class WrappingLoopThenProtectedCall {
 static String trace;static Object wrap(Object value)throws WrappingFailure{trace+="w"+value+",";if("bad".equals(value))throw new WrappingFailure();return value;}
 static Object construct(java.util.List<Object> values)throws Exception{trace+="c,";if(values.contains("fail"))throw new Exception();return values.toString();}
 static Object build(java.util.List<Object> input){if(input.isEmpty()){try{return construct(input);}catch(Exception e){throw new IllegalStateException("empty",e);}}else{java.util.List<Object> wrapped=new java.util.ArrayList<Object>(input.size());for(int i=0;i<input.size();i++){try{wrapped.add(wrap(input.get(i)));}catch(WrappingFailure e){throw new IllegalArgumentException("wrap:"+i,e);}}try{return construct(wrapped);}catch(Exception e){throw new IllegalStateException("construct",e);}}}
 public static void main(String[]args){Object[][] cases={{},{"a"},{"a","b"},{"bad"},{"a","bad"},{"fail"}};for(Object[] c:cases){trace="";try{System.out.println(build(java.util.Arrays.asList(c))+":"+trace);}catch(Exception e){System.out.println(e.getClass().getName()+":"+e.getMessage()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLoopCatchSharedRethrowRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LoopCatchSharedRethrow", `class CreationFailure extends RuntimeException { final int code;CreationFailure(int code){this.code=code;} }
public class LoopCatchSharedRethrow {
 static String trace; static int read(int value){trace+="r"+value+",";if(value<0)throw new CreationFailure(value);return value;}
 static int collect(int[] values){int sum=0;for(int value:values){try{sum+=read(value);}catch(CreationFailure e){if(e.code==-1){String name=values.length>1?"active":null;if(name!=null&&name.equals("active")){trace+="s,";continue;}}throw e;}}return sum;}
 public static void main(String[] args){int[][] cases={{},{1,2},{-1},{-1,2},{-2,1},{2,-1,4},{1,-2,-1}};for(int[] c:cases){trace="";try{System.out.println(collect(c)+":"+trace);}catch(CreationFailure e){System.out.println(e.code+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialNestedConditionalLookupCastRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedConditionalLookupCast", `class ConditionalLookupBound{final boolean primary;ConditionalLookupBound(boolean p){primary=p;}boolean primary(){NestedConditionalLookupCast.trace+="p,";return primary;}}
public class NestedConditionalLookupCast {
 static String trace;
 final java.util.Map<Integer,java.util.Map<String,String>> tokens;final java.util.List<ConditionalLookupBound> bounds;
 NestedConditionalLookupCast(java.util.Map<Integer,java.util.Map<String,String>> t,java.util.List<ConditionalLookupBound> b){tokens=t;bounds=b;}
 java.util.Map<String,String> select(int index){java.util.Map<String,String> result=!tokens.containsKey(index)&&!tokens.containsKey(index+1)?java.util.Collections.<String,String>emptyMap():tokens.get(index+(bounds.get(0).primary()?0:1));return result==null?java.util.Collections.<String,String>emptyMap():result;}
 public static void main(String[] args){NestedConditionalLookupCastOracle.main(args);}
}
class NestedConditionalLookupCastOracle {
 public static void main(String[] args){for(boolean p:new boolean[]{false,true})for(int mask=0;mask<8;mask++)for(int index=0;index<3;index++){java.util.Map<Integer,java.util.Map<String,String>> t=new java.util.HashMap<Integer,java.util.Map<String,String>>();for(int k=0;k<3;k++)if((mask&(1<<k))!=0){java.util.Map<String,String> v=new java.util.TreeMap<String,String>();v.put("v",String.valueOf(k));t.put(k,k==2?null:v);}NestedConditionalLookupCast.trace="";NestedConditionalLookupCast x=new NestedConditionalLookupCast(t,mask==0?java.util.Collections.<ConditionalLookupBound>emptyList():java.util.Arrays.asList(new ConditionalLookupBound(p)));System.out.println(x.select(index)+":"+NestedConditionalLookupCast.trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialConditionalGenericVarargsFactoryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalGenericVarargsFactory", `class FactoryItem{final int code;FactoryItem(int code){this.code=code;ConditionalGenericVarargsFactory.trace+="n"+code+",";}}
public class ConditionalGenericVarargsFactory {
 static String trace;final java.util.List<FactoryItem> items;
 ConditionalGenericVarargsFactory(java.util.List<FactoryItem> values){items=values;}
 static ConditionalGenericVarargsFactory make(boolean empty,int code){return new ConditionalGenericVarargsFactory(empty?java.util.Collections.<FactoryItem>emptyList():java.util.Arrays.asList(new FactoryItem(code)));}
 public static void main(String[] args){for(boolean empty:new boolean[]{false,true})for(int code:new int[]{0,-1,2}){trace="";ConditionalGenericVarargsFactory value=make(empty,code);System.out.println(value.items.size()+":"+(value.items.isEmpty()?"empty":String.valueOf(value.items.get(0).code))+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

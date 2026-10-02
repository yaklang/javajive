package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are original ClassFile contracts, parsed without executing old JARs.
// The separate authored JVM oracles test the semantics of the recovered views.
func reviewedSeedSources(t *testing.T, path, flag string, resolve bool, check func(string)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(name string) ([]byte, bool) {
		b, e := os.ReadFile(filepath.Join(filepath.Dir(path), name[strings.LastIndexByte(name, '/')+1:]+".class"))
		return b, e == nil
	}
	var on string
	for _, setting := range []string{"", "1"} {
		t.Setenv(flag, setting)
		var source string
		if resolve {
			source, err = DecompileWithResolver(raw, resolver)
		} else {
			source, err = Decompile(raw)
		}
		if err != nil {
			t.Fatal(err)
		}
		check(source)
		if setting == "" {
			on = source
		} else if source != on {
			t.Fatal("superseded source-spelling control must preserve the same original invocation binding")
		}
	}
}
func assertReviewedSeedSAM(t *testing.T, raw []byte, erased, instantiated string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, attribute := range object.Attributes {
		bootstrap, ok := attribute.(*BootstrapMethodsAttribute)
		if !ok {
			continue
		}
		for _, method := range bootstrap.BootstrapMethods {
			if len(method.BootstrapArguments) != 3 {
				continue
			}
			sam, samOK := cp.IndexInfo(int(method.BootstrapArguments[0])).(*ConstantMethodTypeInfo)
			inst, instOK := cp.IndexInfo(int(method.BootstrapArguments[2])).(*ConstantMethodTypeInfo)
			if !samOK || !instOK || cp.GetUtf8(int(sam.DescriptorIndex)).Value != erased || cp.GetUtf8(int(inst.DescriptorIndex)).Value != instantiated {
				continue
			}
			handle, ok := cp.IndexInfo(int(method.BootstrapMethodRef)).(*ConstantMethodHandleInfo)
			if !ok || handle.ReferenceKind != 6 {
				t.Fatal("original SAM bootstrap changed")
			}
			member, ok := cp.IndexInfo(int(handle.ReferenceIndex)).(*ConstantMethodrefInfo)
			if !ok || cp.GetClassName(int(member.ClassIndex)) != "java/lang/invoke/LambdaMetafactory" {
				t.Fatal("original SAM was not LambdaMetafactory")
			}
			return
		}
	}
	t.Fatalf("original SAM tuple %s => %s absent", erased, instantiated)
}

func TestAdversarialReviewedConstructorFactoryBindingsRoundTrip(t *testing.T) {
	for _, flag := range []string{"JDEC_CAFFEINE_REMAINING_OFF", "JDEC_COLLECTIONS4_REMAINING_OFF", "JDEC_CTOR_DIAMOND_OFF", "JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF", "JDEC_NEW_RECV_DIAMOND_OFF"} {
		t.Setenv(flag, "1")
	}
	t.Setenv("JDEC_CLASSLIT_ARG_NOCAST_OFF", "")
	t.Setenv("JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF", "1")
	roundTripGenericFlow(t, "SeedBindingsReview", `import java.util.*;import java.util.function.*;import java.util.concurrent.*;
class BindingSupport {
 static String trace="";static final RuntimeException runtime=new IllegalStateException("original");static final Exception checked=new Exception("original");static final InterruptedException interrupted=new InterruptedException("original");
 static final Object marker=new Object();static Map shared=new LinkedHashMap();
 static Integer parse(String text){trace+="P";if(text==null)throw runtime;return Integer.valueOf(text);}
}
class BindingBox<X>{final String selected;final Class<X> cls;final Function<String,X> fn;
 BindingBox(Class<X> cls,Function<String,X> fn){selected="function";this.cls=cls;this.fn=fn;BindingSupport.trace+="C";}
 BindingBox(Class<?> cls,Object fn){selected="object";this.cls=null;this.fn=null;BindingSupport.trace+="O";}
}
class BindingFuture<X>{final X value;BindingFuture(X value){this.value=value;}}
class BindingFutures{static <X>BindingFuture<X> immediate(X value){BindingSupport.trace+="F";return new BindingFuture<>(value);}}
class BindingCollection<K,V>{final Map original;final Class chosen;BindingCollection(Map original,Class chosen){this.original=original;this.chosen=chosen;}}
class BindingFactories{static <K,V,C extends Collection<V>>BindingCollection<K,V> create(Map<K,? super C> map,Class<C> chosen){BindingSupport.trace+="B";return new BindingCollection<>(map,chosen);}}
class BindingLoader<K,V>{int mode;final Map<K,V> shared;BindingLoader(Map<K,V> shared){this.shared=shared;}
 Map<? extends K,? extends V> loadAll(Iterable<? extends K> keys)throws Exception{BindingSupport.trace+="L";if(mode==1)throw BindingSupport.runtime;if(mode==2)throw BindingSupport.checked;if(mode==3)throw BindingSupport.interrupted;return mode==4?null:shared;}
}
class BindingOrder<X> implements Comparator<X>{final boolean backwards;BindingOrder(boolean backwards){this.backwards=backwards;}
 static <C extends Comparable>BindingOrder<C> natural(){BindingSupport.trace+="N";return new BindingOrder<>(false);}
 <S extends X>BindingOrder<S> reverse(){BindingSupport.trace+="R";return new BindingOrder<>(true);}
 public int compare(X left,X right){BindingSupport.trace+="Q";int value=((Comparable)left).compareTo(right);return backwards?-value:value;}
}
public class SeedBindingsReview<K,V>{
 BindingFuture<V> saved;boolean accept;
 BindingBox<Integer> box(){return new BindingBox<>(Integer.class,BindingSupport::parse);}
 BindingFuture<V> fallback(Object value){return accept?saved:BindingFutures.immediate((V)value);}
 BindingCollection<K,V> collection(Map<K,? super Collection<V>> map){return BindingFactories.create((Map)map,(Class)ArrayList.class);}
 Function<Iterable<? extends K>,Map<K,V>> bulk(BindingLoader<? super K,V> loader){return keys->{try{return (Map<K,V>)(Map)loader.loadAll(keys);}catch(RuntimeException error){throw error;}catch(InterruptedException error){Thread.currentThread().interrupt();throw new CompletionException(error);}catch(Exception error){throw new CompletionException(error);}};}
 Comparator<? super K> descending(){return (Comparator<? super K>)(Comparator)BindingOrder.natural().reverse();}
 public static void main(String[]args){
 SeedBindingsReview<String,Object> owner=new SeedBindingsReview<>();
 BindingSupport.trace="";BindingBox<Integer> box=owner.box();System.out.println(box.selected+":"+(box.cls==Integer.class)+":"+box.fn.apply("17")+":"+BindingSupport.trace);
 BindingSupport.trace="";try{box.fn.apply(null);}catch(Throwable error){System.out.println((error==BindingSupport.runtime)+":"+BindingSupport.trace);}
 owner.saved=new BindingFuture<>(BindingSupport.marker);for(boolean accept:new boolean[]{false,true})for(Object value:new Object[]{null,"text",BindingSupport.marker}){owner.accept=accept;BindingSupport.trace="";BindingFuture<Object> result=owner.fallback(value);System.out.println((result==owner.saved)+":"+(result.value==(accept?BindingSupport.marker:value))+":"+BindingSupport.trace);}
 Map<String,Collection<Object>> map=new LinkedHashMap<>();BindingSupport.trace="";BindingCollection<String,Object> wrapped=owner.collection(map);System.out.println((wrapped.original==map)+":"+(wrapped.chosen==ArrayList.class)+":"+BindingSupport.trace);
 BindingSupport.shared.put(Integer.valueOf(7),BindingSupport.marker);BindingLoader<Object,Object> loader=new BindingLoader<>(BindingSupport.shared);Function<Iterable<? extends String>,Map<String,Object>> function=owner.bulk(loader);
 for(int mode=0;mode<5;mode++){loader.mode=mode;BindingSupport.trace="";Thread.interrupted();try{System.out.println((function.apply(Arrays.asList("key"))==(Object)BindingSupport.shared)+":"+BindingSupport.trace);}catch(Throwable error){System.out.println((error==BindingSupport.runtime)+":"+(error.getCause()==BindingSupport.checked)+":"+(error.getCause()==BindingSupport.interrupted)+":"+Thread.currentThread().isInterrupted()+":"+BindingSupport.trace);}Thread.interrupted();}
 BindingSupport.trace="";Comparator comparator=owner.descending();for(Object[]pair:new Object[][]{{"a","b"},{Integer.valueOf(1),Integer.valueOf(2)},{BindingSupport.marker,"x"},{null,"x"}}){try{System.out.println(comparator.compare(pair[0],pair[1]));}catch(Throwable error){System.out.println(error.getClass().getName());}}System.out.println(BindingSupport.trace);
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialReviewedCollectionCallbackBindingsRoundTrip(t *testing.T) {
	t.Setenv("JDEC_COMPRESS_REMAINING_OFF", "1")
	t.Setenv("JDEC_NEW_RECV_DIAMOND_OFF", "1")
	roundTripGenericFlow(t, "SeedCallbacksReview", `import java.util.*;import java.util.function.*;import java.nio.*;
class CallbackSupport{static String trace="";static final RuntimeException failure=new IllegalStateException("callback");static void seen(Object value){trace+="S";}static String convert(String value){trace+="U";if("fail".equals(value))throw failure;return value==null?null:value.toUpperCase();}}
class CallbackTuple{final int index;CallbackTuple(int index){this.index=index;}int index(){CallbackSupport.trace+="T"+index;if(index<0)throw CallbackSupport.failure;return index;}}
public class SeedCallbacksReview {
 Queue<CallbackTuple> queue(){return new PriorityQueue<>(10,(a,b)->Integer.compare(a.index(),b.index()));}
 void sort(List<CallbackTuple> list){list.sort((a,b)->Integer.compare(a.index(),b.index()));}
 long sum(List<Integer> list){return list.stream().peek(CallbackSupport::seen).mapToLong(Integer::longValue).sum();}
 void collect(Map<Integer,Integer> map,List<Integer> out){map.forEach((key,value)->{CallbackSupport.trace+="C";if(value.intValue()<0)throw CallbackSupport.failure;out.add(key);});}
 void copy(Map<String,String> map,UnaryOperator<String> resolver){new HashMap<>(map).forEach((key,value)->{String left=resolver.apply(key);String right=resolver.apply(value);if(left!=null&&right!=null&&!left.equals(right))map.put(left,right);});}
 int buffer(int mode){if(mode==0){List<Integer> list=Arrays.asList(3,4);return list.get(1);}ByteBuffer buffer=ByteBuffer.allocate(2);buffer.putShort((short)mode);buffer.flip();return buffer.getShort();}
 static void report(Throwable error){System.out.println(error.getClass().getName()+":"+(error==CallbackSupport.failure)+":"+CallbackSupport.trace);}
 public static void main(String[]args){SeedCallbacksReview owner=new SeedCallbacksReview();
 for(Object second:new Object[]{new CallbackTuple(1),new CallbackTuple(-1),null,"wrong"}){CallbackSupport.trace="";Queue queue=owner.queue();try{queue.add(new CallbackTuple(2));queue.add(second);System.out.println(((CallbackTuple)queue.remove()).index+":"+CallbackSupport.trace);}catch(Throwable error){report(error);}}
 for(Object first:new Object[]{new CallbackTuple(2),new CallbackTuple(-1),null,"wrong"}){CallbackSupport.trace="";List list=new ArrayList(Arrays.asList(first,new CallbackTuple(1)));try{owner.sort(list);System.out.println(((CallbackTuple)list.get(0)).index+":"+CallbackSupport.trace);}catch(Throwable error){report(error);}}
 for(List list:new List[]{Arrays.asList(1,2),Arrays.asList(1,null),Arrays.asList(1,Long.valueOf(2))}){CallbackSupport.trace="";try{System.out.println(owner.sum(list)+":"+CallbackSupport.trace);}catch(Throwable error){report(error);}}
 for(Object value:new Object[]{Integer.valueOf(1),Integer.valueOf(-1),null,"wrong"}){Map raw=new LinkedHashMap();raw.put(3,value);List<Integer> out=new ArrayList<>();CallbackSupport.trace="";try{owner.collect(raw,out);System.out.println(out+":"+CallbackSupport.trace);}catch(Throwable error){report(error);System.out.println(out);}}
 for(Object value:new Object[]{"b","fail",null,Integer.valueOf(2)}){Map raw=new LinkedHashMap();raw.put("a",value);CallbackSupport.trace="";try{owner.copy(raw,CallbackSupport::convert);System.out.println(raw+":"+CallbackSupport.trace);}catch(Throwable error){report(error);}}
 for(int mode:new int[]{0,1,-1,32767,32768})System.out.println(owner.buffer(mode));
 }
}`, Precision, Compatibility, "legacy")
}

// Retained separately from binding checks: a source catch of Throwable and
// its finally catch-all are not two sibling source catches.
func TestAdversarialCatchThrowableFinallyCleanupRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "CatchThrowableCleanupReview", `public class CatchThrowableCleanupReview {
 static final RuntimeException failure=new IllegalArgumentException("original");
 static void action(int mode){if(mode==1)throw failure;if(mode==2){Thread.currentThread().interrupt();throw failure;}}
 static void run(int mode){Thread.interrupted();try{action(mode);System.out.println("normal");}catch(Throwable error){System.out.println((error==failure)+":"+Thread.currentThread().isInterrupted());}finally{Thread.interrupted();}System.out.println(Thread.currentThread().isInterrupted());}
 public static void main(String[]args){run(0);run(1);run(2);}
 }`, Precision, Compatibility, "legacy")
}

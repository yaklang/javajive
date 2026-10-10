package javaclassparser

import (
	"os"
	"testing"
)

// SAM target types belong to the captured producer, before widening it to an
// unrelated API parameter. Nested captures keep their own lexical identities;
// packed generic varargs retain the original runtime array and allocation.
func TestAdversarialRemainingSAMScopeAndPackedArrayRoundTrip(t *testing.T) {
	for _, flag := range []string{"JDEC_HIKARICP_REMAINING_OFF", "JDEC_POOL2_REMAINING_OFF", "JDEC_VISITANNOTATION_CONSUMER_CAST_OFF", "JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", "JDEC_VARARGS_SPREAD_OFF"} {
		t.Setenv(flag, "1")
	}
	roundTripGenericFlow(t, "RemainingSAMReview", `import java.util.*;import java.util.function.*;import java.util.stream.*;import java.lang.ref.*;import java.io.*;
class SamEffects{static String trace="";static final RuntimeException failure=new IllegalArgumentException("original");static boolean touch(String v){trace+="T";if("throw".equals(v))throw failure;return v!=null;}}
interface SamMetric{}
interface SamGauge<X> extends SamMetric{X get();}
class SamStats{int n;boolean fail;Integer pull(){SamEffects.trace+="G";if(fail)throw SamEffects.failure;return ++n;}}
class SamRegistry{SamMetric saved;String register(String name,SamMetric metric){SamEffects.trace+="M";saved=metric;return name;}String register(String name,Object metric){SamEffects.trace+="WRONG";return name;}}
class SamPack{static Object[] last;static <X>Iterator<X>forArr(X...array){SamEffects.trace+="A";last=array;return Arrays.asList(array).iterator();}static Iterator forArr(Object value){SamEffects.trace+="WRONG";return Collections.emptyIterator();}}
class SamWriter extends PrintWriter{SamWriter(Writer writer){super(writer);}public void println(Object value){SamEffects.trace+="O";super.println(value);}public void println(String value){SamEffects.trace+="T";super.println(value);}}
class SamItem{final String text;SamItem(String text){this.text=text;}public String toString(){SamEffects.trace+="S";if(text==null)throw SamEffects.failure;return text;}}
public class RemainingSAMReview{
 static Predicate<String> nested(String prefix){return outer->{Predicate<String> inner=value->outer.equals(value)&&value.startsWith(prefix)&&SamEffects.touch(value);return inner.test(outer);};}
 static Supplier<Boolean> supplied(List<String>list){return ()->list.stream().anyMatch(value->value.equals(list.get(0))&&SamEffects.touch(value));}
 static String describe(List<SamItem>items){return items.stream().map(SamItem::toString).collect(Collectors.joining(","));}
 static void weak(PrintWriter writer,List<WeakReference<?>>items){items.forEach(item->writer.println(item.get()));}
 static void gauge(SamRegistry registry,SamStats stats){registry.register("gauge",(SamGauge<Integer>)stats::pull);}
 static Consumer<String> annotation(Map<String,String>attributes,String name){return value->attributes.put(name,value);}
 static <N>Iterator<N>pair(N a,N b){return SamPack.forArr(a,b);}
 static Iterator<String>named(String[]array){return SamPack.forArr(array);}
 public static void main(String[]args){
 for(String prefix:new String[]{"a","","z",null})for(String value:new String[]{"abc","throw",null}){SamEffects.trace="";try{System.out.println(nested(prefix).test(value)+":"+SamEffects.trace);}catch(Throwable error){System.out.println(error.getClass().getName()+":"+(error==SamEffects.failure)+":"+SamEffects.trace);}}
 List<String>list=new ArrayList<>(Arrays.asList("abc","other"));Supplier<Boolean>saved=supplied(list);System.out.println(saved.get());list.set(0,"throw");try{saved.get();}catch(Throwable error){System.out.println(error==SamEffects.failure);}
 SamEffects.trace="";System.out.println(describe(Arrays.asList(new SamItem("x"),new SamItem("y")))+":"+SamEffects.trace);try{describe(Arrays.asList(new SamItem("x"),new SamItem(null)));}catch(Throwable error){System.out.println((error==SamEffects.failure)+":"+SamEffects.trace);}
 StringWriter output=new StringWriter();PrintWriter writer=new SamWriter(output);SamEffects.trace="";String strong="kept";WeakReference<String>present=new WeakReference<>(strong);WeakReference<String>empty=new WeakReference<>(null);weak(writer,Arrays.asList(present,empty));System.out.println(output.toString().replace("\r","").replace("\n","|")+":"+SamEffects.trace);
 SamStats stats=new SamStats();SamRegistry registry=new SamRegistry();SamEffects.trace="";gauge(registry,stats);System.out.println(((SamGauge)registry.saved).get()+":"+((SamGauge)registry.saved).get()+":"+SamEffects.trace);stats.fail=true;try{((SamGauge)registry.saved).get();}catch(Throwable error){System.out.println(error==SamEffects.failure);}SamEffects.trace="";try{gauge(registry,null);}catch(Throwable error){System.out.println(error.getClass().getName()+":"+SamEffects.trace);}
 Map<String,String>map=new LinkedHashMap<>();Consumer<String>callback=annotation(map,"name");callback.accept("first");callback.accept(null);System.out.println(map.containsKey("name")+":"+map.get("name"));
 Object marker=new Object();Iterator<Object>pairs=pair(marker,null);Object[]packed=SamPack.last;System.out.println((pairs.next()==marker)+":"+(pairs.next()==null)+":"+packed.getClass().getComponentType().getName());
 String[]array={"original","second"};Iterator<String>names=named(array);System.out.println((SamPack.last==array)+":"+array.getClass().getComponentType().getName());array[0]="mutated";System.out.println(names.next());try{named(null);}catch(Throwable error){System.out.println(error.getClass().getName());}
 }
}`, Precision, Compatibility, "legacy")
}

// Check the actual metafactory target and erased/instantiated SAM tuple, rather
// than inferring the target from the generated spelling of a method reference.
func assertReviewedRemainingSAMTarget(t *testing.T, raw []byte, owner, name, desc, erased, instantiated string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	found := 0
	for _, attr := range object.Attributes {
		bootstrap, ok := attr.(*BootstrapMethodsAttribute)
		if !ok {
			continue
		}
		for _, site := range bootstrap.BootstrapMethods {
			if len(site.BootstrapArguments) != 3 {
				continue
			}
			target, ok := cp.IndexInfo(int(site.BootstrapArguments[1])).(*ConstantMethodHandleInfo)
			if !ok {
				continue
			}
			member, ok := cp.IndexInfo(int(target.ReferenceIndex)).(*ConstantMethodrefInfo)
			if !ok {
				continue
			}
			pair, ok := cp.IndexInfo(int(member.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
			if !ok {
				continue
			}
			if cp.GetClassName(int(member.ClassIndex)) != owner || cp.GetUtf8(int(pair.NameIndex)).Value != name || cp.GetUtf8(int(pair.DescriptorIndex)).Value != desc {
				continue
			}
			sam, ok := cp.IndexInfo(int(site.BootstrapArguments[0])).(*ConstantMethodTypeInfo)
			if !ok {
				t.Fatal("SAM method type missing")
			}
			inst, ok := cp.IndexInfo(int(site.BootstrapArguments[2])).(*ConstantMethodTypeInfo)
			if !ok || cp.GetUtf8(int(sam.DescriptorIndex)).Value != erased || cp.GetUtf8(int(inst.DescriptorIndex)).Value != instantiated {
				t.Fatal("original SAM adaptation changed")
			}
			found++
		}
	}
	if found != 1 {
		t.Fatalf("original SAM target %s.%s%s appeared %d times", owner, name, desc, found)
	}
}

func reviewedRemainingSAMRaw(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/regression/" + name + ".class")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Host inspection of original binding tuples only; the six-mode behavior
// oracle executes authored source separately.

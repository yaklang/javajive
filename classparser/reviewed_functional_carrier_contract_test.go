package javaclassparser

import (
	"reflect"
	"regexp"
	"sort"
	"testing"
)

// The instantiated SAM, not source spelling or neighboring receiver inference,
// determines which payload checks occur before the functional body starts.
func assertReviewedSAMInstantiation(t *testing.T, raw []byte, expected ...string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	var actual []string
	for _, attribute := range object.Attributes {
		bootstrap, ok := attribute.(*BootstrapMethodsAttribute)
		if !ok {
			continue
		}
		for _, method := range bootstrap.BootstrapMethods {
			handle, ok := cp.IndexInfo(int(method.BootstrapMethodRef)).(*ConstantMethodHandleInfo)
			if !ok || handle.ReferenceKind != 6 {
				t.Fatal("fixture bootstrap is not a static method handle")
			}
			reference, ok := cp.IndexInfo(int(handle.ReferenceIndex)).(*ConstantMethodrefInfo)
			if !ok || cp.GetClassName(int(reference.ClassIndex)) != "java/lang/invoke/LambdaMetafactory" {
				t.Fatal("fixture bootstrap must be the original LambdaMetafactory")
			}
			if len(method.BootstrapArguments) != 3 {
				t.Fatal("fixture needs independent review for nonstandard bootstrap arguments")
			}
			sam, ok := cp.IndexInfo(int(method.BootstrapArguments[0])).(*ConstantMethodTypeInfo)
			if !ok {
				t.Fatal("missing original erased SAM")
			}
			instantiated, ok := cp.IndexInfo(int(method.BootstrapArguments[2])).(*ConstantMethodTypeInfo)
			if !ok {
				t.Fatal("missing original instantiated SAM")
			}
			actual = append(actual, cp.GetUtf8(int(sam.DescriptorIndex)).Value+" => "+cp.GetUtf8(int(instantiated.DescriptorIndex)).Value)
		}
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("original instantiated SAM contracts=%q, want=%q", actual, expected)
	}
}

func reviewedFunctionalCarrier(t *testing.T, source, declaration, initializer string) string {
	t.Helper()
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(compactReviewedGenericSource(declaration)) + `([A-Za-z_$][\w$]*)=` + regexp.QuoteMeta(compactReviewedGenericSource(initializer)))
	match := re.FindStringSubmatch(compactReviewedGenericSource(source))
	if len(match) != 2 {
		t.Fatalf("missing independently typed %s carrier initialized by %s: %s", declaration, initializer, source)
	}
	return match[1]
}

func TestAdversarialReviewedFunctionalCarriersRoundTrip(t *testing.T) {
	// Five old inline-spelling switches are now superseded by materialization
	// and bootstrap binding. Keep the still-active instantiated-type gate ON.
	t.Setenv("JDEC_METHODREF_INSTANTIATED_TYPE_OFF", "")
	for _, key := range []string{"JDEC_CTOR_RAWFI_METHODREF_CAST_OFF", "JDEC_DOPRIVILEGED_LAMBDA_CAST_OFF", "JDEC_LAMBDA_ASSIGN_CAST_OFF", "JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF", "JDEC_LAMBDA_RAWRECV_CAST_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "ReviewedCarriers", `import java.util.*;import java.util.function.*;import java.util.stream.*;import java.security.*;
class FIPayload{final String name;final boolean flag;FIPayload(String name,boolean flag){this.name=name;this.flag=flag;}Object val(){FIHelpers.trace.append("M");return name;}}
class FIHelpers {
 static StringBuilder trace=new StringBuilder();
 static final Function identity=(Function<Object,Object>)value->value;
 static List<FIPayload> items(int kind){trace.append("I");if(kind==0)return null;if(kind==1)return new ArrayList<>();if(kind==2)return Arrays.asList(new FIPayload("a",true),null,new FIPayload("b",false));return (List)Arrays.asList(new FIPayload("a",true),"wrong");}
 static void seen(Object value){trace.append("P");}
 static FIHelpers owner(boolean absent){trace.append("R");return absent?null:new FIHelpers();}
 Object read(){trace.append("D");return "domain";}
}
class FIReceiver<T>{void send(Consumer<FIPayload> action,Object value){FIHelpers.trace.append("S");((Consumer)action).accept(value);}}
class FIBiSink{final BiConsumer action;FIBiSink(String name,BiConsumer action){this.action=action;}void apply(Object left,Object right){FIHelpers.trace.append("B");action.accept(left,right);}}
public class ReviewedCarriers {
 final Function builder=FIHelpers.identity;
 static final Comparator<String> order=String::compareTo;
 Function pick(int kind){Function value=builder;if(kind==1)value=(Function<Collection,Collection>)Collections::unmodifiableCollection;else if(kind==2)value=(Function<Collection,Collection>)((Collection values)->Collections.singleton(values.iterator().next()));return value;}
 List<Object> collect(int kind){List<FIPayload> values=FIHelpers.items(kind);if(values==null)return null;return values.stream().filter(Objects::nonNull).peek(FIHelpers::seen).map((FIPayload value)->value.val()).collect(Collectors.toList());}
 void receive(Object receiver,Object value,List<String> out){FIReceiver raw=(FIReceiver)receiver;raw.send((Consumer<FIPayload>)payload->{FIHelpers.trace.append("L");if(payload.flag)out.add(payload.name);},value);}
 FIBiSink sink(){return new FIBiSink("trace",(BiConsumer<Throwable,StackTraceElement[]>)Throwable::setStackTrace);}
 Object privileged(boolean absent){return AccessController.doPrivileged((PrivilegedAction<Object>)FIHelpers.owner(absent)::read);}
 static Object action(){return AccessController.doPrivileged((PrivilegedAction<Object>)()->{FIHelpers.trace.append("A");return "action";});}
 public static void main(String[]args){
  ReviewedCarriers consumer=new ReviewedCarriers();
  for(int kind=0;kind<4;kind++)try{System.out.println(consumer.collect(kind));}catch(Throwable failure){System.out.println(failure.getClass().getName());}
  System.out.println(FIHelpers.trace);FIHelpers.trace.setLength(0);
  for(int kind=0;kind<3;kind++)for(Object input:new Object[]{Arrays.asList("x","y"),Collections.emptyList(),null,"wrong"})try{Function fn=consumer.pick(kind);System.out.println(fn==FIHelpers.identity);Object result=fn.apply(input);System.out.println(result);}catch(Throwable failure){System.out.println(failure.getClass().getName());}
  for(Object input:new Object[]{new FIPayload("yes",true),new FIPayload("no",false),null,"wrong"}){List<String> out=new ArrayList<>();try{consumer.receive(new FIReceiver<String>(),input,out);}catch(Throwable failure){System.out.println(failure.getClass().getName());}System.out.println(out);}
  System.out.println(FIHelpers.trace);FIHelpers.trace.setLength(0);
  Throwable throwable=new Throwable();for(Object left:new Object[]{throwable,null,"wrong"})for(Object right:new Object[]{new StackTraceElement[0],null,"wrong"})try{consumer.sink().apply(left,right);System.out.println("set");}catch(Throwable failure){System.out.println(failure.getClass().getName());}
  for(String left:new String[]{"a","b",null})for(String right:new String[]{"a","b",null})try{System.out.println(Integer.signum(order.compare(left,right)));}catch(Throwable failure){System.out.println(failure.getClass().getName());}
  for(boolean absent:new boolean[]{false,true})try{System.out.println(consumer.privileged(absent));}catch(Throwable failure){System.out.println(failure.getClass().getName());}
  System.out.println(action());System.out.println(FIHelpers.trace);
 }
}`, Precision, Compatibility, "legacy")
}

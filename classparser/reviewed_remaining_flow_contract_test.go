package javaclassparser

import "testing"

func TestAdversarialRemainingNumericReadConcatAndInitRoundTrip(t *testing.T) {
	for _, flag := range []string{"JDEC_ORIG14_REMAINING_OFF", "JDEC_FREEMARKER_REMAINING_OFF", "JDEC_LOGBACK_REMAINING_OFF"} {
		t.Setenv(flag, "1")
	}
	roundTripGenericFlowUnits(t, "RemainingFlowReview", `import java.util.*;import java.io.*;import java.lang.reflect.*;
class FlowEffects{static String trace="";static final IOException readError=new IOException("original");static final Exception initError=new Exception("original");static boolean failCtor;static <E extends Throwable>void sneaky(Throwable error)throws E{throw (E)error;}static final RuntimeException runtime=new IllegalStateException("original");static Object markup(boolean fail)throws Exception{trace+="M";if(fail)throw initError;return new Object();}}
class FlowWrap{final Object value;FlowWrap(Object value){FlowEffects.trace+="W";if(FlowEffects.failCtor)FlowEffects.<RuntimeException>sneaky(FlowEffects.initError);this.value=value;}}
class FlowGoodInit{static final Object MARK;static final FlowWrap FIRST,SECOND;static{try{MARK=FlowEffects.markup(false);}catch(Exception error){throw new IllegalStateException(error);}FIRST=new FlowWrap(MARK);SECOND=new FlowWrap(MARK);}}

class FlowBadInit{static final Object MARK;static{try{MARK=FlowEffects.markup(true);}catch(Exception error){throw new IllegalStateException(error);}}}
class FlowReader{final int value;final int fail;FlowReader(int value,int fail){this.value=value;this.fail=fail;}int read()throws IOException{FlowEffects.trace+="R";if(fail==1)throw FlowEffects.readError;return value;}void handle(int value){FlowEffects.trace+="H";if(fail==2)throw FlowEffects.runtime;}RuntimeException remember(Exception error){FlowEffects.trace+="E";return new RuntimeException(error);}}
class FlowText{final String text;FlowText(String text){this.text=text;}public String toString(){FlowEffects.trace+="S";if(text==null)throw FlowEffects.runtime;return text;}}
public class RemainingFlowReview{
 static String comma(List<String>items){StringBuilder text=new StringBuilder();for(int index=0;index<items.size();index++){if(index!=0)text.append(',');text.append(items.get(index));}return text.toString();}
 static int read(FlowReader reader){try{int value=reader.read();reader.handle(value);return value;}catch(Exception error){throw reader.remember(error);}}
 static byte[]encode(Object value,String separator){return (String.valueOf(value)+separator).getBytes();}
 static Method selected(Optional<Method>method)throws NoSuchMethodException{return method.orElseThrow(()->new NoSuchMethodException("missing"));}
 public static void main(String[]args)throws Exception{
 for(List<String>items:Arrays.asList(Collections.<String>emptyList(),Arrays.asList("first"),Arrays.asList("first","second",null))){System.out.println(comma(items));}
 for(int value:new int[]{-1,0,1,127,65535})for(int fail:new int[]{0,1,2}){FlowEffects.trace="";try{System.out.println(read(new FlowReader(value,fail))+":"+FlowEffects.trace);}catch(RuntimeException error){System.out.println((error.getCause()==(fail==1?FlowEffects.readError:FlowEffects.runtime))+":"+FlowEffects.trace);}}
 for(Object value:new Object[]{null,"text",new FlowText("custom"),new FlowText(null),new char[]{'a','b'}}){FlowEffects.trace="";try{byte[] bytes=encode(value,"!");System.out.println((value instanceof char[]?new String(bytes).endsWith("!"):new String(bytes))+":"+FlowEffects.trace);}catch(Throwable error){System.out.println((error==FlowEffects.runtime)+":"+FlowEffects.trace);}}
 Method method=RemainingFlowReview.class.getDeclaredMethod("comma",List.class);System.out.println(selected(Optional.of(method))==method);try{selected(Optional.empty());}catch(NoSuchMethodException error){System.out.println(error.getMessage());}
 FlowEffects.trace="";System.out.println((FlowGoodInit.FIRST.value==FlowGoodInit.MARK)+":"+(FlowGoodInit.SECOND.value==FlowGoodInit.MARK)+":"+FlowEffects.trace);try{Object value=FlowBadInit.MARK;}catch(ExceptionInInitializerError error){System.out.println((error.getCause().getCause()==FlowEffects.initError)+":"+FlowEffects.trace);}
 }
}`, nil, []string{"FlowGoodInit", "FlowBadInit"}, Precision, Compatibility, "legacy")
}

// The checked initialization producer is protected; the later constructor is
// outside that interval. A generic sneaky throw is legal JVM behavior, so its
// original exception object must not become a new catch wrapper.
func TestAdversarialUnprotectedClinitConstructorFailureDomainRoundTrip(t *testing.T) {
	t.Setenv("JDEC_FREEMARKER_REMAINING_OFF", "1")
	roundTripGenericFlowUnits(t, "ClinitFailureDriver", `class InitDomainSupport{static String trace="";static final Exception failure=new Exception("original");static <E extends Throwable>void sneaky(Throwable error)throws E{throw (E)error;}static Object markup()throws Exception{trace+="M";return new Object();}}
class InitDomainValue{InitDomainValue(Object value){InitDomainSupport.trace+="C";InitDomainSupport.<RuntimeException>sneaky(InitDomainSupport.failure);}}
class InitDomainConsumer{static final Object MARK;static final InitDomainValue VALUE;static{try{MARK=InitDomainSupport.markup();}catch(Exception error){throw new IllegalStateException(error);}VALUE=new InitDomainValue(MARK);}}
public class ClinitFailureDriver{public static void main(String[]args){try{Object value=InitDomainConsumer.VALUE;}catch(ExceptionInInitializerError error){System.out.println((error.getCause()==InitDomainSupport.failure)+":"+InitDomainSupport.trace);}}}
`, nil, []string{"InitDomainConsumer"}, Precision, Compatibility, "legacy")
}

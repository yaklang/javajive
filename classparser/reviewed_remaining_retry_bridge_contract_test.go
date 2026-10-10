package javaclassparser

import "testing"

func TestAdversarialSyntheticConstructorDelegationIdentityRoundTrip(t *testing.T) {
	t.Setenv("JDEC_SYN_BRIDGE_THIS_OFF", "1")
	roundTripGenericFlowUnits(t, "BridgeCtorReview", `class BridgeEvents{static String trace="";static boolean fail;static final RuntimeException failure=new IllegalArgumentException("original");}
public class BridgeCtorReview{private static class Base{private Base(){BridgeEvents.trace+="B";if(BridgeEvents.fail)throw BridgeEvents.failure;}}private static final class Sub extends Base{private Sub(){BridgeEvents.trace+="S";}}static Base make(){return new Sub();}public static void main(String[]args){for(boolean fail:new boolean[]{false,true}){BridgeEvents.trace="";BridgeEvents.fail=fail;try{Object value=make();System.out.println((value!=null)+":"+BridgeEvents.trace);}catch(Throwable error){System.out.println((error==BridgeEvents.failure)+":"+BridgeEvents.trace);}}}}
`, nil, []string{"BridgeCtorReview$Base", "BridgeCtorReview$Sub", "BridgeCtorReview$1"}, Precision, Compatibility, "legacy")
}

func TestAdversarialConnectionRetryCleanupDomainRoundTrip(t *testing.T) {
	t.Setenv("JDEC_REALCONNECTION_CONNECT_OFF", "1")
	roundTripGenericFlow(t, "RetryConnectionReview", `import java.io.*;
class RetryEffects{static String trace="";static IOException first=new IOException("first"),second=new IOException("second");}
class RetryPlan{final boolean tunnel;final int phase;final boolean again;int attempts;RetryPlan(boolean tunnel,int phase,boolean again){this.tunnel=tunnel;this.phase=phase;this.again=again;}void step(int stage)throws IOException{RetryEffects.trace+=stage;if(stage==phase){if(attempts++==0)throw RetryEffects.first;if(again)throw RetryEffects.second;}}}
class RetryError extends RuntimeException{final IOException first;RetryError(IOException error){super(error);first=error;}void append(IOException error){first.addSuppressed(error);}}
public class RetryConnectionReview{Object socket,protocol;void tunnel(RetryPlan plan)throws IOException{plan.step(1);socket=new Object();}void socket(RetryPlan plan)throws IOException{plan.step(2);socket=new Object();}void establish(RetryPlan plan)throws IOException{plan.step(3);protocol=new Object();}void event(RetryPlan plan){RetryEffects.trace+="E";}void cleanup(){RetryEffects.trace+="C";socket=null;protocol=null;}
 void connect(RetryPlan plan,boolean retry){RetryError saved=null;int failures=0;while(true){try{if(plan.tunnel)tunnel(plan);else socket(plan);establish(plan);event(plan);break;}catch(IOException error){cleanup();if(saved==null)saved=new RetryError(error);else saved.append(error);if(!retry||++failures>=2)throw saved;}}}
 public static void main(String[]args){for(boolean tunnel:new boolean[]{false,true})for(int phase:new int[]{0,1,2,3})for(boolean again:new boolean[]{false,true})for(boolean retry:new boolean[]{false,true}){RetryEffects.first=new IOException("first");RetryEffects.second=new IOException("second");RetryEffects.trace="";RetryConnectionReview owner=new RetryConnectionReview();try{owner.connect(new RetryPlan(tunnel,phase,again),retry);System.out.println((owner.socket!=null)+":"+(owner.protocol!=null)+":"+RetryEffects.trace);}catch(RetryError error){Throwable[]suppressed=error.first.getSuppressed();System.out.println((error.first==RetryEffects.first)+":"+(error.getCause()==RetryEffects.first)+":"+(suppressed.length==0||suppressed[0]==RetryEffects.second)+":"+(owner.socket==null)+":"+(owner.protocol==null)+":"+RetryEffects.trace);}}}}
`, Precision, Compatibility, "legacy")
}

func TestAdversarialOptionalFlatMapFindFirstRoundTrip(t *testing.T) {
	t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", "1")
	roundTripGenericFlow(t, "OptionalFirstReview", `import java.util.*;import java.util.stream.*;
class OptionalEffects{static String trace="";static final RuntimeException failure=new IllegalStateException("original");static Optional<Class<?>>seen(Optional<Class<?>>value){trace+="L";return value;}static RuntimeException missing(){trace+="E";return failure;}}
public class OptionalFirstReview{static Class<?>first(List<Optional<Class<?>>>values){return values.stream().map(OptionalEffects::seen).flatMap(value->value.map(Stream::of).orElse(Stream.empty())).findFirst().orElseThrow(OptionalEffects::missing);}public static void main(String[]args){List[]cases={Collections.emptyList(),Arrays.asList(Optional.empty()),Arrays.asList(Optional.empty(),Optional.of(String.class),Optional.of(Integer.class)),Arrays.asList(Optional.of(Integer.class),Optional.empty()),Arrays.asList((Optional)null),Arrays.asList(Optional.of("foreign"))};for(List values:cases){OptionalEffects.trace="";try{Class<?>result=first(values);System.out.println(result.getName()+":"+OptionalEffects.trace);}catch(Throwable error){System.out.println(error.getClass().getName()+":"+(error==OptionalEffects.failure)+":"+OptionalEffects.trace);}}}}
`, Precision, Compatibility, "legacy")
}

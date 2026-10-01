package javaclassparser

import "testing"

func TestAdversarialBoundMethodReferenceErasedReceiverRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedReceiver", `import java.util.function.*;
class ReceiverCheck<T> {
 static int trace;
 boolean test(T value) { trace=trace*10+1; return value!=null; }
 boolean test(String value) { trace=trace*10+2; return value!=null; }
}
class StringReceiverCheck extends ReceiverCheck<String> {
 boolean test(String value) { trace=trace*10+3; return value==null || value.length()>1; }
}
public class ErasedReceiver {
 static <T> Predicate<Object> capture(ReceiverCheck<? super T> owner) { Predicate<Object> f=((ReceiverCheck)owner)::test; return f; }
 static Predicate<Object> concrete(ReceiverCheck<String> owner) { Predicate<Object> f=((ReceiverCheck)owner)::test; return f; }
 static String run(int ownerMode,int inputMode,boolean wildcard) {
  ReceiverCheck.trace=0;
  ReceiverCheck<String> owner=ownerMode==0?new ReceiverCheck<String>():ownerMode==1?new StringReceiverCheck():null;
  Object input=inputMode==0?null:inputMode==1?"a":inputMode==2?"long":Integer.valueOf(inputMode);
  try { Predicate<Object> f=wildcard?capture(owner):concrete(owner); return f.test(input)+":"+ReceiverCheck.trace; }
  catch(Throwable failure) { return failure.getClass().getName()+":"+ReceiverCheck.trace; }
 }
 public static void main(String[] args) {
  for(int o=0;o<3;o++) for(int i=0;i<12;i++) { System.out.println(o+":"+i+":"+run(o,i,true));System.out.println(o+":"+i+":"+run(o,i,false)); }
 }
}`, Precision, Compatibility, "legacy")
}

package javaclassparser

import "testing"

func TestAdversarialErasedReceiverScalarResultKeepsEntryEffects(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ScalarReceiverDriver", `import java.util.Comparator;
interface ScalarFold<T> { int apply(T first,T second); }
class ScalarEvidence {
 static int trace, mode;
 static final RuntimeException sentinel=new IllegalStateException("sentinel");
 static final Object token=new Object();
 static final ScalarFold<String> fold=new ScalarFold<String>() {
  public int apply(String first,String second) { trace=trace*10+3; if(mode==3)throw sentinel; return first==null?17:first.length(); }
 };
 static final Comparator<String> comparator=new Comparator<String>() {
  public int compare(String first,String second){ trace=trace*10+4; if(mode==3)throw sentinel; return first==null?23:first.length(); }
 };
 static Object input(){trace=trace*10+1;return mode==0?null:mode==1?"value":mode==2?token:"sentinel";}
 static <T> ScalarFold<? super T> fold(){trace=trace*10+2;return (ScalarFold)fold;}
 static <T> Comparator<? super T> comparator(){trace=trace*10+2;return (Comparator)comparator;}
}

class ScalarParent<T> {
 public ScalarFold<? super T> fold(){return ScalarEvidence.<T>fold();}
 public Comparator<? super T> comparator(){return ScalarEvidence.<T>comparator();}
}
public class ScalarReceiverDriver<T> extends ScalarParent<T> {
 int folded(T value){return ((ScalarFold)fold()).apply(ScalarEvidence.input(),value);}
 int compared(T value){return ((Comparator)comparator()).compare(ScalarEvidence.input(),value);}
 public static void main(String[]args){
  for(int m=0;m<4;m++)for(int kind=0;kind<2;kind++) {
   ScalarEvidence.mode=m;ScalarEvidence.trace=0;
   try {
    ScalarReceiverDriver<String> driver=new ScalarReceiverDriver<String>();
    int result=kind==0?driver.folded("second"):driver.compared("second");
    int expected=m==0?(kind==0?17:23):5;
    if(result!=expected || ScalarEvidence.trace!=(kind==0?213:214))throw new AssertionError("result/effects");
    System.out.println(m+":"+kind+":ok:"+result+":"+ScalarEvidence.trace);
   }catch(RuntimeException failure){
    if(m==2){if(!(failure instanceof ClassCastException)||ScalarEvidence.trace!=21)throw new AssertionError("bridge");}
    else if(m==3){if(failure!=ScalarEvidence.sentinel||ScalarEvidence.trace!=(kind==0?213:214))throw new AssertionError("callee");}
    else throw new AssertionError("unexpected",failure);
    System.out.println(m+":"+kind+":"+failure.getClass().getName()+":"+ScalarEvidence.trace);
   }
  }
 }
}`, Precision, Compatibility, "legacy")
}

// The caller's variable has a narrower first bound than Comparator's erased
// Object input. Inventing a (N) check would reject a payload accepted by the
// original raw invocation, before the comparator's observable entry effects.
func TestAdversarialSourceBridgeBoundedReceiverDoesNotInventPayloadCheck(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BoundedBridgeDriver", `import java.util.Comparator;
class BoundedEvidence {
 static int trace,mode;
 static final RuntimeException sentinel=new IllegalArgumentException("identity");
 static final Object token=new Object();
 static final Comparator<Object> comparator=new Comparator<Object>() {
  public int compare(Object first,Object second){trace=trace*10+3;if(mode==3)throw sentinel;return first==token?19:first==null?23:29;}
 };
 static Object input(){trace=trace*10+1;return mode==0?null:mode==1?"unrelated":mode==2?token:Integer.valueOf(3);}
}
class BoundedBridgeParent<N extends Number> {
 public Comparator<? super N> comparator(){BoundedEvidence.trace=BoundedEvidence.trace*10+2;return (Comparator)BoundedEvidence.comparator;}
}
public class BoundedBridgeDriver<N extends Number> extends BoundedBridgeParent<N> {
 int call(N second){return ((Comparator)comparator()).compare(BoundedEvidence.input(),second);}
 public static void main(String[]args){for(int mode=0;mode<4;mode++){
  BoundedEvidence.trace=0;BoundedEvidence.mode=mode;
  try{int result=new BoundedBridgeDriver<Integer>().call(Integer.valueOf(7));
   if(result!=(mode==0?23:mode==1?29:19)||BoundedEvidence.trace!=213)throw new AssertionError("result/effects");
   System.out.println(mode+":ok:"+result+":"+BoundedEvidence.trace);
  }catch(RuntimeException failure){if(mode!=3||failure!=BoundedEvidence.sentinel||BoundedEvidence.trace!=213)throw new AssertionError("invented check",failure);System.out.println(mode+":sentinel:"+BoundedEvidence.trace);}
 }}
}`, Precision, Compatibility, "legacy")
}

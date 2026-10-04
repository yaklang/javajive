package javaclassparser

import "testing"

// Capture temporaries must retain the solved type of the value copied at the
// invokedynamic, and its identity when the live local is subsequently assigned.
func TestAdversarialLambdaJoinCaptureRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "JoinedCapture", `import java.util.*;import java.util.function.*;
public class JoinedCapture {
  static void run(int kind,int count,boolean binary) {
    CaptureAccumulator accumulator=null;
    List<IntConsumer> queued=new ArrayList<>();
    for(int n=0;n<count;n++) {
      if(accumulator==null) {
        if(kind==0)accumulator=new CaptureSingle(n);
        else if(kind==1)accumulator=new CaptureNumeric(n);
        else accumulator=new CaptureBinary(n);
      }
      final CaptureAccumulator target=accumulator;
      final CapturePayload payload;
      if((n&1)==0)payload=null;
      else payload=new CapturePayload(n);
      final long value=n*0x100000003L;
      IntConsumer consumer;
      if(n%3==0)consumer=id -> target.reset(id);
      else if(binary)consumer=id -> target.add(id,payload);
      else consumer=id -> target.add(id,value);
      queued.add(consumer);
      accumulator=null;
    }
    CaptureJoinOracle.mark("queued");
    int id=0;for(IntConsumer consumer:queued)consumer.accept(id++);
  }
  public static void main(String[] args){CaptureJoinOracle.run();}
}
class CapturePayload {
  final int value;CapturePayload(int n){CaptureJoinOracle.mark("payload"+n);value=n;}
}
class CaptureAccumulator {
  final int value;CaptureAccumulator(int n){CaptureJoinOracle.mark("base"+n);value=n;}
  void reset(int id){CaptureJoinOracle.mark("reset:"+value+":"+id);}
  void add(int id,long n){CaptureJoinOracle.mark("long:"+value+":"+id+":"+n);}
  void add(int id,CapturePayload p){CaptureJoinOracle.mark("payload:"+value+":"+id+":"+(p==null ? -1 : p.value));}
}
class CaptureSingle extends CaptureAccumulator {
  CaptureSingle(int n){super(n);CaptureJoinOracle.mark("single"+n);}
  void add(int id,Object p){CaptureJoinOracle.mark("wrong-object-overload");}
}
class CaptureNumeric extends CaptureAccumulator {
  CaptureNumeric(int n){super(n);CaptureJoinOracle.mark("numeric"+n);}
}
class CaptureBinary extends CaptureAccumulator {
  CaptureBinary(int n){super(n);CaptureJoinOracle.mark("binary"+n);}
}
class CaptureJoinOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String label){trace.append(label).append(';');if(++step==failAt)throw new IllegalStateException(label);}
  static void run(){
    for(int kind=0;kind<3;kind++)for(int count:new int[]{0,1,2,4,7})for(int binary=0;binary<2;binary++)for(failAt=0;failAt<24;failAt++) {
      trace=new StringBuilder();step=0;String result;
      try{JoinedCapture.run(kind,count,binary!=0);result="ok";}
      catch(Throwable e){result=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(kind+":"+count+":"+binary+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}

package javaclassparser

import "testing"

// The inner supplier is created after the loop has overwritten its mutable
// working local. Both lambda layers must still use the value captured when the
// outer factory was created, with the same exception and allocation trace.
func TestAdversarialNestedCaptureSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedCaptureSnapshot", `import java.util.*;import java.util.function.*;
public class NestedCaptureSnapshot {
  static long run(int seed,int count,boolean reverse) {
    List<Supplier<LongSupplier>> queued=new ArrayList<>();
    CaptureCell working=null;
    for(int n=0;n<count;n++) {
      working=new CaptureCell(seed+n);
      final CaptureCell snapshot=working;
      final long amount=(seed-n)*0x100000003L;
      Supplier<LongSupplier> factory=()->{
        NestedCaptureOracle.mark("factory:"+snapshot.value);
        final CaptureCell inner=snapshot;
        return ()->{NestedCaptureOracle.mark("inner:"+inner.value);return inner.read(amount);};
      };
      queued.add(factory);
      working=null;
    }
    long result=0;
    for(int n=0;n<count;n++) {
      int index=reverse ? count-n-1 : n;
      result=result*31+queued.get(index).get().getAsLong();
    }
    return result;
  }
  public static void main(String[] args){NestedCaptureOracle.run();}
}
class CaptureCell {
  final int value;
  CaptureCell(int v){NestedCaptureOracle.mark("new:"+v);value=v;}
  long read(long amount){NestedCaptureOracle.mark("read:"+value+":"+amount);return amount^(value*0x9e3779b9L);}
}
class NestedCaptureOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String s){trace.append(s).append(';');if(++step==failAt)throw new IllegalStateException(s);}
  static void run(){
    for(int seed:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})for(int count:new int[]{0,1,4})for(int reverse=0;reverse<2;reverse++)for(failAt=0;failAt<12;failAt++){
      trace=new StringBuilder();step=0;String result;
      try{result=Long.toString(NestedCaptureSnapshot.run(seed,count,reverse!=0));}
      catch(Throwable e){result=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(seed+":"+count+":"+reverse+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}

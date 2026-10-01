package javaclassparser

import "testing"

// A fixed subclass is not a raw view of its generic ancestor. The bytecode
// descriptor selects the ancestor's erased entry, including when a bridge
// later rejects a polluted payload. Compare receiver/argument side effects and
// exception timing; merely making javac accept a narrower argument is unsafe.
func TestAdversarialInheritedErasedInvocationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedPool", `
class PoolItem { }
class SpecificItem extends PoolItem { }
class OtherItem extends PoolItem { }
class InvokeTrace { static int value; }
class GenericPool<E extends PoolItem> {
 public void release(E entry,boolean reuse) { InvokeTrace.value=InvokeTrace.value*10+4; }
 public void release(Object entry,boolean reuse) { InvokeTrace.value=InvokeTrace.value*10+5; }
 public void release(OtherItem entry,boolean reuse) { InvokeTrace.value=InvokeTrace.value*10+7; }
}
class FixedPool extends GenericPool<SpecificItem> { }
class OverridingPool extends FixedPool {
 public void release(SpecificItem entry,boolean reuse) { InvokeTrace.value=InvokeTrace.value*10+6; }
}
public class ErasedPool {
 static FixedPool owner(int mode,int fail) {
  InvokeTrace.value=InvokeTrace.value*10+1;
  if(fail==1) throw new IllegalStateException();
  return mode==0?new FixedPool():mode==1?new OverridingPool():null;
 }
 static PoolItem entry(int mode,int fail) {
  InvokeTrace.value=InvokeTrace.value*10+2;
  if(fail==2) throw new IllegalArgumentException();
  return mode==0?new SpecificItem():mode==1?new PoolItem():null;
 }
 static boolean reuse(int fail) {
  InvokeTrace.value=InvokeTrace.value*10+3;
  if(fail==3) throw new UnsupportedOperationException();
  return fail%2==0;
 }
 static String run(int mode,int input,int fail) {
  InvokeTrace.value=0;
  try { ((GenericPool)owner(mode,fail)).release(entry(input,fail),reuse(fail)); return "ok:"+InvokeTrace.value; }
  catch(Throwable failure) { return failure.getClass().getName()+":"+InvokeTrace.value; }
 }
 static String literalNull(int mode) {
  InvokeTrace.value=0;
  try { ((GenericPool)owner(mode,0)).release((PoolItem)null,reuse(0)); return "ok:"+InvokeTrace.value; }
  catch(Throwable failure) { return failure.getClass().getName()+":"+InvokeTrace.value; }
 }
 public static void main(String[] args) {
  for(int o=0;o<3;o++) for(int i=0;i<3;i++) for(int f=0;f<5;f++) System.out.println(o+":"+i+":"+f+":"+run(o,i,f));
  for(int o=0;o<3;o++) System.out.println("null:"+o+":"+literalNull(o));
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialFactoryErasedInvocationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ErasedFactoryCalls", `
interface InvokePair<A,B> { long apply(A a,B b); }
class PairOps {
 static int trace;
 static long apply(long a,int b) { trace=trace*10+3; return a*31+b; }
 static long apply(long a,long b) { trace=trace*10+4; return -999; }
}

public class ErasedFactoryCalls {
 static InvokePair<Long,Integer> factory(boolean missing) {
  PairOps.trace=PairOps.trace*10+1;
  if(missing) return null;
  InvokePair<Long,Integer> result=PairOps::apply; return result;
 }
 static Object input(int mode,int value) {
  PairOps.trace=PairOps.trace*10+2;
  return mode==0?Long.valueOf(value):mode==1?Integer.valueOf(value):mode==2?"bad":null;
 }
 static String run(boolean missing,int a,int b,int value) {
  PairOps.trace=0;
  try { long result=((InvokePair)factory(missing)).apply(input(a,value),input(b,value+1)); return result+":"+PairOps.trace; }
  catch(Throwable failure) { return failure.getClass().getName()+":"+PairOps.trace; }
 }
 public static void main(String[] args) {
  for(int n=0;n<2;n++) for(int a=0;a<4;a++) for(int b=0;b<4;b++) for(int x=-2;x<=2;x++) System.out.println(n+":"+a+":"+b+":"+x+":"+run(n==1,a,b,x));
 }
}`, Precision, Compatibility, "legacy")
}

// Hide the generic ancestor's bytes while retaining its fixed child's Signature
// and the argument's known subtype edge. Missing overload metadata must not
// justify a new widening cast that makes a valid inherited call ill-typed.
func TestAdversarialMissingAncestorWideningRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowWithResolverFilter(t, "MissingAncestorCalls", `
class HiddenItem { }
class HiddenSpecific extends HiddenItem { }
class MissingTrace { static int value; }
class HiddenOwner<E extends HiddenItem> {
 public void take(E value,boolean flag) { MissingTrace.value=MissingTrace.value*10+3; }
}
class VisibleFixed extends HiddenOwner<HiddenSpecific> { }
class VisibleOverride extends VisibleFixed {
 public void take(HiddenSpecific value,boolean flag) { MissingTrace.value=MissingTrace.value*10+4; }
}
public class MissingAncestorCalls {
 static VisibleFixed owner(int mode) { MissingTrace.value=MissingTrace.value*10+1; return mode==0?new VisibleFixed():mode==1?new VisibleOverride():null; }
 static HiddenSpecific payload(boolean missing) { MissingTrace.value=MissingTrace.value*10+2; return missing?null:new HiddenSpecific(); }
 static String run(int mode,boolean missing) {
  MissingTrace.value=0;
  try { owner(mode).take(payload(missing),true); return "ok:"+MissingTrace.value; }
  catch(Throwable failure) { return failure.getClass().getName()+":"+MissingTrace.value; }
 }
 public static void main(String[] args) { for(int o=0;o<3;o++) for(int n=0;n<2;n++) System.out.println(o+":"+n+":"+run(o,n==0)); }
}`, func(name string) bool { return name != "HiddenOwner" }, Precision, Compatibility, "legacy")
}

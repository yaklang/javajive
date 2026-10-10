package javaclassparser

import "testing"

// The exception parameter is absent from instantiatedMethodType. Recover input
// bindings from the exact SAM Signature, keep the exception existential, and
// leave invocation adaptation to the method reference. A replacement lambda
// can move receiver checks or add a cast to a discarded generic result.
func TestAdversarialThrowsOnlyFunctionalBindingRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ThrowsBinding", `
interface FaultAction<T,X extends Throwable> { void act(T value) throws X; }
interface FaultPair<A,B,X extends Throwable> { long act(A a,B b) throws X; }
interface FaultFactory<R,X extends Throwable> { R make() throws X; }
class BindingOps {
 static int trace;
 static void raise(Throwable value) throws Throwable { trace=trace*10+1; if(value!=null) throw value; }
 static long combine(long a,int b) { trace=trace*10+2; return a*31+b; }
 static long combine(long a,long b) { trace=trace*10+3; return -999; }
 static <R> R erased() { trace=trace*10+4; return (R)(Object)Integer.valueOf(7); }
 void member(Throwable value) throws Throwable { trace=trace*10+5; if(value!=null) throw value; }
}
public class ThrowsBinding {
 static FaultAction action() { FaultAction<Throwable,Throwable> f=BindingOps::raise; return f; }
 static FaultPair pair() { FaultPair<Long,Integer,Throwable> f=BindingOps::combine; return f; }
 static FaultFactory factory() { FaultFactory<String,Throwable> f=BindingOps::erased; return f; }
 static FaultAction<Throwable,Throwable> bound(BindingOps owner) { FaultAction<Throwable,Throwable> f=owner::member; return f; }
 static String run(int mode,int x) {
  BindingOps.trace=0;
  try {
   switch(mode) {
    case 0: ((FaultAction)action()).act(null); break;
    case 1: ((FaultAction)action()).act(new IllegalStateException("i")); break;
    case 2: ((FaultAction)action()).act("bad"); break;
    case 3: return pair().act((long)x,x-3)+":"+BindingOps.trace;
    case 4: ((FaultPair)pair()).act(Integer.valueOf(x),Integer.valueOf(x)); break;
    case 5: ((FaultPair)pair()).act(Long.valueOf(x),Long.valueOf(x)); break;
    case 6: ((FaultPair)pair()).act(null,Integer.valueOf(x)); break;
    case 7: return ((FaultFactory)factory()).make()+":"+BindingOps.trace;
    case 8: return ((String)factory().make())+":"+BindingOps.trace;
    case 9: bound(null); break;
    case 10: bound(new BindingOps()).act(null); break;
    default: bound(new BindingOps()).act(new AssertionError("a"));
   }
   return "ok:"+BindingOps.trace;
  } catch(Throwable failure) { return failure.getClass().getName()+":"+BindingOps.trace; }
 }
 public static void main(String[] args) {
  for(int x=-4;x<=4;x++) for(int mode=0;mode<12;mode++) System.out.println(mode+":"+x+":"+run(mode,x));
 }
}`, Precision, Compatibility, "legacy")
}

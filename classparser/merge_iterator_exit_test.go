package javaclassparser

import "testing"

// Iteration can stop normally, by cancellation, or through either protected
// iterator operation. The final pending-count decrement belongs only to normal
// completion/error paths. Helpers stay original so callback order is observable.
func TestAdversarialMergeIteratorExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "MergeIteratorExit", `import java.util.*;
import java.util.concurrent.atomic.AtomicInteger;
class MergeEvents {
  static String trace="";
  static int cancelled;
  static boolean disposed() { trace+="D"; return --cancelled==0; }
  static void error(Throwable x) { trace+="E"+x.getMessage(); }
  static void done() { trace+="F"; }
  static Iterable<Integer> source(final int fail) {
    return new Iterable<Integer>() {public Iterator<Integer> iterator() {
      trace+="I"; if(fail==0)throw new IllegalStateException("I");
      return new Iterator<Integer>() {int n;
        public boolean hasNext() {trace+="H";if(fail==1 && n==1)throw new IllegalStateException("H");return n<3;}
        public Integer next() {trace+="N";if(fail==2 && n==1)throw new IllegalStateException("N");return n++;}
      };
    }};
  }
  static void subscribe(int n,AtomicInteger pending) {trace+="S"+n;pending.decrementAndGet();}
}
public class MergeIteratorExit {
  static void run(Iterable<Integer> sources) {
    Iterator<Integer> iterator;
    try {iterator=sources.iterator();}catch(Throwable ex){MergeEvents.error(ex);return;}
    AtomicInteger pending=new AtomicInteger(1);
    for(;;) {
      if(MergeEvents.disposed())return;
      boolean next;
      try {next=iterator.hasNext();}catch(Throwable ex){MergeEvents.error(ex);break;}
      if(!next)break;
      if(MergeEvents.disposed())return;
      Integer item;
      try {item=iterator.next();}catch(Throwable ex){MergeEvents.error(ex);break;}
      if(MergeEvents.disposed())return;
      pending.getAndIncrement();
      MergeEvents.subscribe(item,pending);
    }
    if(pending.decrementAndGet()==0)MergeEvents.done();
  }
  public static void main(String[] args) {
    for(int fail=-1;fail<3;fail++)for(int cancel=0;cancel<12;cancel++) {
      MergeEvents.trace="";MergeEvents.cancelled=cancel;
      run(MergeEvents.source(fail));System.out.print(MergeEvents.trace+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}

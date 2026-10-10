package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialBooleanQueueSlotRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BooleanQueueSlot", `import java.util.*;
public class BooleanQueueSlot {
  final StringBuilder trace=new StringBuilder();
  Queue<String> queue;
  int active;
  int work;
  boolean acquire;
  boolean demand;
  Queue<String> getQueue() {trace.append("q;"); if(queue==null)queue=new ArrayDeque<>();return queue;}
  void drain() {trace.append("d;");}
  void success(String value) {
    if(work==0 && acquire) {
      boolean done=--active==0;
      if(demand) {
        trace.append("v:").append(value).append(";");
        Queue<String> q=queue;
        if(done && (q==null || q.isEmpty())) {trace.append("end;");return;}
      } else {
        Queue<String> q=getQueue();
        synchronized(q) {q.offer(value);}
      }
      if(--work==0)return;
    } else {
      Queue<String> q=getQueue();
      synchronized(q) {q.offer(value);}
      --active;
      if(work++!=0)return;
    }
    drain();
  }
  public static void main(String[] args) {
    for(int pending=0;pending<3;pending++)for(int flags=0;flags<16;flags++) {
      BooleanQueueSlot s=new BooleanQueueSlot();s.active=pending;
      s.work=(flags&1);s.acquire=(flags&2)!=0;s.demand=(flags&4)!=0;
      if((flags&8)!=0) {s.queue=new ArrayDeque<>();s.queue.offer("old");}
      s.success("new");
      System.out.print(s.trace+":"+s.active+":"+s.work+":"+s.queue+"|");
    }
  }
}`, Precision, Compatibility, "legacy")
}

// The same numeric local may share a name prefix with an unrelated monitor.
// Retyping the former changes valid arithmetic even after IR has split the slots.
func TestAdversarialMonitorLocalNamePrefixes(t *testing.T) {
	for _, monitor := range []string{"var2_1", "var20", "var21_2"} {
		source := "class C { void f(Object " + monitor + ") {\nint var2 = 0;\nsynchronized(" + monitor + "){var2++;}\nSystem.out.print(var2);\n} }"
		result := fixHardjarShapes(source)
		if !strings.Contains(result, "int var2 = 0;") || strings.Contains(result, "Object var2 = null;") {
			t.Fatalf("monitor %s changed the independent integer declaration:\n%s", monitor, result)
		}
	}
}

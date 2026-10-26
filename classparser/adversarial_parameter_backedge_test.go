package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialParameterBackedgeRetainsOwnerAfterDeadClear(t *testing.T) {
	const fixture = `
class QueueNode<T> {
  final int id; int visits; Object state; QueueLink links;
  QueueNode(int id) { this.id=id; }
  void visit() { if (++visits != 1) throw new IllegalStateException("revisited owner"); }
  QueueLink take(QueueLink tail) {
    QueueLink head=links; links=null;
    if(head==null) return tail;
    QueueLink end=head; while(end.next!=null) end=end.next; end.next=tail; return head;
  }
}
class QueueLink { final Object task; QueueLink next; QueueLink(Object task,QueueLink next){this.task=task;this.next=next;} }
class Transfer<T> { final QueueNode<T> owner; final boolean accept; Transfer(QueueNode<T> owner,boolean accept){this.owner=owner;this.accept=accept;} }
public class ParameterQueue {
  public static StringBuilder events=new StringBuilder();
  public static void drain(QueueNode<?> current) {
    QueueLink pending=null;
    outer: while(true) {
      current.visit(); events.append('V').append(current.id).append(';');
      pending=current.take(pending);
      current=null;
      while(pending!=null) {
        QueueLink item=pending; pending=pending.next; Object task=item.task;
        if(task instanceof Transfer) {
          Transfer move=(Transfer)task; current=move.owner;
          if(current.state==move) {
            if(move.accept) { current.state=null; continue outer; }
          }
        } else {
          events.append('T').append(task).append(';');
        }
      }
      return;
    }
  }
}
class QueueDriver {
  public static void main(String[] args) {
    int count=0;
    for(int depth:new int[]{0,1,2,5}) for(int rejected=0;rejected<3;rejected++) {
      QueueNode<Integer>[] nodes=new QueueNode[depth+1];
      StringBuilder want=new StringBuilder();
      for(int i=0;i<nodes.length;i++) nodes[i]=new QueueNode<Integer>(i);
      for(int i=0;i<nodes.length;i++) {
        QueueLink list=new QueueLink("tail"+i,null);
        if(i<depth) { Transfer<Integer> move=new Transfer<Integer>(nodes[i+1],true);nodes[i+1].state=move;list=new QueueLink(move,list); }
        for(int j=0;j<rejected;j++) {
          QueueNode<Integer> spare=new QueueNode<Integer>(-j-1);
          Transfer<Integer> move=new Transfer<Integer>(spare,false);spare.state=move;list=new QueueLink(move,list);
        }
        nodes[i].links=list;
        want.append('V').append(i).append(';');
      }
      for(int i=depth;i>=0;i--) want.append("Ttail").append(i).append(';');
      ParameterQueue.events.setLength(0);ParameterQueue.drain(nodes[0]);
      if(!want.toString().equals(ParameterQueue.events.toString())) throw new AssertionError("events "+ParameterQueue.events+" wanted "+want);
      for(QueueNode<?> node:nodes) if(node.visits!=1||node.links!=null||node.state!=null) throw new AssertionError("node state");
      count++;
    }
    System.out.println(count+":parameter:backedge:dead-clear:effects");
  }
}`
	modern, _ := t04Tools(t)
	for _, layout := range []string{"wildcard", "raw", "wide", "instance", "renamed"} {
		t.Run(layout, func(t *testing.T) {
			source, owner := fixture, "ParameterQueue"
			switch layout {
			case "raw":
				source = strings.ReplaceAll(source, "drain(QueueNode<?> current)", "drain(QueueNode current)")
			case "wide":
				source = strings.ReplaceAll(source, "drain(QueueNode<?> current)", "drain(long salt,double scale,QueueNode<?> current)")
				source = strings.ReplaceAll(source, "drain(nodes[0])", "drain(7L,-0.0,nodes[0])")
			case "instance":
				source = strings.ReplaceAll(source, "static void drain", "void drain")
				source = strings.ReplaceAll(source, "ParameterQueue.drain(nodes[0])", "new ParameterQueue().drain(nodes[0])")
			case "renamed":
				source = strings.NewReplacer("ParameterQueue", "DispatchStage", "QueueNode", "Recipient", "QueueLink", "Envelope", "Transfer", "Handoff", "drain", "finish", "current", "recipient").Replace(source)
				owner = "DispatchStage"
			}
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					compiler := modern
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Fatal("native compiler required")
						}
					}
					compile := func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, source, owner, debug, false)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{owner}, "QueueDriver", "12:parameter:backedge:dead-clear:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
				})
			}
		})
	}
}

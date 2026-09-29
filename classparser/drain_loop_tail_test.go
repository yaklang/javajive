package javaclassparser

import "testing"

func TestAdversarialDrainLoopTailRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DrainLoopTail", `import java.util.concurrent.atomic.AtomicInteger;
public class DrainLoopTail extends AtomicInteger {
  final int[] items;int index,total;boolean active,cancelled;String trace="";
  DrainLoopTail(int[] items,boolean active,boolean cancelled) { this.items=items;this.active=active;this.cancelled=cancelled; }
  int next() {trace+="N";return index==items.length ? -1 : 100/items[index++];}
  void drain(boolean recover) {
    if(getAndIncrement()==0) {
    for(;;) {
      if(cancelled)return;
      if(!active) {
        try {
          int value=next();
          if(value==-1)return;
          if(value!=0) {
            try {
              if(value==100)continue;
              if(value<0)throw new IllegalArgumentException("negative");
              total+=value;active=true;
            } catch(IllegalArgumentException e) {trace+="E";return;}
          }
        } catch(ArithmeticException e) {trace+="C";if(!recover)return;continue;}
      }
      trace+="D";
      if(decrementAndGet()==0)break;
    }
    }
  }
  public static void main(String[] args) {
    for(int[] items:new int[][]{{},{1,2},{0,2},{-4},{200,3}})
      for(boolean active:new boolean[]{false,true})for(boolean cancelled:new boolean[]{false,true})for(boolean recover:new boolean[]{false,true}) {
        DrainLoopTail d=new DrainLoopTail(items,active,cancelled);d.drain(recover);
        System.out.print(d.total+":"+d.index+":"+d.get()+":"+d.trace+";");
      }
    DrainLoopTail busy=new DrainLoopTail(new int[]{2},false,false);busy.set(2);busy.drain(false);
    System.out.print("busy:"+busy.get()+":"+busy.trace);
  }
}`, Precision, Compatibility, "legacy")
}

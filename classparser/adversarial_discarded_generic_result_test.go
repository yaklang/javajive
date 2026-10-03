package javaclassparser

import "testing"

func TestAdversarialDiscardedGenericResultKeepsMaterializedSAMChecks(t *testing.T) {
	roundTripGenericFlow(t, "DiscardedResultDriver", `import java.util.*;import java.util.concurrent.*;import java.util.function.*;
class RemovalSpy implements BiConsumer<Object,Object>{
 int calls;Object key,value;boolean fail;final RuntimeException failure=new RuntimeException("shared");
 public void accept(Object key,Object value){calls++;this.key=key;this.value=value;if(fail)throw failure;}
}
public class DiscardedResultDriver<K,V>{
 final ConcurrentMap<K,V> data=new ConcurrentHashMap<>();final BiConsumer<K,V> writer;
 DiscardedResultDriver(BiConsumer writer){this.writer=writer;}
 V remove(Object key){Object[] seen=new Object[1];
  BiFunction<? super K,? super V,? extends V> action=(k,v)->{((BiConsumer)writer).accept(key,v);seen[0]=v;return null;};
  data.computeIfPresent((K)key,action);return (V)seen[0];
 }
 public static void main(String[]args){for(Object key:new Object[]{"text",Integer.valueOf(7),null})for(boolean fail:new boolean[]{false,true}){
  Object value=new Object();RemovalSpy spy=new RemovalSpy();spy.fail=fail;DiscardedResultDriver<String,Object> driver=new DiscardedResultDriver<>(spy);
  if(key!=null)((Map)driver.data).put(key,value);
  try{Object result=driver.remove(key);if(key==null||fail||result!=value||spy.calls!=1||spy.key!=key||spy.value!=value||!driver.data.isEmpty())throw new AssertionError("payload/effects");System.out.println("removed");}
  catch(NullPointerException failure){if(key!=null||spy.calls!=0)throw new AssertionError("null effect order",failure);System.out.println("null");}
  catch(RuntimeException failure){if(!fail||failure!=spy.failure||spy.calls!=1||spy.key!=key||spy.value!=value||driver.data.get(key)!=value)throw new AssertionError("throw identity/order",failure);System.out.println("shared");}
 }}
}`, Precision, Compatibility, "legacy")
}

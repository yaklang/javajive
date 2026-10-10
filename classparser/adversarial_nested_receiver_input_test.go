package javaclassparser

import "testing"

// The helper's erased entry must accept polluted keys without inventing a K
// check, while a nested Box<V> argument must retain its original check. The
// The runtime implementation's String overload must not steal dispatch from
// the bytecode-selected generic Map declaration.
func TestAdversarialNestedReceiverErasedInputsKeepChecksAndBinding(t *testing.T) {
	roundTripGenericFlow(t, "ErasedInputDriver", `import java.util.*;import java.util.function.*;
class InputBox<V>{V value;InputBox(V value){this.value=value;}}
class InputStore<K,V> extends HashMap<K,InputBox<V>>{
 int calls;Object key,box;final IllegalStateException failure=new IllegalStateException("shared");
 public boolean replace(K key,InputBox<V> before,InputBox<V> after){calls++;this.key=key;box=after;if(before!=after)throw failure;return true;}
 public boolean replace(String key,InputBox<V> before,InputBox<V> after){throw new AssertionError("overload");}
}
public class ErasedInputDriver<K,V>{
 final Map<K,InputBox<V>> cache;ErasedInputDriver(InputStore<K,V> cache){this.cache=cache;}
 BiConsumer<? super K,? super InputBox<V>> callback(Map<K,V> values){return (BiConsumer)((BiConsumer<Object,InputBox>)((key,box)->{V value=values.get(key);if(box!=null)box.value=value;((Map)cache).replace(key,box,box);}));}
 public static void main(String[]args){
  for(Object key:new Object[]{null,"text",Integer.valueOf(7)}){
   InputStore<String,Integer> store=new InputStore<>();ErasedInputDriver<String,Integer> driver=new ErasedInputDriver<>(store);InputBox<Integer> box=new InputBox<>(17);
   BiConsumer call=driver.callback(new HashMap<String,Integer>());call.accept(key,box);
   if(store.calls!=1||store.key!=key||store.box!=box)throw new AssertionError("erased identity/effects");
   try{call.accept(key,new Object());throw new AssertionError("missing original check");}catch(ClassCastException expected){if(store.calls!=1)throw new AssertionError("late check");}
   call.accept(key,null);if(store.calls!=2||store.key!=key||store.box!=null)throw new AssertionError("null payload");
   System.out.println(store.calls+":"+(key==null));
  }
 }
}`, Precision, Compatibility, "legacy")
}

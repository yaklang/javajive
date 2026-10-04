package javaclassparser

import "testing"

func TestAdversarialCustomSamInstantiationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CustomSamInstantiation", `import java.util.*;
public class CustomSamInstantiation {
 private static final Three<String,Object,Map<String,Object>> STORE=(key,value,map)->map.put(key,value);
 private static final Reverse<Map<String,Object>,Object,String> REVERSE=(key,value,map)->map.put(key,value);
 private static final Three<String,Object,Map<String,Object>> REFERENCE=CustomSamInstantiation::direct;
 private static final Returning<Map<String,Object>> NEWMAP=HashMap::new;
 private static final Pair<String> SAME=(first,second)->first.equals(second);
 static void direct(String key,Object value,Map<String,Object> map){map.put(key,value);}
 static void insert(String key,Object value,Map<String,Object> map){
  STORE.accept(key,value,map);REVERSE.accept(key,value,map);REFERENCE.accept(key,value,map);
  Map<String,Object> fresh=NEWMAP.get();fresh.put(key,value);map.putAll(fresh);SAME.same(key,key);
 }
 public static void main(String[] args){CustomSamProbe.main(args);}
}
interface Three<A,B,C> {void accept(A first,B second,C third);}
interface Reverse<A,B,C> {void accept(C first,B second,A third);}
interface Returning<T> {T get();}
interface Pair<T> {boolean same(T first,T second);}
class CustomSamProbe {
 public static void main(String[] args){
  for(String key:new String[]{null,"a","b"})for(Object value:new Object[]{null,Integer.valueOf(3),"x"})for(int kind=0;kind<3;kind++){
   Map<String,Object> map=kind==0?null:kind==1?new HashMap<String,Object>():new TreeMap<String,Object>();String outcome="ok";
   try{CustomSamInstantiation.insert(key,value,map);}catch(RuntimeException error){outcome=error.getClass().getName();}
   System.out.println(key+":"+value+":"+kind+":"+outcome+":"+map);
  }
 }
}
`, Precision, Compatibility, "legacy")
}

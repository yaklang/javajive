package javaclassparser

import "testing"

// A source view must preserve the original overload and erased payload. These
// cases distinguish a typed null from an Object overload, preserve a mutable
// Class chain, and prevent an added payload check on a generic Map key.
func TestAdversarialErasedMutationViewsRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_ARRAY_PARAM_REF_ARG_CAST_OFF", "JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF", "JDEC_GENERIC_PARAM_INFER_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "ErasedMutationViews", `import java.io.*;import java.util.*;
class ErasedMutationWitness {
 static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("same");
 static byte[] array(int mode){trace+="A";if(mode==2)throw failure;return mode==0?null:new byte[]{3,4};}
 static int argument(int mode){trace+="I";if(mode==3)throw failure;return mode;}

}
class ErasedSerializableBase implements Serializable{}class ErasedSerializableChild extends ErasedSerializableBase{}
public class ErasedMutationViews<E,V> {
 final Map<E,? super V> map;final V value;
 ErasedMutationViews(Map<E,? super V> map,V value){this.map=map;this.value=value;}
 static int take(byte[] data,int value){ErasedMutationWitness.trace+="B";return data==null?value:data.length+value;}
 static int take(Object data,int value){ErasedMutationWitness.trace+="O";return -999;}
 static int array(int mode){byte[] data=ErasedMutationWitness.array(mode);return take(data,ErasedMutationWitness.argument(mode));}
 static <T> Class<? super T> parent(Class<T> type){Class<? super T> result=type;while(Serializable.class.isAssignableFrom(result)){result=result.getSuperclass();if(result==null)throw new Error("hierarchy");}return result;}
 boolean addAll(Collection<? extends E> keys){int size=map.size();for(E key:keys)map.put(key,value);return map.size()!=size;}
 public static void main(String[] args){
  for(int mode=0;mode<4;mode++){ErasedMutationWitness.trace="";try{System.out.println(array(mode)+":"+ErasedMutationWitness.trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e==ErasedMutationWitness.failure)+":"+ErasedMutationWitness.trace);}}
  for(Class type:new Class[]{Object.class,String.class,ErasedSerializableChild.class,Serializable.class,null}){try{Class result=parent(type);System.out.println(result==Object.class?"object":result==type?"same":result.getName());}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());}}
  Object marker=new Object(),foreign=new Object();Map<Object,Object> backing=new LinkedHashMap<>();ErasedMutationViews<String,Object> consumer=new ErasedMutationViews<>((Map)backing,marker);
  for(Collection keys:new Collection[]{Collections.emptyList(),Arrays.asList("a",null,"a"),Arrays.asList(foreign,"a"),Arrays.asList(foreign)}){System.out.println(consumer.addAll(keys)+":"+backing.size()+":"+backing.containsKey(foreign)+":"+(backing.get("a")==marker)+":"+(backing.get(foreign)==marker));}
 }
}`, Precision, Compatibility, "legacy")
}

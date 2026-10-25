package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const memberEntrySignatureFixture = `
class EntryOwner<K extends Comparable<K>,V> {
 class Entry implements java.util.Map.Entry<K,V>, Comparable<Entry> {
  final K key;V value;
  Entry(java.util.Map.Entry<K,V> input){this(input.getKey(),input.getValue());}
  Entry(K k,V v){key=k;value=v;}
  public K getKey(){return key;}public V getValue(){return value;}
  public V setValue(V v){V old=value;value=v;return old;}
  public int compareTo(Entry other){return key.compareTo(other.key);}
 }
 Entry make(K key,V value){return new Entry(new java.util.AbstractMap.SimpleEntry<K,V>(key,value));}
}
class EntryProbe implements java.util.Map.Entry {
 final int mode;String trace="";final RuntimeException sentinel=new IllegalStateException("entry");
 EntryProbe(int m){mode=m;}public Object getKey(){trace+="K";if(mode==1)throw sentinel;return mode==2?new Object():Integer.valueOf(4);}
 public Object getValue(){trace+="V";if(mode==3)throw sentinel;return this;}public Object setValue(Object x){throw new AssertionError();}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 Class<?> c=EntryOwner.Entry.class;
 if(c.getTypeParameters().length!=0||c.getEnclosingClass()!=EntryOwner.class)throw new AssertionError("lexical owner");
 java.lang.reflect.Method compare=c.getDeclaredMethod("compareTo",c);
 java.lang.reflect.ParameterizedType parameter=(java.lang.reflect.ParameterizedType)compare.getGenericParameterTypes()[0];
 java.lang.reflect.ParameterizedType owner=(java.lang.reflect.ParameterizedType)parameter.getOwnerType();
 if(parameter.getRawType()!=c||owner.getRawType()!=EntryOwner.class||!java.util.Arrays.equals(owner.getActualTypeArguments(),EntryOwner.class.getTypeParameters()))throw new AssertionError("owner bindings");
 int rows=0;for(Integer k:new Integer[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})for(Integer q:new Integer[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE}){
  EntryOwner<Integer,Object> outer=new EntryOwner<Integer,Object>();Object a=new Object(),b=new Object();EntryOwner<Integer,Object>.Entry x=outer.make(k,a),y=outer.make(q,b);
  if(Integer.signum(x.compareTo(y))!=Integer.signum(Integer.compare(k,q))||x.getKey()!=k||x.getValue()!=a||x.setValue(b)!=a||x.getValue()!=b)throw new AssertionError("entry semantics");rows++;
 }
 for(int mode=0;mode<4;mode++){
  EntryProbe probe=new EntryProbe(mode);EntryOwner<Integer,Object> outer=new EntryOwner<Integer,Object>();Object actual;
  try{actual=outer.new Entry((java.util.Map.Entry<Integer,Object>)probe);}catch(Throwable failure){actual=failure;}
  if(mode==0){if(!(actual instanceof EntryOwner.Entry)||((EntryOwner.Entry)actual).getValue()!=probe)throw new AssertionError("result identity");}
  else if(mode==2){if(!(actual instanceof ClassCastException))throw new AssertionError("original checkcast");}
  else if(actual!=probe.sentinel)throw new AssertionError("exception identity");
  if(!probe.trace.equals(mode==1||mode==2?"K":"KV"))throw new AssertionError("evaluation order "+probe.trace);rows++;
 }
 for(java.lang.reflect.Constructor<?> ctor:c.getDeclaredConstructors()){
  java.lang.reflect.Type[] formal=ctor.getGenericParameterTypes();
  if(formal.length!=ctor.getParameterCount()-1)throw new AssertionError("hidden port leaked");
  if(formal.length==2&&!java.util.Arrays.equals(formal,EntryOwner.class.getTypeParameters()))throw new AssertionError("constructor binders");
 }
 System.out.println(rows+":member-entry:lexical:bridge:identity");}}
`

func TestAdversarialMemberEntryKeepsEnclosingGenericSignatures(t *testing.T) {
	for _, name := range []string{"EntryOwner", "DifferentOrderedOwner"} {
		for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", name, profile, debug), func(t *testing.T) {
					t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
					fixture := strings.ReplaceAll(memberEntrySignatureFixture, "EntryOwner", name)
					constructorSuffixFamilyWithOriginalCompilerRoundTrip(t, fixture, debug, []string{name, name + "$Entry"}, "16:member-entry:lexical:bridge:identity\n", 8, 8, ModernJavac, profile)
				})
			}
		}
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

const lexicalThisGenericConsumerFixture = `class RangeItem<T>{final T value;RangeItem(T value){this.value=value;}T key(){return value;}}class RangeEffects{static String trace="";}
class RangeOwner<K>{final RangeItem<K> item;RangeOwner(K value){item=new RangeItem<K>(value);}abstract class Gate{boolean accept(K value,boolean last){RangeEffects.trace+=last?"L":"F";return value==item.value;}boolean accept(Object value,int wrong){throw new AssertionError("overload");}}class View extends Gate{K select(boolean present,boolean last){RangeItem raw=item;Object key=present?raw.key():null;if(key!=null&&accept((K)key,last))return (K)key;throw new java.util.NoSuchElementException();}}}
class RangeDriver{public static void main(String[]args){Object token=new Object();RangeOwner<Object> owner=new RangeOwner<Object>(token);RangeOwner<Object>.View view=owner.new View();for(boolean present:new boolean[]{false,true})for(boolean last:new boolean[]{false,true}){RangeEffects.trace="";try{Object got=view.select(present,last);if(!present||got!=token||!RangeEffects.trace.equals(last?"L":"F"))throw new AssertionError("lexical result/identity/order");}catch(java.util.NoSuchElementException e){if(present||!RangeEffects.trace.equals(""))throw new AssertionError("lexical failure/order");}}System.out.println("4:lexical-this:inherited:generic:identity:overload:failure");}}`

func TestAdversarialLexicalThisInheritedGenericConsumer(t *testing.T) {
	testNativeIndependentFamilyFixture(t, lexicalThisGenericConsumerFixture, []string{"RangeOwner"}, "RangeDriver", "4:lexical-this:inherited:generic:identity:overload:failure\n", nativeLexicalExactSignatures)
}
func TestAdversarialLexicalThisDeepInheritedGenericConsumer(t *testing.T) {
	f := strings.Replace(lexicalThisGenericConsumerFixture, "class View extends Gate", "abstract class Layer extends Gate{}class View extends Layer", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"RangeOwner"}, "RangeDriver", "4:lexical-this:inherited:generic:identity:overload:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialLexicalThisGenericConsumerWinsBeforeBoxedOverload(t *testing.T) {
	f := strings.Replace(lexicalThisGenericConsumerFixture, "accept(Object value,int wrong)", "accept(Object value,Boolean wrong)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"RangeOwner"}, "RangeDriver", "4:lexical-this:inherited:generic:identity:overload:failure\n", nativeLexicalExactSignatures)
}

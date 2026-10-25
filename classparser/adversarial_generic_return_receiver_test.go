package javaclassparser

import (
	"strings"
	"testing"
)

const genericReturnReceiverFixture = `class PortBox<T>{T value;int calls;boolean fail;T take(Object key){calls++;if(fail)throw new NullPointerException();return value;}T remove(Object key){return take(key);}}
public class SuffixOwner {
 static <V> V read(PortBox<V> box,Object key){try{return box.take(key);}catch(ClassCastException|NullPointerException e){return null;}}
 static <V> V erase(PortBox<V> box,Object key){try{return box.remove(key);}catch(ClassCastException|NullPointerException e){return null;}}
}
class SuffixDriver {public static void main(String[]args){
 int rows=0;Object token=new Object();
 for(Object value:new Object[]{null,"text",token,Integer.valueOf(19)})for(boolean fail:new boolean[]{false,true})for(boolean erase:new boolean[]{false,true}){
  PortBox raw=new PortBox();raw.value=value;raw.fail=fail;Object result=erase?SuffixOwner.erase(raw,token):SuffixOwner.read(raw,token);
  if(result!=(fail?null:value)||raw.calls!=1)throw new AssertionError("erased result identity and one invocation");
  PortBox<String> typed=raw;Throwable caught=null;String text=null;try{text=erase?SuffixOwner.erase(typed,token):SuffixOwner.read(typed,token);}catch(Throwable e){caught=e;}
  boolean wrong=!fail&&value!=null&&!(value instanceof String);
  if(wrong?!(caught instanceof ClassCastException):caught!=null||text!=(fail?null:value))throw new AssertionError("caller checkcast must stay outside callee handler",caught);
  if(raw.calls!=2)throw new AssertionError("calls");rows++;
 }
 if(SuffixOwner.read(null,token)!=null||SuffixOwner.erase(null,token)!=null)throw new AssertionError("null receiver handler");
 System.out.println(rows+":generic:return:receiver:handler:identity");}}
`

func TestAdversarialGenericResultReceiverKeepsCallerCastOutsideHandler(t *testing.T) {
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, receiver := range []string{"declared", "public interface"} {
				t.Run(string(profile)+"/"+debug+"/"+receiver, func(t *testing.T) {
					fixture := genericReturnReceiverFixture
					if receiver == "public interface" {
						fixture = strings.NewReplacer("class PortBox<T>{", "class PortBox<T> extends java.util.HashMap<Object,T>{", "T take(Object", "public T get(Object", "T remove(Object", "public T remove(Object", "take(key)", "get(key)", "PortBox<V> box", "java.util.Map<?,V> box").Replace(fixture)
					}
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "16:generic:return:receiver:handler:identity\n", 8, 8, profile)
				})
			}
		}
	}
}

func TestAdversarialGenericFunctionalResultKeepsErasedReturnView(t *testing.T) {
	const fixture = `class PortFunction<T> implements java.util.function.Function<SuffixOwner,T>{T value;Object seen;int calls;boolean fail;public T apply(SuffixOwner input){calls++;seen=input;if(fail)throw new NullPointerException();return value;}}
public class SuffixOwner {
 static <V> V project(java.util.function.Function<SuffixOwner,V> function,SuffixOwner input){try{return function.apply(input);}catch(ClassCastException|NullPointerException e){return null;}}
 <V> V invoke(java.util.function.Function<SuffixOwner,V> function){try{return function.apply(this);}catch(ClassCastException|NullPointerException e){return null;}}
}
class SuffixDriver {public static void main(String[]args){
 int rows=0;Object token=new Object();SuffixOwner owner=new SuffixOwner();
 for(Object value:new Object[]{null,"text",token,Integer.valueOf(19)})for(boolean fail:new boolean[]{false,true})for(boolean instance:new boolean[]{false,true}){
  PortFunction raw=new PortFunction();raw.value=value;raw.fail=fail;Object result=instance?owner.invoke(raw):SuffixOwner.project(raw,owner);
  if(result!=(fail?null:value)||raw.calls!=1||raw.seen!=owner)throw new AssertionError("erased result/input identity");
  PortFunction<String> typed=raw;Throwable caught=null;String text=null;try{text=instance?owner.invoke(typed):SuffixOwner.project(typed,owner);}catch(Throwable e){caught=e;}
  boolean wrong=!fail&&value!=null&&!(value instanceof String);
  if(wrong?!(caught instanceof ClassCastException):caught!=null||text!=(fail?null:value))throw new AssertionError("caller checkcast moved into callee handler",caught);
  if(raw.calls!=2)throw new AssertionError("duplicate call");rows++;
 }
 if(SuffixOwner.project(null,owner)!=null||owner.invoke(null)!=null)throw new AssertionError("null receiver handler");
 System.out.println(rows+":generic:functional:return:identity:handler");}}
`
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "16:generic:functional:return:identity:handler\n", 8, 8, profile)
			})
		}
	}
}

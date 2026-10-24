package javaclassparser

import (
	"strings"
	"testing"
)

// A generic enclosing constructor can owe a Signature projection even when
// Object.<init> needs no executable capture reordering. Later class recovery
// must retain the actual declaration that authorized that projection.
func TestAdversarialConstructorBindingPublicationKeepsCompletedGenericAssignments(t *testing.T) {
	for _, shape := range []string{"ordinary", "wide", "renamed"} {
		for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(shape+"/"+string(profile)+"/"+debug, func(t *testing.T) {
					source := constructorBindingPublicationFixture
					member := "SuffixOwner$Member"
					switch shape {
					case "wide":
						source = strings.Replace(source, "Member(T value,", "Member(long left,T value,", 1)
						source = strings.Replace(source, "callbacks){this.value", "callbacks){if(left!=Long.MIN_VALUE)throw new AssertionError(\"wide port\");this.value", 1)
						source = strings.Replace(source, "new Member<String>(value,", "new Member<String>(Long.MIN_VALUE,value,", 1)
					case "renamed":
						source = strings.ReplaceAll(source, "Member", "Container")
						member = "SuffixOwner$Container"
					}
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, []string{member}, "12:binding:publication:identity:exceptions\n", 8, 8, profile)
				})
			}
		}
	}
}

const constructorBindingPublicationFixture = `
class SuffixOwner {
 final Object token;SuffixOwner(Object token){this.token=token;}
 class Member<T> {
  final java.util.List<Object> all=new java.util.ArrayList<Object>();
  final java.util.Map<Class<?>,java.util.function.BiConsumer<Object,T>> callbacks;
  final T value;
  Member(T value,java.util.Map<Class<?>,java.util.function.BiConsumer<Object,T>> callbacks){this.value=value;this.callbacks=callbacks;}
  Object owner(){return SuffixOwner.this.token;}
 }
 Member<String> make(String value,java.util.Map<Class<?>,java.util.function.BiConsumer<Object,String>> callbacks){return new Member<String>(value,callbacks);}
}
class SuffixDriver {
 public static void main(String[]args){int rows=0;
  java.lang.reflect.Constructor<?> ctor=SuffixOwner.Member.class.getDeclaredConstructors()[0];
  if(ctor.getGenericParameterTypes().length+1!=ctor.getParameterTypes().length)throw new AssertionError("implicit generic port");
  for(Object token:new Object[]{null,"outer",new Object()})for(String value:new String[]{null,"value"})for(boolean fail:new boolean[]{false,true}){
   SuffixOwner owner=new SuffixOwner(token);Object item=new Object();Object[] seen=new Object[2];int[] count={0};
   RuntimeException expected=new IllegalArgumentException("exact identity");
   java.util.Map<Class<?>,java.util.function.BiConsumer<Object,String>> callbacks=new java.util.HashMap<Class<?>,java.util.function.BiConsumer<Object,String>>();
   callbacks.put(String.class,(a,b)->{seen[0]=a;seen[1]=b;count[0]++;if(fail)throw expected;});
   SuffixOwner.Member<String> member=owner.make(value,callbacks);
   if(member.owner()!=token||member.value!=value||member.callbacks!=callbacks||!member.all.isEmpty())throw new AssertionError("constructor identity");
   Throwable actual=null;try{member.all.add(item);member.callbacks.get(String.class).accept(item,member.value);}catch(Throwable t){actual=t;}
   if(actual!=(fail?expected:null)||count[0]!=1||seen[0]!=item||seen[1]!=value||member.all.size()!=1||member.all.get(0)!=item)throw new AssertionError("effects or failure",actual);
   rows++;
  }
  System.out.println(rows+":binding:publication:identity:exceptions");
 }
}
`

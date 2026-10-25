package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialHiddenOwnerRawMemberSignatureProjection(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []struct {
				name    string
				target  int
				profile SourceCompilerProfile
			}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
				name, source := "Member", hiddenOwnerRawMemberFixture
				if renamed {
					name = "View"
					source = strings.NewReplacer("Member", "View", "Leaf", "Node").Replace(source)
				}
				t.Run(name+"/"+debug+"/"+compiler.name, func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, []string{"SuffixOwner$" + name}, "12:raw:member:owner:signature:identity\n", 8, compiler.target, compiler.profile)
				})
			}
		}
	}
}

const hiddenOwnerRawMemberFixture = `class SuffixOwner<K extends Number> {
 class Leaf { final K payload; Leaf(K payload){this.payload=payload;} }
 class Member<T> {
  Leaf value;
	  java.util.List<Leaf> list;
  Leaf read(){return value;}
	  java.util.List<Leaf> values(){return list;}
  T own(T token){return token;}
 }
	 Member<Object> make(K payload,boolean absent){Member<Object> member=new Member<Object>();member.value=absent?null:new Leaf(payload);member.list=java.util.Collections.singletonList(member.value);return member;}
}
class SuffixDriver {
 static void owner(java.lang.reflect.Type type){
  if(!(type instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("member owner missing");
  java.lang.reflect.ParameterizedType member=(java.lang.reflect.ParameterizedType)type;
  if(member.getRawType()!=SuffixOwner.Leaf.class||member.getActualTypeArguments().length!=0||!(member.getOwnerType() instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("member identity");
  java.lang.reflect.ParameterizedType outer=(java.lang.reflect.ParameterizedType)member.getOwnerType();
  if(outer.getRawType()!=SuffixOwner.class||outer.getActualTypeArguments().length!=1||!(outer.getActualTypeArguments()[0] instanceof java.lang.reflect.TypeVariable))throw new AssertionError("outer identity");
  java.lang.reflect.TypeVariable<?> binder=(java.lang.reflect.TypeVariable<?>)outer.getActualTypeArguments()[0];
  if(binder.getGenericDeclaration()!=SuffixOwner.class||!binder.getName().equals("K"))throw new AssertionError("outer binder");
 }
 public static void main(String[]args)throws Exception{
  owner(SuffixOwner.Member.class.getDeclaredField("value").getGenericType());
  owner(SuffixOwner.Member.class.getDeclaredMethod("read").getGenericReturnType());
	  owner(((java.lang.reflect.ParameterizedType)SuffixOwner.Member.class.getDeclaredField("list").getGenericType()).getActualTypeArguments()[0]);
	  owner(((java.lang.reflect.ParameterizedType)SuffixOwner.Member.class.getDeclaredMethod("values").getGenericReturnType()).getActualTypeArguments()[0]);
  java.lang.reflect.TypeVariable<?> own=(java.lang.reflect.TypeVariable<?>)SuffixOwner.Member.class.getDeclaredMethod("own",Object.class).getGenericReturnType();
  if(own.getGenericDeclaration()!=SuffixOwner.Member.class||!own.getName().equals("T"))throw new AssertionError("own binder");
  int rows=0;for(Number payload:new Number[]{null,Integer.MIN_VALUE,Long.MAX_VALUE})for(boolean absent:new boolean[]{false,true})for(Object token:new Object[]{null,new Object()}){
   SuffixOwner<Number> outer=new SuffixOwner<Number>();SuffixOwner<Number>.Member<Object> member=outer.make(payload,absent);
	   if(member.read()!=member.value||member.own(token)!=token||member.values()!=member.list||member.values().size()!=1||member.values().get(0)!=member.value||absent&&member.read()!=null||!absent&&member.read().payload!=payload)throw new AssertionError("value identity");rows++;
  }System.out.println(rows+":raw:member:owner:signature:identity");
 }
}`

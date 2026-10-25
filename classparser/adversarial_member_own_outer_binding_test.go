package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// A physical member retains its own arity, while its outer formal is absent
// from Java source scope. Erasure must not silently change reflection binding.
func ownAndOuterBindingFixture() string {
	return `class SuffixOwner<T extends Number> {
 class Member<U extends CharSequence> {
  T saved;U selected;Member(){}
  T outer(T value){saved=value;return value;}
  U own(U value){selected=value;return value;}
  <T extends CharSequence>T shadow(T value){return value;}
 }
 Member<CharSequence> make(){return new Member<CharSequence>();}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 Class<?> cls=SuffixOwner.Member.class;
 java.lang.reflect.Type outer=SuffixOwner.class.getTypeParameters()[0], own=cls.getTypeParameters()[0];
 java.lang.reflect.Method fromOuter=cls.getDeclaredMethod("outer",Number.class),fromOwn=cls.getDeclaredMethod("own",CharSequence.class),shadow=cls.getDeclaredMethod("shadow",CharSequence.class);
 if(cls.getTypeParameters().length!=1||!fromOuter.getGenericReturnType().equals(outer)||!fromOuter.getGenericParameterTypes()[0].equals(outer)||!fromOwn.getGenericReturnType().equals(own)||!cls.getDeclaredField("saved").getGenericType().equals(outer)||!cls.getDeclaredField("selected").getGenericType().equals(own))throw new AssertionError("outer/member declaration identity");
 if(!shadow.getGenericReturnType().equals(shadow.getTypeParameters()[0])||shadow.getTypeParameters()[0].equals(outer))throw new AssertionError("method shadow identity");
 int rows=0;for(Number n:new Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")})for(CharSequence s:new CharSequence[]{null,"literal",new StringBuilder("mutable")}){
  SuffixOwner<Number> parent=new SuffixOwner<Number>();SuffixOwner<Number>.Member<CharSequence> member=parent.make();
  if(member.outer(n)!=n||member.saved!=n||member.own(s)!=s||member.selected!=s||member.shadow(s)!=s||fromOuter.invoke(member,n)!=n||fromOwn.invoke(member,s)!=s)throw new AssertionError("erased ABI/result/field identity");rows++;
 }System.out.println(rows+":own:outer:shadow:fields:identity");}}
`
}

func TestAdversarialOwnAndEnclosingMemberFormalsRemainDistinct(t *testing.T) {
	fixture := ownAndOuterBindingFixture()
	for _, root := range []string{"SuffixOwner", "DualScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", root, debug, compiler), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Member"}, "12:own:outer:shadow:fields:identity\n", 8, 8, compiler)
				})
			}
		}
	}
}

func TestAdversarialOwnAndEnclosingBindingsPreserveNestedTypeStructure(t *testing.T) {
	fixture := strings.Replace(ownAndOuterBindingFixture(), "T saved;U selected;Member(){}", `T saved;U selected;Member(){}
  T[] array(T[] value){return value;}
  java.util.List<java.util.List<T>> nested(java.util.List<java.util.List<T>> value){return value;}
  java.util.List<? extends T> upper(java.util.List<? extends T> value){return value;}
  java.util.List<? super U> lower(java.util.List<? super U> value){return value;}
  java.util.List<T[]> arrayArgument(java.util.List<T[]> value){return value;}
  <V extends T> V dependent(V value){return value;}`, 1)
	fixture = strings.Replace(fixture, "int rows=0;", `
 java.lang.reflect.Type arrayType=cls.getDeclaredMethod("array",Number[].class).getGenericReturnType();
 if(!(arrayType instanceof java.lang.reflect.GenericArrayType)||!((java.lang.reflect.GenericArrayType)arrayType).getGenericComponentType().equals(outer))throw new AssertionError("generic array binder");
 java.lang.reflect.ParameterizedType nested=(java.lang.reflect.ParameterizedType)cls.getDeclaredMethod("nested",java.util.List.class).getGenericReturnType();
 java.lang.reflect.ParameterizedType inside=(java.lang.reflect.ParameterizedType)nested.getActualTypeArguments()[0];
 if(!inside.getActualTypeArguments()[0].equals(outer))throw new AssertionError("nested binder");
 java.lang.reflect.ParameterizedType up=(java.lang.reflect.ParameterizedType)cls.getDeclaredMethod("upper",java.util.List.class).getGenericReturnType();
 if(!((java.lang.reflect.WildcardType)up.getActualTypeArguments()[0]).getUpperBounds()[0].equals(outer))throw new AssertionError("upper variance");
 java.lang.reflect.ParameterizedType low=(java.lang.reflect.ParameterizedType)cls.getDeclaredMethod("lower",java.util.List.class).getGenericReturnType();
 if(!((java.lang.reflect.WildcardType)low.getActualTypeArguments()[0]).getLowerBounds()[0].equals(own))throw new AssertionError("lower variance");
 java.lang.reflect.ParameterizedType aa=(java.lang.reflect.ParameterizedType)cls.getDeclaredMethod("arrayArgument",java.util.List.class).getGenericReturnType();
 if(!((java.lang.reflect.GenericArrayType)aa.getActualTypeArguments()[0]).getGenericComponentType().equals(outer))throw new AssertionError("array type argument");
 java.lang.reflect.Method dependent=cls.getDeclaredMethod("dependent",Number.class);
 if(!dependent.getTypeParameters()[0].getBounds()[0].equals(outer)||!dependent.getGenericReturnType().equals(dependent.getTypeParameters()[0]))throw new AssertionError("dependent bound identity");
 int rows=0;`, 1)
	fixture = strings.Replace(fixture, "rows++;", `
 Number[] array=new Number[]{n};java.util.List<Number> list=java.util.Arrays.asList(array);
 java.util.List<java.util.List<Number>> nestedList=java.util.Collections.singletonList(list);
 java.util.List<Number[]> arrayList=java.util.Collections.singletonList(array);
 java.util.List<Object> lowList=new java.util.ArrayList<Object>();lowList.add(s);
 if(member.array(array)!=array||member.nested(nestedList)!=nestedList||member.upper(list)!=list||member.lower(lowList)!=lowList||member.arrayArgument(arrayList)!=arrayList||member.dependent(n)!=n)throw new AssertionError("projected shape identity");rows++;`, 1)
	for _, root := range []string{"SuffixOwner", "NestedBinding"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", root, debug, compiler), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Member"}, "12:own:outer:shadow:fields:identity\n", 8, 8, compiler)
				})
			}
		}
	}
}

func TestAdversarialHiddenOuterBinderUsedOnlyInReturnOrMethodBound(t *testing.T) {
	fixture := `class SuffixOwner<T extends Number> {
 class Member<U> {
  Member(){} T result(){return null;}
  <V extends T> V bounded(V value){return value;}
  <V extends CharSequence> V independent(V value){return value;}
 }
 Member<Object> make(){return new Member<Object>();}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 Class<?> cls=SuffixOwner.Member.class;java.lang.reflect.Type outer=SuffixOwner.class.getTypeParameters()[0];
 java.lang.reflect.Method result=cls.getDeclaredMethod("result"),bounded=cls.getDeclaredMethod("bounded",Number.class),independent=cls.getDeclaredMethod("independent",CharSequence.class);
 if(cls.getTypeParameters().length!=1||!result.getGenericReturnType().equals(outer)||!bounded.getTypeParameters()[0].getBounds()[0].equals(outer)||!independent.getTypeParameters()[0].getBounds()[0].equals(CharSequence.class))throw new AssertionError("return/bound-only lexical binding");
 SuffixOwner<Number>.Member<Object> member=new SuffixOwner<Number>().make();
 for(Number n:new Number[]{null,Integer.valueOf(-7),Long.valueOf(Long.MAX_VALUE)})if(member.result()!=null||member.bounded(n)!=n||result.invoke(member)!=null||bounded.invoke(member,n)!=n)throw new AssertionError("return/bound-only value identity");
 StringBuilder s=new StringBuilder("mutable");if(member.independent(s)!=s)throw new AssertionError("independent method binder");
 System.out.println("3:return:bound:independent:identity");}}
`
	for _, debug := range []string{"none", "source,lines,vars"} {
		for _, compiler := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
			t.Run(fmt.Sprintf("%s/%s", debug, compiler), func(t *testing.T) {
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "3:return:bound:independent:identity\n", 8, 8, compiler)
			})
		}
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func ownDependentBoundFixture() string {
	return `class SuffixOwner<T extends Number> {
 class Member<U extends T> {
  U stored; Member(U value){stored=value;}
  U run(U value){U previous=stored;stored=value;return previous;}
  <V extends U> V nested(V value){return value;}
 }
 Member<T> make(T value){return new Member<T>(value);}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 Class<?> cls=SuffixOwner.Member.class;java.lang.reflect.TypeVariable<?> own=cls.getTypeParameters()[0];
 java.lang.reflect.Type outer=SuffixOwner.class.getTypeParameters()[0];
 java.lang.reflect.Method run=cls.getDeclaredMethod("run",Number.class),nested=cls.getDeclaredMethod("nested",Number.class);
 java.lang.reflect.Constructor<?> ctor=cls.getDeclaredConstructor(SuffixOwner.class,Number.class);
 if(cls.getTypeParameters().length!=1||!own.getBounds()[0].equals(outer)||!run.getGenericReturnType().equals(own)||!run.getGenericParameterTypes()[0].equals(own)||!nested.getTypeParameters()[0].getBounds()[0].equals(own)||!cls.getDeclaredField("stored").getGenericType().equals(own)||ctor.getGenericParameterTypes().length!=1||!ctor.getGenericParameterTypes()[0].equals(own))throw new AssertionError("class-bound/constructor/method binder identity");
 int rows=0;for(Number first:new Number[]{null,Integer.valueOf(-3),Long.valueOf(Long.MIN_VALUE)})for(Number next:new Number[]{null,Long.valueOf(Long.MAX_VALUE),new java.math.BigInteger("9223372036854775808")}){
  SuffixOwner<Number>.Member<Number> member=new SuffixOwner<Number>().make(first);
  if(member.stored!=first||member.run(next)!=first||member.stored!=next||member.nested(first)!=first||run.invoke(member,first)!=next||member.stored!=first)throw new AssertionError("dependent bound behavior");rows++;
 }System.out.println(rows+":dependent:class:method:constructor:identity");}}
`
}

func TestAdversarialMemberOwnBoundRetainsOriginalOuterDeclaration(t *testing.T) {
	for _, root := range []string{"SuffixOwner", "RenamedDependentScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []struct {
				target   int
				compiler SourceCompilerProfile
			}{{8, ModernJavac}, {21, ModernJavac}, {8, NativeJavac8}} {
				t.Run(fmt.Sprintf("%s/%s/%s/%d", root, debug, profile.compiler, profile.target), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(ownDependentBoundFixture(), "SuffixOwner", root), debug, []string{root + "$Member"}, "9:dependent:class:method:constructor:identity\n", 8, profile.target, profile.compiler)
				})
			}
		}
	}
}

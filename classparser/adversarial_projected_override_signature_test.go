package javaclassparser

import (
	"strings"
	"testing"
)

const projectedOverrideSignatureFixture = `class SuffixOwner<K,V>{
 static class Pair<A,B>{final A key;final B value;Pair(A key,B value){this.key=key;this.value=value;}}
 abstract class Task<T>{abstract T apply(Pair<K,V> input);}
 class Sub extends Task<V>{V apply(Pair<K,V> input){return input.value;}}
 V execute(Pair<K,V> input){return new Sub().apply(input);}
}
class SuffixDriver{public static void main(String[]args)throws Exception{
 java.lang.reflect.Method apply=SuffixOwner.Sub.class.getDeclaredMethod("apply",SuffixOwner.Pair.class);
 java.lang.reflect.ParameterizedType pair=(java.lang.reflect.ParameterizedType)apply.getGenericParameterTypes()[0];
 java.lang.reflect.TypeVariable<?>[] owner=SuffixOwner.class.getTypeParameters();
 if(pair.getRawType()!=SuffixOwner.Pair.class||!pair.getActualTypeArguments()[0].equals(owner[0])||!pair.getActualTypeArguments()[1].equals(owner[1])||!apply.getGenericReturnType().equals(owner[1]))throw new AssertionError("override original bindings");
 int rows=0;for(Integer key:new Integer[]{null,Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(String value:new String[]{null,"","字𝄞"}){
  SuffixOwner<Integer,String> outer=new SuffixOwner<Integer,String>();SuffixOwner.Pair<Integer,String> input=new SuffixOwner.Pair<Integer,String>(key,value);
  SuffixOwner<Integer,String>.Task<String> task=outer.new Sub();
  if(outer.execute(input)!=value||task.apply(input)!=value||input.key!=key)throw new AssertionError("virtual override identity");rows++;
 }System.out.println(rows+":projected:override:identity:binding");
}}
`

func TestAdversarialProjectedOverrideRetainsOriginalParameterBindings(t *testing.T) {
	for label, owner := range map[string]string{"original": "SuffixOwner", "renamed": "RenamedProjection", "supplementary_unicode": "𐐀Owner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			name := owner
			source := strings.ReplaceAll(projectedOverrideSignatureFixture, "SuffixOwner", name)
			// Keep the temporary classpath ASCII: some JVM URL loaders reject
			// supplementary characters in paths before loading even original code.
			// The compiled class names still exercise supplementary MUTF-8.
			t.Run(label+"/"+debug, func(t *testing.T) {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{name, name + "$Pair", name + "$Task", name + "$Sub"}, "12:projected:override:identity:binding\n")
			})
		}
	}
}

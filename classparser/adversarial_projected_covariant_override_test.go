package javaclassparser

import (
	"strings"
	"testing"
)

const projectedCovariantOverrideFixture = `class SuffixOwner<K,V>{
 static class Pair<A,B>{final A key;final B value;Pair(A k,B v){key=k;value=v;}}
 abstract class Task<T>{abstract T apply(Pair<K,V> input);}
 class Sub extends Task<Boolean>{Boolean apply(Pair<K,V> input){return input.value!=null;}}
 Task<Boolean> task(){return new Sub();}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 SuffixOwner<Integer,String> outer=new SuffixOwner<Integer,String>();
 java.lang.reflect.Method apply=outer.task().getClass().getDeclaredMethod("apply",SuffixOwner.Pair.class);
 java.lang.reflect.ParameterizedType pair=(java.lang.reflect.ParameterizedType)apply.getGenericParameterTypes()[0];
 java.lang.reflect.TypeVariable<?>[] owner=SuffixOwner.class.getTypeParameters();
 if(pair.getRawType()!=SuffixOwner.Pair.class||!pair.getActualTypeArguments()[0].equals(owner[0])||!pair.getActualTypeArguments()[1].equals(owner[1])||apply.getGenericReturnType()!=Boolean.class)throw new AssertionError("covariant original binding");
 int rows=0;for(Integer key:new Integer[]{null,Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(String value:new String[]{null,"","字𝄞"}){
  SuffixOwner.Pair<Integer,String> pairValue=new SuffixOwner.Pair<Integer,String>(key,value);SuffixOwner<Integer,String>.Task<Boolean> task=outer.task();
  Boolean typed=task.apply(pairValue); Object erased=((SuffixOwner.Task)task).apply(pairValue);
  if(typed.booleanValue()!=(value!=null)||!typed.equals(erased)||pairValue.key!=key||pairValue.value!=value)throw new AssertionError("covariant bridge dispatch");rows++;
 }System.out.println(rows+":covariant:virtual:binding");
}}
`

func TestAdversarialProjectedCovariantOverrideKeepsParentProtocol(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		source, suffix, label := projectedCovariantOverrideFixture, "$Sub", "named"
		if anonymous {
			source = strings.Replace(source, "class Sub extends Task<Boolean>{Boolean apply(Pair<K,V> input){return input.value!=null;}}\n Task<Boolean> task(){return new Sub();}", "Task<Boolean> task(){return new Task<Boolean>(){Boolean apply(Pair<K,V> input){return input.value!=null;}};}", 1)
			suffix, label = "$1", "anonymous"
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(label+"/"+debug, func(t *testing.T) {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
				t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
				constructorSuffixFamilyRoundTrip(t, source, debug, []string{"SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task", "SuffixOwner" + suffix}, "12:covariant:virtual:binding\n")
			})
		}
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const visibilityVarargsBridgeFixture = `abstract class HiddenBuilder<T extends HiddenBuilder<T>> {
 long total;int calls;
 protected abstract T self();
 public T set(long value){calls++;total=value;return self();}
 public T append(int... values){calls++;for(int n:values)total+=n;return self();}
 public long value(){return total;}
}
public class SuffixOwner extends HiddenBuilder<SuffixOwner> {protected SuffixOwner self(){return this;}}
class SuffixDriver {public static void main(String[]args)throws Exception{
 for(java.lang.reflect.Method m:SuffixOwner.class.getDeclaredMethods())if(m.getName().equals("set")||m.getName().equals("append"))throw new AssertionError("invented inherited declaration");
 if(SuffixOwner.class.getMethod("append",int[].class).getDeclaringClass()!=HiddenBuilder.class)throw new AssertionError("changed declaring owner");
 Object[] cases=new Object[]{null,new int[0],new int[]{1,-1},new int[]{Integer.MAX_VALUE,Integer.MIN_VALUE,7}};int rows=0;
 for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(Object entry:cases){
  SuffixOwner object=new SuffixOwner();if(object.set(seed)!=object)throw new AssertionError("fluent identity");
  int[] values=(int[])entry;try{if(object.append(values)!=object)throw new AssertionError("array identity");if(values==null)throw new AssertionError("null accepted");}catch(NullPointerException e){if(values!=null)throw e;}
  java.math.BigInteger sum=java.math.BigInteger.valueOf(seed);if(values!=null)for(int value:values)sum=sum.add(java.math.BigInteger.valueOf(value));
  if(object.value()!=sum.longValue()||object.calls!=2)throw new AssertionError("overflow, inherited state, exception effect order");rows++;
 }System.out.println(rows+":inherited:varargs:fluent:visibility:effects");}}
`

func TestAdversarialVisibilityBridgeComposesWithInheritedVarargs(t *testing.T) {
	for _, release := range []int{5, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("original%d/%s", release, debug), func(t *testing.T) {
				for _, rename := range []bool{false, true} {
					t.Run(fmt.Sprint(rename), func(t *testing.T) {
						source := visibilityVarargsBridgeFixture
						if rename {
							source = strings.NewReplacer("HiddenBuilder", "Ancestor", "append", "collect", "set", "assign").Replace(source)
						}
						constructorSuffixFamilyAtTargetRoundTrip(t, source, debug, []string{"SuffixOwner"}, "16:inherited:varargs:fluent:visibility:effects\n", release, 8)
					})
				}
			})
		}
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

const memberConstructorChainScopeFixture = `class SuffixOwner<T> {
 final T token;SuffixOwner(T token){this.token=token;}
 class Member {
  final java.util.List<T> values;final long seed;
  Member(long seed){this(null,seed);}
  Member(java.util.List<T> values,long seed){this.values=values;this.seed=seed;}
  Object owner(){return SuffixOwner.this.token;}
  java.util.List<T> values(){return values;}
  long fold(long n){return n*seed+(values==null?0:values.size());}
 }
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 int rows=0;for(Object token:new Object[]{null,new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  SuffixOwner<Object> outer=new SuffixOwner<Object>(token);
  SuffixOwner<Object>.Member a=outer.new Member(seed);java.util.List<Object> values=new java.util.ArrayList<Object>();values.add(token);
  SuffixOwner<Object>.Member b=outer.new Member(values,seed);
  if(a.owner()!=token||b.owner()!=token||a.values()!=null||b.values()!=values)throw new AssertionError("capture/delegation identity");
  for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(seed)).longValue();
   if(a.fold(n)!=expected||b.fold(n)!=expected+1)throw new AssertionError("wide argument and suffix");rows++;
  }
 }
 Class<?> member=Class.forName("SuffixOwner$Member");if(member.getTypeParameters().length!=0)throw new AssertionError("synthetic source binder escaped");
 boolean generic=false;for(java.lang.reflect.Constructor<?> c:member.getDeclaredConstructors())if(c.getParameterTypes().length==3){generic=true;java.lang.reflect.Type[] parameters=c.getGenericParameterTypes();if(parameters.length!=2||!(parameters[0]instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("original constructor Signature");}
 if(!generic)throw new AssertionError("constructor inventory");
 System.out.println(rows+":constructor:chain:scope:identity:overflow");
}}
`

func TestAdversarialMemberConstructorChainPreservesCaptureAndGenericScope(t *testing.T) {
	for _, shape := range []string{"direct delegation", "two edges", "generic forwarding", "renamed"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(shape+"/"+debug, func(t *testing.T) {
				fixture, member := memberConstructorChainScopeFixture, "SuffixOwner$Member"
				if shape == "two edges" {
					fixture = strings.Replace(fixture, "Member(long seed){this(null,seed);}", "Member(long seed){this(seed,0);}\n Member(long seed,int ignored){this(null,seed);}", 1)
					fixture = strings.Replace(fixture, "if(c.getParameterTypes().length==3)", "if(c.getParameterTypes().length==3&&c.getParameterTypes()[1]==java.util.List.class)", 1)
				}
				if shape == "generic forwarding" {
					fixture = strings.Replace(fixture, "Member(java.util.List<T> values,long seed){this.values=values;this.seed=seed;}", "Member(java.util.List<T> values,long seed){this(values,seed,0); } Member(java.util.List<T> values,long seed,int marker){this.values=values;this.seed=seed;}", 1)
				}
				if shape == "renamed" {
					fixture = strings.NewReplacer("Member", "Payload", "<T>", "<Element>", " T ", " Element ", "(T ", "(Element ").Replace(fixture)
					member = "SuffixOwner$Payload"
				}
				constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{member}, "50:constructor:chain:scope:identity:overflow\n")
			})
		}
	}
}

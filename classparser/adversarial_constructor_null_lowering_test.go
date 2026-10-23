package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const constructorNullLoweringFixture = `class SuffixWitness {static SuffixBase escaped;static RuntimeException failure;static int visits;static Object token;static long seed;}
abstract class SuffixBase {
 SuffixBase(Object input,NULLTYPE marker){SuffixWitness.escaped=this;if(value()!=SuffixWitness.token||seed()!=SuffixWitness.seed||answer()!=0||SuffixWitness.visits!=0||marker!=null)throw new AssertionError("early null constructor state");if(input==null)throw SuffixWitness.failure;}
 SuffixBase(Object input,Integer rival){throw new AssertionError("wrong null overload");}
 abstract Object value();abstract long seed();abstract long answer();
}
class SuffixOwner {final Object token;final long seed;final int mode;SuffixOwner(Object token,long seed,int mode){this.token=token;this.seed=seed;this.mode=mode;}
 SuffixBase make(Object input){return new Member(input);}
 class Member extends SuffixBase {long result;Member(Object input){super(input,(NULLTYPE)null);long r=SuffixOwner.this.seed;for(int i=0;i<5;i++)r=r*31L+SuffixOwner.this.mode;result=r;SuffixWitness.visits++;}Object value(){return SuffixOwner.this.token;}long seed(){return SuffixOwner.this.seed;}long answer(){return result;}}
}
class SuffixDriver {public static void main(String[]args){int rows=0;
 for(Object token:new Object[]{null,"token",new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int mode:new int[]{-3,0,4})for(boolean fail:new boolean[]{false,true}){
 SuffixOwner owner=new SuffixOwner(token,seed,mode);SuffixWitness.token=token;SuffixWitness.seed=seed;SuffixWitness.escaped=null;SuffixWitness.visits=0;SuffixWitness.failure=new IllegalArgumentException("same");
 SuffixBase value=null;Throwable error=null;try{value=owner.make(fail?null:new Object());}catch(Throwable e){error=e;}
 java.math.BigInteger oracle=java.math.BigInteger.valueOf(seed);for(int i=0;i<5;i++)oracle=oracle.multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(mode));
 if(error!=(fail?SuffixWitness.failure:null)||SuffixWitness.escaped==null||SuffixWitness.escaped.value()!=token||SuffixWitness.escaped.seed()!=seed||SuffixWitness.escaped.answer()!=(fail?0:oracle.longValue())||SuffixWitness.visits!=(fail?0:1)||!fail&&value!=SuffixWitness.escaped)throw new AssertionError("null capture overload order overflow",error);rows++;
 }System.out.println(rows+":constructor:null:binding:original:oracle");}}
`

func TestAdversarialConstructorLoweringNullKeepsCaptureOrderAndExactOverload(t *testing.T) {
	for _, typ := range []string{"String", "int[]"} {
		for _, shape := range []string{"checked overload", "checked chain", "bare null"} {
			for _, target := range []int{8, 16} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(fmt.Sprintf("%s/%s/target%d/%s", typ, shape, target, debug), func(t *testing.T) {
						fixture := constructorNullLoweringFixture
						if shape == "checked chain" {
							fixture = strings.Replace(fixture, "super(input,(NULLTYPE)null)", "super(input,(NULLTYPE)(Object)(Integer)null)", 1)
						}
						if shape == "bare null" {
							fixture = strings.Replace(fixture, ` SuffixBase(Object input,Integer rival){throw new AssertionError("wrong null overload");}`, "", 1)
							fixture = strings.Replace(fixture, `super(input,(NULLTYPE)null)`, `super(input,null)`, 1)
						}
						constructorSuffixFamilyAtTargetRoundTrip(t, strings.ReplaceAll(fixture, "NULLTYPE", typ), debug, []string{"SuffixOwner$Member"}, "72:constructor:null:binding:original:oracle\n", 8, target)
					})
				}
			}
		}
	}
}

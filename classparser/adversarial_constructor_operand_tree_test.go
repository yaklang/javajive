package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// All callable helpers and the unchanged driver are original dependencies.
// Only the physical Member is reconstructed. The driver independently checks
// effects before SUPER, abrupt exits, allocation results and wide-word overflow.
const constructorOperandTreeFixture = `class SuffixWitness {static SuffixBase escaped;static RuntimeException failure;static int fail;static String trace;static Object token;static long seed;static int visits;
 static void step(int n){trace+=n;if(fail==n)throw failure;}}
class SuffixPayload {final long word;SuffixPayload(long word){SuffixWitness.step(3);this.word=word;}}
class SuffixOps {
 static Object pass(Object value){SuffixWitness.step(1);return value;}
 static long number(long seed){SuffixWitness.step(2);return seed*31L;}
 static SuffixPayload pack(long word){return new SuffixPayload(word);}
 static SuffixPayload payload(long seed){return pack(number(seed));}
}
abstract class SuffixBase {final SuffixPayload payload;
 SuffixBase(Object input,SuffixPayload payload){SuffixWitness.escaped=this;if(value()!=SuffixWitness.token||seed()!=SuffixWitness.seed||answer()!=0||SuffixWitness.visits!=0||input!=SuffixWitness.token)throw new AssertionError("pre SUPER captured state");this.payload=payload;SuffixWitness.step(4);}
 abstract Object value();abstract long seed();abstract long answer();}
class SuffixOwner {final Object token;final long seed;final int mode;SuffixOwner(Object token,long seed,int mode){this.token=token;this.seed=seed;this.mode=mode;}
 SuffixBase make(){return new Member(token,seed,mode);}
 class Member extends SuffixBase {long result;Member(Object input,long seed,int mode){super(SuffixOps.pass(input),OPERAND);long r=seed;for(int i=0;i<5;i++)r=r*31L+mode;result=r;SuffixWitness.visits++;SuffixWitness.step(5);}
 Object value(){return SuffixOwner.this.token;}long seed(){return SuffixOwner.this.seed;}long answer(){return result;}}
}
class SuffixDriver {public static void main(String[]args){int rows=0;
 for(Object token:new Object[]{null,"token",new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int mode:new int[]{-3,0,4})for(int fail=0;fail<=5;fail++){
 SuffixWitness.escaped=null;SuffixWitness.failure=new IllegalArgumentException("same");SuffixWitness.fail=fail;SuffixWitness.trace="";SuffixWitness.token=token;SuffixWitness.seed=seed;SuffixWitness.visits=0;
 SuffixBase value=null;Throwable error=null;try{value=new SuffixOwner(token,seed,mode).make();}catch(Throwable e){error=e;}
 java.math.BigInteger word=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31));java.math.BigInteger result=java.math.BigInteger.valueOf(seed);for(int i=0;i<5;i++)result=result.multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(mode));
 String trace=fail==0?"12345":"12345".substring(0,fail);boolean entered=fail==0||fail>=4;boolean completedBody=fail==0||fail==5;
 if(error!=(fail==0?null:SuffixWitness.failure)||!SuffixWitness.trace.equals(trace)||(SuffixWitness.escaped!=null)!=entered||SuffixWitness.visits!=(completedBody?1:0)||fail!=0&&value!=null)throw new AssertionError("operand order or abrupt effect",error);
 if(entered&&(SuffixWitness.escaped.value()!=token||SuffixWitness.escaped.seed()!=seed||SuffixWitness.escaped.payload.word!=word.longValue()||SuffixWitness.escaped.answer()!=(completedBody?result.longValue():0)||fail==0&&value!=SuffixWitness.escaped))throw new AssertionError("capture word or escape identity");rows++;
 }System.out.println(rows+":constructor:operand:tree:independent:oracle");}}
`

func TestAdversarialConstructorOperandTreePreservesAllocationCallsAndAbruptOrder(t *testing.T) {
	constructorOperandTreeRoundTrip(t, 8)
}

func TestAdversarialConstructorOperandTreePreservesNativeJava4CaptureConvention(t *testing.T) {
	constructorOperandTreeRoundTrip(t, 4)
}

func constructorOperandTreeRoundTrip(t *testing.T, release int) {
	t.Helper()
	for _, shape := range []string{"static call", "nested calls", "allocation", "interface calls"} {
		if release < 8 && shape == "interface calls" {
			continue
		}
		for _, target := range []int{8, 16} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/target%d/%s", shape, target, debug), func(t *testing.T) {
					operand := "SuffixOps.payload(seed)"
					if shape == "nested calls" || shape == "interface calls" {
						operand = "SuffixOps.pack(SuffixOps.number(seed))"
					}
					if shape == "allocation" {
						operand = "new SuffixPayload(SuffixOps.number(seed))"
					}
					fixture := strings.Replace(constructorOperandTreeFixture, "OPERAND", operand, 1)
					if shape == "interface calls" {
						fixture = strings.Replace(fixture, "class SuffixOps", "interface SuffixOps", 1)
					}
					if release == 4 {
						fixture = strings.Replace(fixture, `for(Object token:new Object[]{null,"token",new Object()})for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(int mode:new int[]{-3,0,4})for(int fail=0;fail<=5;fail++){`, `Object[] tokens={null,"token",new Object()};long[] seeds={Long.MIN_VALUE,-1,0,Long.MAX_VALUE};int[] modes={-3,0,4};
 for(int ti=0;ti<tokens.length;ti++)for(int si=0;si<seeds.length;si++)for(int mi=0;mi<modes.length;mi++)for(int fail=0;fail<=5;fail++){Object token=tokens[ti];long seed=seeds[si];int mode=modes[mi];`, 1)
					}
					constructorSuffixFamilyAtTargetRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "216:constructor:operand:tree:independent:oracle\n", release, target)
				})
			}
		}
	}
}

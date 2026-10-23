package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Only the physical member is rebuilt. Its original superclass observes the
// capture before SUPER completes; its original caller checks every abrupt exit.
const constructorLiteralPacketFixture = `class SuffixWitness {static Object token;static int mode,fail,visits;static String trace;static RuntimeException failure;static SuffixBase escaped;
 static void step(int n){trace+=n;if(fail==n)throw failure;}}
abstract class SuffixBase {final String label;final long word;final double zero;final int code;
 SuffixBase(String label,long word,double zero,int code){SuffixWitness.escaped=this;this.label=label;this.word=word;this.zero=zero;this.code=code;
 if(value()!=SuffixWitness.token||answer()!=0||SuffixWitness.visits!=0)throw new AssertionError("early capture");SuffixWitness.step(1);}
 abstract Object value();abstract long answer();}
class SuffixOwner {final Object token;final int mode;SuffixOwner(Object token,int mode){this.token=token;this.mode=mode;}
 SuffixBase make(){return new Member(mode);}
 class Member extends SuffixBase {long result;Member(int mode){super(LABEL,Long.MIN_VALUE,-0.0,CODE);long r=mode;for(int i=0;i<5;i++)r=r*31+mode;result=r;SuffixWitness.visits++;SuffixWitness.step(2);}
 Object value(){return SuffixOwner.this.token;}long answer(){return result;}}}
class SuffixDriver {public static void main(String[]args){int rows=0;Object[] tokens={null,"token",new Object()};int[] modes={-3,0,4,Integer.MAX_VALUE};
 for(int ti=0;ti<tokens.length;ti++)for(int mi=0;mi<modes.length;mi++)for(int fail=0;fail<3;fail++){Object token=tokens[ti];int mode=modes[mi];SuffixWitness.token=token;SuffixWitness.mode=mode;SuffixWitness.fail=fail;SuffixWitness.visits=0;SuffixWitness.trace="";SuffixWitness.escaped=null;SuffixWitness.failure=new IllegalArgumentException("same");
 SuffixBase made=null;Throwable error=null;try{made=new SuffixOwner(token,mode).make();}catch(Throwable caught){error=caught;}
 java.math.BigInteger expected=java.math.BigInteger.valueOf(mode);for(int i=0;i<5;i++)expected=expected.multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(mode));SuffixBase seen=SuffixWitness.escaped;
 if(error!=(fail==0?null:SuffixWitness.failure)||!SuffixWitness.trace.equals(fail==1?"1":"12")||SuffixWitness.visits!=(fail==1?0:1)||seen==null||seen.value()!=token||seen.answer()!=(fail==1?0:expected.longValue())||fail==0&&made!=seen||fail!=0&&made!=null)throw new AssertionError("effects and identity",error);
 if(!seen.label.equals(LABEL)||seen.word!=Long.MIN_VALUE||Double.doubleToRawLongBits(seen.zero)!=Long.MIN_VALUE||seen.code!=CODE)throw new AssertionError("literal payload or category");rows++;}
 System.out.println(rows+":constructor:literal:packet:oracle");}}
`

func TestAdversarialConstructorLiteralPacketPreservesTypedWordsAndCapture(t *testing.T) {
	for _, literal := range []struct{ name, label, code string }{
		{"iconst", `"literal"`, "5"}, {"bipush", `"literal"`, "-123"}, {"sipush", `"literal"`, "12345"}, {"ldc", `"literal"`, "1000009"}, {"UTF16", `"a\ud800\\n"`, "-2147483648"},
	} {
		for _, target := range []int{8, 16} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/target%d/%s", literal.name, target, debug), func(t *testing.T) {
					fixture := strings.ReplaceAll(strings.ReplaceAll(constructorLiteralPacketFixture, "LABEL", literal.label), "CODE", literal.code)
					constructorSuffixFamilyAtTargetRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "36:constructor:literal:packet:oracle\n", 8, target)
				})
			}
		}
	}
}

func TestAdversarialConstructorLiteralPacketPreservesLegacyCaptureAndFloat(t *testing.T) {
	for _, release := range []int{4, 8} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("release%d/%s", release, debug), func(t *testing.T) {
				fixture := strings.ReplaceAll(strings.ReplaceAll(constructorLiteralPacketFixture, "LABEL", `"legacy\ud800"`), "CODE", "1000009")
				fixture = strings.ReplaceAll(fixture, "double zero", "float zero")
				fixture = strings.ReplaceAll(fixture, "-0.0", "-0.0F")
				fixture = strings.ReplaceAll(fixture, "Double.doubleToRawLongBits(seen.zero)!=Long.MIN_VALUE", "Float.floatToRawIntBits(seen.zero)!=Integer.MIN_VALUE")
				constructorSuffixFamilyAtTargetRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "36:constructor:literal:packet:oracle\n", release, 8)
			})
		}
	}
}

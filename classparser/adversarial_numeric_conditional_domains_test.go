package javaclassparser

import (
	"os"
	"testing"
)

// The unchanged driver observes int overload selection, signed/unsigned words,
// evaluation count, exact abrupt identity, and real narrow storage semantics.
func TestAdversarialNumericConditionalDomainsRoundTrip(t *testing.T) {
	const fixture = `
class NumericDomain {
 static int calls; static byte fb; static short fs; static char fc;
 static char readChar(int n,RuntimeException fault){calls++;if(fault!=null)throw fault;return (char)n;}
 static byte readByte(int n,RuntimeException fault){calls++;if(fault!=null)throw fault;return (byte)n;}
 static short readShort(int n,RuntimeException fault){calls++;if(fault!=null)throw fault;return (short)n;}
 static long overload(int n){return 0x100000000L+(long)n;}
 static long overload(char n){return 0x200000000L+(long)n;}
 static long overload(byte n){return 0x300000000L+(long)n;}
 static long overload(short n){return 0x400000000L+(long)n;}
 static long chooseChar(boolean yes,int n,int fallback,RuntimeException fault){int word=yes?readChar(n,fault):fallback;return overload(word);}
 static long chooseByte(boolean yes,int n,int fallback,RuntimeException fault){int word=yes?readByte(n,fault):fallback;return overload(word);}
 static long chooseShort(boolean yes,int n,int fallback,RuntimeException fault){int word=yes?readShort(n,fault):fallback;return overload(word);}
 static long narrow(boolean yes,int n){fb=(byte)(yes?n:65535);fs=(short)(yes?n:65535);fc=(char)(yes?n:-1);byte[]b=new byte[1];short[]s=new short[1];char[]c=new char[1];b[0]=(byte)(yes?n:65535);s[0]=(short)(yes?n:65535);c[0]=(char)(yes?n:-1);return ((long)(fb+b[0])<<32)^((long)(fs+s[0])<<16)^(fc+c[0]);}
}
class NumericDomainDriver {
 public static void main(String[]args){int rows=0;int[]words={Integer.MIN_VALUE,-65537,-32769,-32768,-129,-128,-1,0,1,127,128,32767,32768,65535,65536,Integer.MAX_VALUE};
 for(int n:words)for(int fallback:words)for(boolean yes:new boolean[]{false,true}){
  NumericDomain.calls=0;long c=NumericDomain.chooseChar(yes,n,fallback,null),b=NumericDomain.chooseByte(yes,n,fallback,null),s=NumericDomain.chooseShort(yes,n,fallback,null);
  long ec=0x100000000L+(long)(yes?(n&65535):fallback),eb=0x100000000L+(long)(yes?((n&255)>=128?(n&255)-256:n&255):fallback),es=0x100000000L+(long)(yes?((n&65535)>=32768?(n&65535)-65536:n&65535):fallback);
  if(c!=ec||b!=eb||s!=es||NumericDomain.calls!=(yes?3:0))throw new AssertionError("numeric promotion/overload/evaluation");rows++;
 }
 for(int n:words)for(boolean yes:new boolean[]{false,true}){
  int x=yes?n:65535;int bv=(x&255)>=128?(x&255)-256:x&255;int sv=(x&65535)>=32768?(x&65535)-65536:x&65535;int cv=(yes?n:-1)&65535;
  long expected=((long)(bv+bv)<<32)^((long)(sv+sv)<<16)^(cv+cv);if(NumericDomain.narrow(yes,n)!=expected)throw new AssertionError("narrow store");rows++;
 }
 RuntimeException fault=new RuntimeException("same");for(int kind=0;kind<3;kind++){NumericDomain.calls=0;try{if(kind==0)NumericDomain.chooseChar(true,3,-1,fault);else if(kind==1)NumericDomain.chooseByte(true,3,-1,fault);else NumericDomain.chooseShort(true,3,-1,fault);throw new AssertionError("missing fault");}catch(RuntimeException got){if(got!=fault||NumericDomain.calls!=1)throw new AssertionError("fault identity");}rows++;}
 if(rows!=547)throw new AssertionError("coverage "+rows);System.out.println("547:promotion:overloads:evaluation:identity:narrow-storage");
 }
}`
	t.Run("modern", func(t *testing.T) {
		testNativeIndependentFamilyFixture(t, fixture, []string{"NumericDomain"}, "NumericDomainDriver", "547:promotion:overloads:evaluation:identity:narrow-storage\n", nativeLexicalExactSignatures)
	})
	t.Run("original8", func(t *testing.T) {
		compiler := os.Getenv("JAVA8_JAVAC")
		if compiler == "" {
			t.Skip("JAVA8_JAVAC required for independent native8 compiler")
		}
		testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
			return nativePrivateEnumCompile(t, fixture, "NumericDomain", debug)
		}, NativeJavac8, compiler, []string{"NumericDomain"}, "NumericDomainDriver", "547:promotion:overloads:evaluation:identity:narrow-storage\n", nil, nativeLexicalExactSignatures)
	})
}

package javaclassparser

import (
	"os"
	"testing"
)

func TestAdversarialCharacterWordJoinPreservesAllBits(t *testing.T) {
	const fixture = `
public class WordCompare {
 static int effects;
 static byte signedByte(int x){effects++;return (byte)x;}
 static short signedShort(int x){effects++;return (short)x;}
 static int wide(int x){effects++;return x;}
 static int byteJoin(boolean select,int x){int word=0;if(select)word=signedByte(x);else word=0;return word;}
 static int shortJoin(boolean select,int x){int word=0;if(select)word=signedShort(x);else word=0;return word;}
 static int intJoin(boolean select,int x){int word=0;if(select)word=wide(x);else word=0;return word;}
 public static int firstBit(String left,int start,int length,String right,int otherStart,int otherLength){
  boolean zero=true;int end=start+length,otherEnd=otherStart+otherLength,limit=Math.max(end,otherEnd);int first=0,second=0;
  for(int i=0;i<limit;i++){int at=start+i,otherAt=otherStart+i;if(at>=end){first=0;}else{first=left.charAt(at);}if(right!=null&&otherAt<otherEnd){second=right.charAt(otherAt);}else{second=0;}
   if(first!=second)return i*16+Integer.numberOfLeadingZeros(first^second)-16;if(first!=0)zero=false;
  }return zero?-1:-2;
 }
}
class WordCompareDriver {public static void main(String[]args){int rows=0;String[] words={"","p","p0","p1","pa","pb","\u0000","\u0000x","\u8000","\uffff","\uD800\uDC00"};
 for(String a:words)for(String b:words){int expected=-1;boolean different=false;for(int i=0;i<Math.max(a.length(),b.length());i++){int x=i<a.length()?a.charAt(i):0,y=i<b.length()?b.charAt(i):0;if(x!=0||y!=0)expected=-2;for(int bit=15;bit>=0;bit--)if(((x>>>bit)&1)!=((y>>>bit)&1)){expected=i*16+15-bit;different=true;break;}if(different)break;}
  int got=WordCompare.firstBit(a,0,a.length(),b,0,b.length());if(got!=expected)throw new AssertionError("word bits "+rows+":"+got+" != "+expected);rows++;
 }
 for(int x:new int[]{Integer.MIN_VALUE,-32769,-32768,-129,-128,-1,0,1,127,128,32767,32768,65535,Integer.MAX_VALUE})for(boolean select:new boolean[]{false,true}){
  WordCompare.effects=0;int b=WordCompare.byteJoin(select,x),s=WordCompare.shortJoin(select,x),i=WordCompare.intJoin(select,x);
  int wantB=select?((x&255)<128?(x&255):(x&255)-256):0,wantS=select?((x&65535)<32768?(x&65535):(x&65535)-65536):0;
  if(b!=wantB||s!=wantS||i!=(select?x:0)||WordCompare.effects!=(select?3:0))throw new AssertionError("descriptor join "+x+":"+select);rows++;
 }System.out.println(rows+":character:word:join:all-bits");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "WordCompare", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"WordCompare"}, "WordCompareDriver", "149:character:word:join:all-bits\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

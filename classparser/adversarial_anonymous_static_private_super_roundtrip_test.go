package javaclassparser

import (
	"strings"
	"testing"
)

// Inheritance from an owned static member carries no enclosing-instance word.
// Its private constructor still has a distinct physical access bridge. Keep
// the ordinary arguments, exact private target and adjacent unused marker
// separate; none of these permits inventing a lexical outer-instance path.
const anonymousStaticPrivateSuperFixture = `public class StaticSuperScope{
 static String trace="";static int arguments,parents;static final java.io.IOException failure=new java.io.IOException("identity");
 public static abstract class Parent{
  final long word;final Object tag;
  private Parent(long word,Object tag)throws java.io.IOException{trace+="P";parents++;this.word=word;this.tag=tag;if(word<0)throw failure;}
  private Parent(int word,Object tag)throws java.io.IOException{throw new AssertionError("wrong overload");}
  public abstract long value();
 }
 private static long argument(long word){trace+="A";arguments++;return word;}
 public static Parent make(final long word,final Object tag)throws java.io.IOException{
  return new Parent(argument(word),tag){public long value(){trace+="G";if(this.tag!=tag)throw new AssertionError("capture identity");return word^0xCAFEBABEL;}};
 }
}
class StaticSuperDriver{
 public static void main(String[]args)throws Exception{int rows=0;Object sentinel=new Object();
  for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object tag:new Object[]{null,sentinel}){
   StaticSuperScope.trace="";StaticSuperScope.arguments=0;StaticSuperScope.parents=0;
   try{StaticSuperScope.Parent p=StaticSuperScope.make(word,tag);
    long want=java.math.BigInteger.valueOf(word).xor(java.math.BigInteger.valueOf(0xCAFEBABEL)).longValue();
    if(word<0||p.word!=word||p.tag!=tag||p.value()!=want||!StaticSuperScope.trace.equals("APG"))throw new AssertionError("values, capture, overload or evaluation order");
    Class<?> c=p.getClass();if(!c.isAnonymousClass()||c.getSuperclass()!=StaticSuperScope.Parent.class||c.getEnclosingClass()!=StaticSuperScope.class||!c.getEnclosingMethod().getName().equals("make"))throw new AssertionError("source ownership");
   }catch(java.io.IOException e){if(word>=0||e!=StaticSuperScope.failure||!StaticSuperScope.trace.equals("AP"))throw new AssertionError("exception identity or partial effects",e);}
   if(StaticSuperScope.arguments!=1||StaticSuperScope.parents!=1)throw new AssertionError("duplicated evaluation");rows++;
  }System.out.println(rows+":static-private-parent:overload:capture:effects:identity");
 }
}`

func TestAdversarialAnonymousStaticMemberPrivateSuperRoundTrip(t *testing.T) {
	for _, owner := range []string{"StaticSuperScope", "RenamedStaticSuperScope"} {
		t.Run(owner, func(t *testing.T) {
			source := strings.ReplaceAll(anonymousStaticPrivateSuperFixture, "StaticSuperScope", owner)
			testNativePrivateSetterCompiledFixture(t, owner, "StaticSuperDriver", "10:static-private-parent:overload:capture:effects:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
			})
		})
	}
}

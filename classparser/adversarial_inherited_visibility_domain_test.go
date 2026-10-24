package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialInheritedVisibilityBridgeRetainsOriginalAbsence(t *testing.T) {
	for _, shape := range []string{"self", "object", "existing bridge", "public parent", "renamed"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, compiler := range []struct {
				name    string
				target  int
				profile SourceCompilerProfile
			}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
				t.Run(shape+"/"+debug+"/"+compiler.name, func(t *testing.T) {
					source, owner := inheritedVisibilityDomainFixture, "SuffixOwner$Visible"
					switch shape {
					case "object":
						source = strings.Replace(source, "T extends Hidden<T>", "T", 1)
					case "existing bridge":
						source = strings.Replace(source, "long first,bits;", "public int read(){return 19;} long first,bits;", 1)
					case "public parent":
						source = strings.Replace(source, "static class Hidden", "public static class Hidden", 1)
					case "renamed":
						source = strings.NewReplacer("Hidden", "Ancestor", "Visible", "Child", "Derived", "Grandchild").Replace(source)
						owner = "SuffixOwner$Child"
					}
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, []string{owner}, "72:inherited:visibility:dispatch:failure:words\n", 8, compiler.target, compiler.profile)
				})
			}
		}
	}
}

// javac8 leaves the generic inherited method in Hidden. Later javac versions
// additionally declare a synthetic visibility forwarder in Visible. Its
// presence changes the original declaration/lookup surface even though the
// ordinary invocation still reaches Hidden's implementation.
const inheritedVisibilityDomainFixture = `class SuffixOwner {
 static String trace;static boolean fail;static java.io.IOException failure;
 static class Hidden<T extends Hidden<T>> {
  long first,bits;String text;
  public T set(long first,double second,String text)throws java.io.IOException{
   trace+="S";this.first=first;this.bits=Double.doubleToRawLongBits(second);this.text=text;
   if(fail)throw failure;observe();return (T)this;
  }
  void observe(){trace+="P";}
 }
 public static class Visible extends Hidden<Visible> {}
 static class Derived extends Visible {void observe(){trace+="D";}}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 try{SuffixOwner.Visible.class.getDeclaredMethod("set",long.class,double.class,String.class);throw new AssertionError("invented declaration");}catch(NoSuchMethodException expected){}
 int rows=0;
 for(long first:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double second:new double[]{-0.0d,0.0d,Double.NaN})for(String text:new String[]{null,"字"})for(boolean fail:new boolean[]{false,true})for(boolean derived:new boolean[]{false,true}){
  SuffixOwner.Visible receiver=derived?new SuffixOwner.Derived():new SuffixOwner.Visible();SuffixOwner.trace="";SuffixOwner.fail=fail;SuffixOwner.failure=new java.io.IOException("identity");
  SuffixOwner.Visible result=null;Throwable error=null;try{result=receiver.set(first,second,text);}catch(Throwable e){error=e;}
  if(error!=(fail?SuffixOwner.failure:null)||!fail&&result!=receiver||receiver.first!=first||receiver.bits!=Double.doubleToRawLongBits(second)||receiver.text!=text||!SuffixOwner.trace.equals(fail?"S":derived?"SD":"SP"))throw new AssertionError("original inherited method",error);
  rows++;
 }System.out.println(rows+":inherited:visibility:dispatch:failure:words");
}}
`

package javaclassparser

import (
	"strings"
	"testing"
)

// The unchanged parent calls a virtual method before the child constructor
// returns. The captured outer reference must therefore be installed before
// delegation, and an invented CHECKCAST cannot be hidden by capture lowering.
func TestAdversarialConstructorNullUniqueGenericParent(t *testing.T) {
	for _, generic := range []bool{false, true} {
		for _, shape := range []string{"unique", "renamed", "competing"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, compiler := range []struct {
					name    string
					target  int
					profile SourceCompilerProfile
				}{{"modern8", 8, ModernJavac}, {"modern21", 21, ModernJavac}, {"native8", 8, NativeJavac8}} {
					t.Run(map[bool]string{false: "plain", true: "generic"}[generic]+"/"+shape+"/"+debug+"/"+compiler.name, func(t *testing.T) {
						source, member := constructorNullFamilyFixture, "SuffixOwner$Member"
						if generic {
							source = strings.Replace(source, "class Base {", "class Base<T> {", 1)
							source = strings.Replace(source, "extends Base {", "extends Base<String> {", 1)
						}
						if shape == "competing" {
							source = strings.Replace(source, "abstract int read();", "Base(String other){throw new AssertionError(\"wrong overload\");} abstract int read();", 1)
							source = strings.Replace(source, "super(null)", "super((Token)null)", 1)
						}
						if shape == "renamed" {
							source = strings.NewReplacer("Base", "Ancestor", "Member", "Descendant", "Token", "Marker").Replace(source)
							member = "SuffixOwner$Descendant"
						}
						constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, []string{member}, "6:null:constructor:capture:order:identity\n", 8, compiler.target, compiler.profile)
					})
				}
			}
		}
	}
}

const constructorNullFamilyFixture = `class SuffixOwner {
 static String trace; static RuntimeException failure; static boolean fail;
 final int value; SuffixOwner(int value){this.value=value;}
 static class Token {}
 static abstract class Base {
  final int observed;
  Base(Token token){trace+="P";if(token!=null)throw new AssertionError("null");observed=read();if(fail)throw failure;}
  abstract int read();
 }
 class Member extends Base {
  Member(){super(null);}
  int read(){trace+="R";return SuffixOwner.this.value;}
 }
 Member make(){return new Member();}
}
class SuffixDriver {public static void main(String[]args){int rows=0;
 for(int value:new int[]{Integer.MIN_VALUE,0,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){
  SuffixOwner owner=new SuffixOwner(value);SuffixOwner.trace="";SuffixOwner.fail=fail;SuffixOwner.failure=new IllegalStateException("identity");
  Throwable error=null;SuffixOwner.Member result=null;try{result=owner.make();}catch(Throwable e){error=e;}
  if(error!=(fail?SuffixOwner.failure:null)||!SuffixOwner.trace.equals("PR")||!fail&&(result==null||result.observed!=value))throw new AssertionError("capture/order/exception",error);
  rows++;
 }System.out.println(rows+":null:constructor:capture:order:identity");}}
`

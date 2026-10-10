package javaclassparser

import (
	"strings"
	"testing"
)

const anonymousInheritedArgumentFixture = `class InheritedArgEffects{static int calls,fail,x,y;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("original");static int left(int x){trace+="A";if(fail==1)throw failure;return x;}static int right(int y){trace+="B";if(fail==2)throw failure;return y;}}
class InheritedArgOwner{private Object token;InheritedArgOwner(Object token){this.token=token;}abstract class Layer{final Object run(int x,int y){InheritedArgEffects.trace+="R";InheritedArgEffects.x=x;InheritedArgEffects.y=y;if(InheritedArgEffects.fail==3)throw InheritedArgEffects.failure;return visit();}Object run(long x,long y){throw new AssertionError("wrong widened overload");}Object run(Integer x,Integer y){throw new AssertionError("wrong boxed overload");}Object run(int...xs){throw new AssertionError("wrong varargs overload");}abstract Object visit();}Object get(int x,int y){return new Layer(){Object visit(){InheritedArgEffects.calls++;InheritedArgEffects.trace+="V";if(!getClass().isAnonymousClass())throw new AssertionError("anonymous inherited receiver identity");if(InheritedArgEffects.fail==4)throw InheritedArgEffects.failure;return token;}}.run(InheritedArgEffects.left(x),InheritedArgEffects.right(y));}}
class InheritedArgDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()}){InheritedArgOwner owner=new InheritedArgOwner(token);for(int fail:new int[]{0,1,2,3,4})for(int x:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}){int y=~x;InheritedArgEffects.calls=0;InheritedArgEffects.trace="";InheritedArgEffects.fail=fail;try{Object got=owner.get(x,y);if(fail!=0||got!=token)throw new AssertionError("inherited argument result identity");}catch(IllegalArgumentException e){if(fail==0||e!=InheritedArgEffects.failure)throw new AssertionError("inherited argument failure identity");}String expected=fail==1?"A":fail==2?"AB":fail==3?"ABR":"ABRV";if(InheritedArgEffects.calls!=(fail==0||fail==4?1:0)||!InheritedArgEffects.trace.equals(expected))throw new AssertionError("inherited argument dispatch/once/order");if(fail!=1&&fail!=2&&(InheritedArgEffects.x!=x||InheritedArgEffects.y!=y))throw new AssertionError("inherited argument binding/order");rows++;}}System.out.println(rows+":anonymous:inherited:arguments");}}
`
const anonymousInheritedReferenceArgumentFixture = `class InheritedRefEffects{static int calls,fail;static Object x,y;static String trace="";static final IllegalArgumentException failure=new IllegalArgumentException("original");static String left(String x){trace+="A";if(fail==1)throw failure;return x;}static String right(String y){trace+="B";if(fail==2)throw failure;return y;}}
class InheritedRefOwner{private Object token;InheritedRefOwner(Object token){this.token=token;}abstract class Layer{final Object run(Object x,Object y){InheritedRefEffects.trace+="R";InheritedRefEffects.x=x;InheritedRefEffects.y=y;if(InheritedRefEffects.fail==3)throw InheritedRefEffects.failure;return visit();}Object run(String x,String y){throw new AssertionError("wrong narrower overload");}Object run(Object...xs){throw new AssertionError("wrong varargs overload");}abstract Object visit();}Object get(String x,String y){return new Layer(){Object visit(){InheritedRefEffects.calls++;InheritedRefEffects.trace+="V";if(!getClass().isAnonymousClass())throw new AssertionError("anonymous inherited receiver identity");if(InheritedRefEffects.fail==4)throw InheritedRefEffects.failure;return token;}}.run((Object)InheritedRefEffects.left(x),(Object)InheritedRefEffects.right(y));}}
class InheritedRefDriver{public static void main(String[]args){int rows=0;for(Object token:new Object[]{null,new Object()}){InheritedRefOwner owner=new InheritedRefOwner(token);for(int fail:new int[]{0,1,2,3,4})for(String x:new String[]{null,"",new String("payload")}){String y=new String("right");InheritedRefEffects.calls=0;InheritedRefEffects.trace="";InheritedRefEffects.fail=fail;try{Object got=owner.get(x,y);if(fail!=0||got!=token)throw new AssertionError("inherited argument result identity");}catch(IllegalArgumentException e){if(fail==0||e!=InheritedRefEffects.failure)throw new AssertionError("inherited argument failure identity");}String expected=fail==1?"A":fail==2?"AB":fail==3?"ABR":"ABRV";if(InheritedRefEffects.calls!=(fail==0||fail==4?1:0)||!InheritedRefEffects.trace.equals(expected))throw new AssertionError("inherited argument dispatch/once/order");if(fail!=1&&fail!=2&&(InheritedRefEffects.x!=x||InheritedRefEffects.y!=y))throw new AssertionError("inherited argument binding/order");rows++;}}System.out.println(rows+":anonymous:inherited:arguments");}}
`

func TestAdversarialAnonymousInheritedArgumentCallsKeepBinding(t *testing.T) {
	for _, shape := range []string{"int-pair", "long-pair", "reference-widening", "array-widening"} {
		t.Run(shape, func(t *testing.T) {
			for _, rename := range []string{"original", "renamed"} {
				t.Run(rename, func(t *testing.T) {
					source, owner, driver, want := anonymousInheritedArgumentFixture, "InheritedArgOwner", "InheritedArgDriver", "40:anonymous:inherited:arguments\n"
					if shape == "long-pair" {
						source = strings.Replace(source, "Object run(long x,long y){", "Object run(float x,float y){", 1)
						source = strings.Replace(source, "static int calls,fail,x,y;", "static int calls,fail;static long x,y;", 1)
						source = strings.NewReplacer("int x", "long x", "int y", "long y", "static int left(", "static long left(", "static int right(", "static long right(", "new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE}", "new long[]{Long.MIN_VALUE,-1L,0L,Long.MAX_VALUE}").Replace(source)
					}
					if shape == "reference-widening" || shape == "array-widening" {
						source, owner, driver, want = anonymousInheritedReferenceArgumentFixture, "InheritedRefOwner", "InheritedRefDriver", "30:anonymous:inherited:arguments\n"
					}
					if shape == "array-widening" {
						source = strings.NewReplacer("static String left(String x)", "static String[] left(String[] x)", "static String right(String y)", "static String[] right(String[] y)", "run(String x,String y)", "run(String[] x,String[] y)", "get(String x,String y)", "get(String[] x,String[] y)", "for(String x:new String[]{null,\"\",new String(\"payload\")})", "for(String[] x:new String[][]{null,new String[0],new String[]{\"payload\"}})", "String y=new String(\"right\");", "String[] y=new String[]{\"right\"};").Replace(source)
					}
					if rename == "renamed" {
						source = strings.ReplaceAll(source, owner, "ArgumentScope")
						owner = "ArgumentScope"
					}
					testNativePrivateSetterFixture(t, source, owner, driver, want)
				})
			}
		})
	}
}

func TestAdversarialAnonymousInheritedArgumentExternalParentKeepsAccess(t *testing.T) {
	source := anonymousInheritedArgumentFixture
	start := strings.Index(source, "abstract class Layer{")
	end := strings.Index(source, "Object get(int x,int y)")
	if start < 0 || end <= start {
		t.Fatal("fixture shape")
	}
	parent := strings.Replace(source[start:end], "abstract class Layer{", "abstract class InheritedArgBase{", 1)
	source = parent + "\n" + source[:start] + source[end:]
	source = strings.Replace(source, "new Layer(){", "new InheritedArgBase(){", 1)
	testNativePrivateSetterFixture(t, source, "InheritedArgOwner", "InheritedArgDriver", "40:anonymous:inherited:arguments\n")
}

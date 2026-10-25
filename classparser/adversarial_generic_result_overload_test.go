package javaclassparser

import (
	"os"
	"testing"
)

func TestAdversarialGenericResultOverloadRetainsConcreteInputTarget(t *testing.T) {
	const fixture = `interface InputView {}
class InputToken<T> implements InputView {T cast(Object value){return (T)value;}}
class RouteTrace {static int depth,calls;static String trace;static Object value;static boolean fail;static final RuntimeException FAILURE=new IllegalStateException("same");}
public class RouteOwner {
 public <T>T select(String text,InputToken<T> token){RouteTrace.depth++;RouteTrace.trace+="W";if(RouteTrace.depth>1)throw new AssertionError("recursive overload");Object result=select(text,(InputView)token);return token.cast(result);}
 public <T>T select(String text,InputView view){RouteTrace.calls++;RouteTrace.trace+="T";if(RouteTrace.fail)throw RouteTrace.FAILURE;return (T)RouteTrace.value;}
 public Object select(String text,Object other){throw new AssertionError("wrong broad overload");}
}
class RouteDriver {public static void main(String[]args){RouteOwner owner=new RouteOwner();int rows=0;for(Object value:new Object[]{null,"result",Integer.valueOf(42),new Object()})for(boolean fail:new boolean[]{false,true}){RouteTrace.depth=0;RouteTrace.calls=0;RouteTrace.trace="";RouteTrace.value=value;RouteTrace.fail=fail;try{Object got=owner.select("input",new InputToken<Object>());if(fail||got!=value)throw new AssertionError("result identity");}catch(RuntimeException e){if(!fail||e!=RouteTrace.FAILURE)throw new AssertionError("failure identity",e);}if(RouteTrace.depth!=1||RouteTrace.calls!=1||!RouteTrace.trace.equals("WT"))throw new AssertionError("target/effects");rows++;}System.out.println(rows+":generic-result:concrete-input:overload:identity:effects");}}
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
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "RouteOwner", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"RouteOwner"}, "RouteDriver", "8:generic-result:concrete-input:overload:identity:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialGenericResultOverloadRetainsReflectiveTypeTarget(t *testing.T) {
	const fixture = `
class ReflectRouteTrace {static int depth,calls;static String trace;static Object value;static boolean fail;static final RuntimeException FAILURE=new IllegalStateException("same");}
public class ReflectRouteOwner {
 public <T>T select(String text,Class<T> token){ReflectRouteTrace.depth++;ReflectRouteTrace.trace+="W";if(ReflectRouteTrace.depth>1)throw new AssertionError("recursive overload");Object result=select(text,(java.lang.reflect.Type)token);return token.cast(result);}
 public <T>T select(String text,java.lang.reflect.Type view){ReflectRouteTrace.calls++;ReflectRouteTrace.trace+="T";if(ReflectRouteTrace.fail)throw ReflectRouteTrace.FAILURE;return (T)ReflectRouteTrace.value;}
 public Object select(String text,Object other){throw new AssertionError("wrong broad overload");}
}
class ReflectRouteDriver {public static void main(String[]args){ReflectRouteOwner owner=new ReflectRouteOwner();int rows=0;for(Object value:new Object[]{null,"result",Integer.valueOf(42),new Object()})for(boolean fail:new boolean[]{false,true}){ReflectRouteTrace.depth=0;ReflectRouteTrace.calls=0;ReflectRouteTrace.trace="";ReflectRouteTrace.value=value;ReflectRouteTrace.fail=fail;try{Object got=owner.select("input",Object.class);if(fail||got!=value)throw new AssertionError("result identity");}catch(RuntimeException e){if(!fail||e!=ReflectRouteTrace.FAILURE)throw new AssertionError("failure identity",e);}if(ReflectRouteTrace.depth!=1||ReflectRouteTrace.calls!=1||!ReflectRouteTrace.trace.equals("WT"))throw new AssertionError("target/effects");rows++;}System.out.println(rows+":generic-result:concrete-input:overload:identity:effects");}}
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
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "ReflectRouteOwner", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"ReflectRouteOwner"}, "ReflectRouteDriver", "8:generic-result:concrete-input:overload:identity:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialGenericResultOverloadRetainsAdaptedReflectiveTypeTarget(t *testing.T) {
	const fixture = `
class AdaptedRouteTrace {static int depth,calls;static String trace;static Object value;static boolean fail;static final RuntimeException FAILURE=new IllegalStateException("same");}
public class AdaptedRouteOwner {
 public <T>T select(String text,Class<T> token){AdaptedRouteTrace.depth++;AdaptedRouteTrace.trace+="W";if(AdaptedRouteTrace.depth>1)throw new AssertionError("recursive overload");Object result=select(text,(java.lang.reflect.Type)token);return RouteAdapter.wrap(token).cast(result);}
 public <T>T select(String text,java.lang.reflect.Type view){AdaptedRouteTrace.calls++;AdaptedRouteTrace.trace+="T";if(AdaptedRouteTrace.fail)throw AdaptedRouteTrace.FAILURE;return (T)AdaptedRouteTrace.value;}
 public Object select(String text,Object other){throw new AssertionError("wrong broad overload");}
}
class RouteAdapter {static <T>Class<T> wrap(Class<T> token){return token;}}
class AdaptedRouteDriver {public static void main(String[]args){AdaptedRouteOwner owner=new AdaptedRouteOwner();int rows=0;for(Object value:new Object[]{null,"result",Integer.valueOf(42),new Object()})for(boolean fail:new boolean[]{false,true}){AdaptedRouteTrace.depth=0;AdaptedRouteTrace.calls=0;AdaptedRouteTrace.trace="";AdaptedRouteTrace.value=value;AdaptedRouteTrace.fail=fail;try{Object got=owner.select("input",Object.class);if(fail||got!=value)throw new AssertionError("result identity");}catch(RuntimeException e){if(!fail||e!=AdaptedRouteTrace.FAILURE)throw new AssertionError("failure identity",e);}if(AdaptedRouteTrace.depth!=1||AdaptedRouteTrace.calls!=1||!AdaptedRouteTrace.trace.equals("WT"))throw new AssertionError("target/effects");rows++;}System.out.println(rows+":generic-result:concrete-input:overload:identity:effects");}}
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
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "AdaptedRouteOwner", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"AdaptedRouteOwner"}, "AdaptedRouteDriver", "8:generic-result:concrete-input:overload:identity:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

func TestAdversarialGenericResultOverloadRetainsAdaptedConcreteInputTarget(t *testing.T) {
	const fixture = `interface InputView {}
class InputToken<T> implements InputView {T cast(Object value){return (T)value;}}
class RouteTrace {static int depth,calls;static String trace;static Object value;static boolean fail;static final RuntimeException FAILURE=new IllegalStateException("same");}
public class RouteOwner {
 public <T>T select(String text,InputToken<T> token){RouteTrace.depth++;RouteTrace.trace+="W";if(RouteTrace.depth>1)throw new AssertionError("recursive overload");Object result=select(text,(InputView)token);return TokenAdapter.wrap(token).cast(result);}
 public <T>T select(String text,InputView view){RouteTrace.calls++;RouteTrace.trace+="T";if(RouteTrace.fail)throw RouteTrace.FAILURE;return (T)RouteTrace.value;}
 public Object select(String text,Object other){throw new AssertionError("wrong broad overload");}
}
class TokenAdapter {static <T>InputToken<T> wrap(InputToken<T> token){return token;}}
class RouteDriver {public static void main(String[]args){RouteOwner owner=new RouteOwner();int rows=0;for(Object value:new Object[]{null,"result",Integer.valueOf(42),new Object()})for(boolean fail:new boolean[]{false,true}){RouteTrace.depth=0;RouteTrace.calls=0;RouteTrace.trace="";RouteTrace.value=value;RouteTrace.fail=fail;try{Object got=owner.select("input",new InputToken<Object>());if(fail||got!=value)throw new AssertionError("result identity");}catch(RuntimeException e){if(!fail||e!=RouteTrace.FAILURE)throw new AssertionError("failure identity",e);}if(RouteTrace.depth!=1||RouteTrace.calls!=1||!RouteTrace.trace.equals("WT"))throw new AssertionError("target/effects");rows++;}System.out.println(rows+":generic-result:concrete-input:overload:identity:effects");}}
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
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "RouteOwner", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"RouteOwner"}, "RouteDriver", "8:generic-result:concrete-input:overload:identity:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}

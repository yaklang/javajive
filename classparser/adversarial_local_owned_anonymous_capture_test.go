package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source spelling, value storage and receiver identity are separate obligations.
// Rebuild the original compiler protocol and run an unchanged binary caller;
// exact class/field/constructor ABI and generic signatures remain mandatory.
func TestAdversarialLocalOwnedAnonymousCaptureBindings(t *testing.T) {
	const body = `class CaptureTransport {static CaptureInput.View make(java.util.function.Supplier<Object> alias){class Local{CaptureInput.View create(){return new CaptureInput.View(){public Object retained(){Object result=alias.get();if(result==null)throw new AssertionError("result");return result;}};}}return new Local().create();}static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->make(alias);}}`
	for _, binding := range []string{"alias", "var1", "capturé"} {
		t.Run(binding, func(t *testing.T) {
			source := captureActivationInput + strings.ReplaceAll(body, "alias", binding) + captureActivationDriver
			for _, profile := range []string{"modern", "native8"} {
				t.Run(profile, func(t *testing.T) {
					if profile == "native8" {
						testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
							return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
						}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "16:nested:capture:timing:activation:failure:identity\n", nil, nativeLexicalExactSignatures)
					} else {
						testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "16:nested:capture:timing:activation:failure:identity\n", nativeLexicalExactSignatures)
					}
				})
			}
		})
	}
}

func TestAdversarialLocalOwnedAnonymousCaptureCarrierIdentity(t *testing.T) {
	source := captureActivationInput + `class CaptureTransport {static CaptureInput.View make(java.util.function.Supplier<Object> alias){class Local{CaptureInput.View create(){return new CaptureInput.View(){public Object retained(){return alias.get();}};}}return new Local().create();}static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->make(alias);}}
 class CaptureDriver {public static void main(String[]args)throws Exception{CaptureInput input=new CaptureInput(false);java.util.function.Function<Object,CaptureInput.View> factory=CaptureTransport.bind(input.supplier());CaptureInput.View x=factory.apply(null),y=factory.apply(null);java.lang.reflect.Field[] enclosing=x.getClass().getDeclaredFields();if(enclosing.length!=1||!enclosing[0].isSynthetic())throw new AssertionError("original receiver carrier");java.lang.reflect.Field outer=enclosing[0];outer.setAccessible(true);Object a=outer.get(x),b=outer.get(y);if(a==null||b==null||a==b)throw new AssertionError("activation carrier identity");java.lang.reflect.Field[] captured=a.getClass().getDeclaredFields();if(captured.length!=1||!captured[0].isSynthetic())throw new AssertionError("original value carrier");java.lang.reflect.Field value=captured[0];value.setAccessible(true);final Object marker=new Object();java.util.function.Supplier<Object> replacement=()->marker;value.set(a,replacement);if(x.retained()!=marker||input.calls!=0)throw new AssertionError("late capture update");if(y.retained()==marker||input.calls!=1)throw new AssertionError("distinct carrier bindings");value.set(a,null);try{x.retained();throw new AssertionError("missing null capture");}catch(NullPointerException expected){}outer.set(x,null);try{x.retained();throw new AssertionError("missing null receiver");}catch(NullPointerException expected){}if(input.calls!=1)throw new AssertionError("premature supplier");System.out.println("capture:carrier:identity:late-update:null-boundaries");}}
 `
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "capture:carrier:identity:late-update:null-boundaries\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "capture:carrier:identity:late-update:null-boundaries\n", nativeLexicalExactSignatures)
			}
		})
	}
}
func TestAdversarialLocalOwnedAnonymousCaptureEqualTypedAndWide(t *testing.T) {
	source := captureActivationInput + `class CaptureTransport {static CaptureInput.View make(java.util.function.Supplier<Object> alias,java.util.function.Supplier<Object> peer,long wide){class Local{CaptureInput.View create(){return new CaptureInput.View(){public Object retained(){Object result=alias.get();if(wide!=17L||peer==alias)throw new AssertionError("capture words");return result;}};}}return new Local().create();}static Object peer(){throw new AssertionError("unrelated capture evaluated");}static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->make(alias,()->peer(),17L);}}` + captureActivationDriver
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "16:nested:capture:timing:activation:failure:identity\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "16:nested:capture:timing:activation:failure:identity\n", nativeLexicalExactSignatures)
			}
		})
	}
}

func TestAdversarialLocalOwnedAnonymousGenericCaptureBinding(t *testing.T) {
	source := `class CaptureInput {interface View{String retained();}}class CaptureTransport {static String choose(Object value){return "object";}static String choose(Number value){return value==null?"number:null":"number:"+value.longValue();}static CaptureInput.View make(java.util.function.Supplier<? extends Number> alias){class Local {CaptureInput.View create(){return new CaptureInput.View(){public String retained(){return choose(alias.get());}};}}return new Local().create();}}
 class CaptureDriver {public static void main(String[]args)throws Exception{for(Number n:new Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MAX_VALUE)}){final int[] calls={0};java.util.function.Supplier<Number> supplier=()->{calls[0]++;return n;};CaptureInput.View view=CaptureTransport.make(supplier);if(calls[0]!=0)throw new AssertionError("premature capture");String expected=n==null?"number:null":"number:"+n.longValue();if(!expected.equals(view.retained())||calls[0]!=1)throw new AssertionError("original generic overload");java.lang.reflect.Field outer=view.getClass().getDeclaredFields()[0];outer.setAccessible(true);Object local=outer.get(view);java.lang.reflect.Field field=local.getClass().getDeclaredFields()[0];field.setAccessible(true);java.util.function.Supplier<Object> wrong=()->"wrong";field.set(local,wrong);try{view.retained();throw new AssertionError("original cast missing");}catch(ClassCastException expectedFailure){}}System.out.println("generic:binding:null:deferred:cast");}}`
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "generic:binding:null:deferred:cast\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "generic:binding:null:deferred:cast\n", nativeLexicalExactSignatures)
			}
		})
	}
}

// A captured field read still has its original null/exception behavior when
// its assigned local is unused. Another local method keeps the capture live.
func TestAdversarialLocalOwnedAnonymousUnusedReadKeepsNullBoundary(t *testing.T) {
	source := `class CaptureInput {interface View{Object retained();}}class CaptureTransport {static CaptureInput.View make(final Object alias){class Local {Object keep(){return alias;}CaptureInput.View create(){return new CaptureInput.View(){public Object retained(){Object unused=alias;return Integer.valueOf(7);}};}}return new Local().create();}}class CaptureDriver{public static void main(String[]args)throws Exception{CaptureInput.View v=CaptureTransport.make(new Object());java.lang.reflect.Field f=v.getClass().getDeclaredFields()[0];f.setAccessible(true);f.set(v,null);try{v.retained();throw new AssertionError("original field read was lost");}catch(NullPointerException expected){}System.out.println("unused:field:null-boundary");}}`
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "unused:field:null-boundary\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "unused:field:null-boundary\n", nativeLexicalExactSignatures)
			}
		})
	}
}

// An outer capture used by a different method does not constrain the ordinary
// field namespace of a child that never reads that capture.
func TestAdversarialLocalOwnedAnonymousUnusedOuterBinding(t *testing.T) {
	source := `class CaptureInput {interface View{Object retained();}}class CaptureTransport {static CaptureInput.View make(final Object alias){class Local {Object keep(){return alias;}CaptureInput.View create(){return new CaptureInput.View(){Object alias;public Object retained(){return this.alias;}};}}return new Local().create();}}class CaptureDriver{public static void main(String[]args)throws Exception{Object ignored=new Object(),marker=new Object();CaptureInput.View v=CaptureTransport.make(ignored);if(v.retained()!=null)throw new AssertionError("field borrowed unrelated binding");java.lang.reflect.Field f=v.getClass().getDeclaredField("alias");f.setAccessible(true);f.set(v,marker);if(v.retained()!=marker)throw new AssertionError("own field identity");System.out.println("unused:outer:binding:own-field");}}`
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", "unused:outer:binding:own-field\n", nil, nativeLexicalExactSignatures)
			} else {
				testNativeIndependentFamilyFixture(t, source, []string{"CaptureTransport"}, "CaptureDriver", "unused:outer:binding:own-field\n", nativeLexicalExactSignatures)
			}
		})
	}
}

// An ordinary factory and its generated constructor can change together and
// hide an equal-erasure port swap. An unchanged caller invokes the hidden
// constructor directly with distinct values and compares the real storage map
// with the original JVM, independently of the Go constructor classifier.
func TestAdversarialLocalOwnedAnonymousHiddenConstructorKeepsPorts(t *testing.T) {
	source := captureActivationInput + `class CaptureTransport {static CaptureInput.View make(java.util.function.Supplier<Object> alias,java.util.function.Supplier<Object> peer,long wide){class Local{CaptureInput.View create(){return new CaptureInput.View(){public Object retained(){Object result=alias.get();if(wide!=17L||peer==alias)throw new AssertionError("capture words");return result;}};}}return new Local().create();}}
 class CaptureDriver {public static void main(String[]args)throws Exception{CaptureInput.View view=CaptureTransport.make(()->new Object(),()->new Object(),17L);java.lang.reflect.Field[] carriers=view.getClass().getDeclaredFields();if(carriers.length!=1)throw new AssertionError("one receiver carrier");carriers[0].setAccessible(true);Class<?> local=carriers[0].get(view).getClass();java.lang.reflect.Constructor<?>[] constructors=local.getDeclaredConstructors();if(constructors.length!=1)throw new AssertionError("one original constructor");java.lang.reflect.Constructor<?> ctor=constructors[0];Class<?>[] types=ctor.getParameterTypes();Object[] values=new Object[types.length];for(int i=0;i<types.length;i++){if(types[i]==java.util.function.Supplier.class){final Object marker=new Object();values[i]=(java.util.function.Supplier<Object>)()->marker;}else if(types[i]==long.class){values[i]=Long.valueOf(17L);}else throw new AssertionError("unexpected hidden word");}ctor.setAccessible(true);Object receiver=ctor.newInstance(values);java.util.TreeMap<String,String> ports=new java.util.TreeMap<>();for(java.lang.reflect.Field field:local.getDeclaredFields()){if(!field.isSynthetic())throw new AssertionError("original capture field");field.setAccessible(true);Object value=field.get(receiver);String port=null;if(field.getType()==long.class){if(!Long.valueOf(17L).equals(value))throw new AssertionError("wide word");port="wide:17";}else{for(int i=0;i<values.length;i++)if(value==values[i]){if(port!=null)throw new AssertionError("ambiguous port");port="arg:"+i;}}if(port==null)throw new AssertionError("missing original port");ports.put(field.getName(),port);}if(ports.size()!=3)throw new AssertionError("three capture ports");System.out.println(ports);}}
 `
	_, java := t04Tools(t)
	for _, profile := range []string{"modern", "native8"} {
		t.Run(profile, func(t *testing.T) {
			compile := func(debug string) map[string][]byte {
				if profile == "native8" {
					return nativePrivateEnumCompile(t, source, "CaptureTransport", debug)
				}
				return nativeCompileSourceReleaseClasses(t, map[string]string{"CaptureTransport.java": source}, debug, "8")
			}
			original := compile("none")
			dir := t.TempDir()
			for name, raw := range original {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, dir, "CaptureDriver")
			// Reuse only the already compiled original; generated source and rebuilt
			// binaries still travel through the complete production family pipeline.
			cachedCompile := func(debug string) map[string][]byte {
				if debug == "none" {
					return original
				}
				return compile(debug)
			}
			if profile == "native8" {
				testNativeIndependentCompilerFamilyFixture(t, cachedCompile, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"CaptureTransport"}, "CaptureDriver", want, nil, nativeLexicalExactSignatures)
			} else {
				javac, _ := t04Tools(t)
				testNativeIndependentCompilerFamilyFixture(t, cachedCompile, ModernJavac, javac, []string{"CaptureTransport"}, "CaptureDriver", want, nil, nativeLexicalExactSignatures)
			}
		})
	}
}

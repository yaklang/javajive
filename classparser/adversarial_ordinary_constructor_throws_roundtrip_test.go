package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A throws-only inference variable has no argument lower bound. javac infers
// RuntimeException, but the constructor can still throw the *same* IOException,
// Error, or null supplied through a separate Throwable argument. The unchanged
// binary caller observes that distinction, NEW initialization before argument
// effects, wide operand slots, escaped receiver identity, and the method table.
const ordinaryConstructorThrowsRoundTripFixture = `
class NewInferenceState {
 static String trace=""; static Object published;
 static final java.io.IOException io=new java.io.IOException("same-io");
 static final RuntimeException runtime=new IllegalStateException("same-runtime");
 static final Error error=new AssertionError("same-error");
 static boolean argument(boolean fail,int phase){trace+="A";if(phase==1)throw runtime;return fail;}
 static long wide(long value){trace+="W";return value*3+1;}
}
interface NewInferenceProduct {Object value();long wide();long bits();}
class NewInferenceLoose implements NewInferenceProduct {
 static{NewInferenceState.trace+="I";}
 final Object stored;final long number,encoded;
 @SuppressWarnings("unchecked") <WITNESS extends Throwable>NewInferenceLoose(boolean fail,Throwable value,long wide,double bits)throws WITNESS{
  NewInferenceState.trace+="C";NewInferenceState.published=this;stored=value;number=wide;encoded=Double.doubleToRawLongBits(bits);
  if(fail)throw(WITNESS)value;NewInferenceState.trace+="E";
 }
 public Object value(){return stored;} public long wide(){return number;} public long bits(){return encoded;}
}
class NewInferenceException implements NewInferenceProduct {
 static{NewInferenceState.trace+="X";}
 final Object stored;final long number,encoded;
 @SuppressWarnings("unchecked") <WITNESS extends Exception>NewInferenceException(boolean fail,Exception value,long wide,double bits)throws WITNESS{
  NewInferenceState.trace+="C";NewInferenceState.published=this;stored=value;number=wide;encoded=Double.doubleToRawLongBits(bits);
  if(fail)throw(WITNESS)value;NewInferenceState.trace+="E";
 }
 public Object value(){return stored;} public long wide(){return number;} public long bits(){return encoded;}
}
class NewInferenceMixed implements NewInferenceProduct {
 static{NewInferenceState.trace+="M";}
 final Object stored;final long number,encoded;
 <WITNESS extends Exception>NewInferenceMixed(boolean fail,java.io.IOException value,long wide,double bits)throws WITNESS,java.io.IOException{
  NewInferenceState.trace+="C";NewInferenceState.published=this;stored=value;number=wide;encoded=Double.doubleToRawLongBits(bits);
  if(fail)throw value;NewInferenceState.trace+="E";
 }
 public Object value(){return stored;} public long wide(){return number;} public long bits(){return encoded;}
}
class NewInferenceFactory {
 static Object loose(boolean fail,Throwable value,int phase,long wide,double bits){NewInferenceState.trace+="F";return new NewInferenceLoose(NewInferenceState.argument(fail,phase),value,NewInferenceState.wide(wide),bits);}
 static Object exception(boolean fail,Exception value,int phase,long wide,double bits){NewInferenceState.trace+="F";return new NewInferenceException(NewInferenceState.argument(fail,phase),value,NewInferenceState.wide(wide),bits);}
 static Object mixed(boolean fail,java.io.IOException value,int phase,long wide,double bits)throws java.io.IOException{NewInferenceState.trace+="F";return new NewInferenceMixed(NewInferenceState.argument(fail,phase),value,NewInferenceState.wide(wide),bits);}
 static Object broad(boolean fail,java.io.IOException value,int phase,long wide,double bits)throws Exception{NewInferenceState.trace+="F";return new NewInferenceMixed(NewInferenceState.argument(fail,phase),value,NewInferenceState.wide(wide),bits);}
 static String literal(){return "/*jdec-ordinary-new:literal*/ new int[]{1,2}";}
}
class NewInferenceDriver {
 static Object invoke(int kind,boolean fail,Throwable value,int phase,long wide,double bits)throws Exception{
  if(kind==0)return NewInferenceFactory.loose(fail,value,phase,wide,bits);
  if(kind==1)return NewInferenceFactory.exception(fail,(Exception)value,phase,wide,bits);
  if(kind==2)return NewInferenceFactory.mixed(fail,(java.io.IOException)value,phase,wide,bits);
  return NewInferenceFactory.broad(fail,(java.io.IOException)value,phase,wide,bits);
 }
 public static void main(String[]args){int rows=0;
  for(int kind=0;kind<4;kind++){
   NewInferenceState.trace="";NewInferenceState.published=null;Throwable argumentFailure=null;
   try{invoke(kind,false,null,1,Long.MIN_VALUE,-0.0);}catch(Throwable actual){argumentFailure=actual;}
   String initial=kind==0?"I":kind==1?"X":kind==2?"M":"";
   if(argumentFailure!=NewInferenceState.runtime||NewInferenceState.published!=null||!NewInferenceState.trace.equals("F"+initial+"A"))throw new AssertionError("NEW initialization/argument failure order:"+NewInferenceState.trace,argumentFailure);
   rows++;
   Throwable[] values=kind==0?new Throwable[]{NewInferenceState.io,NewInferenceState.runtime,NewInferenceState.error,null}:kind==1?new Throwable[]{NewInferenceState.io,NewInferenceState.runtime,null}:new Throwable[]{NewInferenceState.io,null};
   for(boolean fail:new boolean[]{false,true})for(int index=0;index<values.length;index++){
    Throwable value=values[index],caught=null;Object result=null;NewInferenceState.trace="";NewInferenceState.published=null;
    long number=index%2==0?Long.MAX_VALUE:Long.MIN_VALUE;
    long encoded=index%3==0?0x8000000000000000L:index%3==1?0x7ff0000000000000L:0x7ff8000000000042L;
    try{result=invoke(kind,fail,value,0,number,Double.longBitsToDouble(encoded));}catch(Throwable actual){caught=actual;}
    if(fail?(value==null?!(caught instanceof NullPointerException):caught!=value):caught!=null)throw new AssertionError("exception identity",caught);
    if(!NewInferenceState.trace.equals(fail?"FAWC":"FAWCE")||NewInferenceState.published==null)throw new AssertionError("argument/constructor effects:"+NewInferenceState.trace);
    if(!fail){NewInferenceProduct product=(NewInferenceProduct)result;if(result!=NewInferenceState.published||product.value()!=value||product.wide()!=number*3+1||product.bits()!=encoded)throw new AssertionError("receiver/value/wide/IEEE bits");}
    rows++;
   }
  }
  if(!NewInferenceFactory.literal().equals("/*jdec-ordinary-new:literal*/ new int[]{1,2}"))throw new AssertionError("literal changed");
  System.out.println(rows+":new:identity:init:wide:methods="+NewInferenceFactory.class.getDeclaredMethods().length);
 }
}`

func TestAdversarialOrdinaryConstructorThrowsOriginalBinaryCaller(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC is required for the native8 compiler oracle")
				}
				if out, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(out), "javac 1.8.") {
					t.Fatalf("native8 needs the actual compiler: %v / %s", err, out)
				}
			}
			for _, owner := range []string{"NewInferenceFactory", "AlternateInferenceEmission"} {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(ordinaryConstructorThrowsRoundTripFixture, "NewInferenceFactory", owner)
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if profile == NativeJavac8 {
								files = nativePrivateEnumCompile(t, fixture, owner, debug)
							} else {
								files = nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
							}
							write := func(dir, name string, raw []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
									t.Fatal(err)
								}
							}
							before := map[string][]byte{}
							original := t.TempDir()
							for name, raw := range files {
								before[name] = bytes.Clone(raw)
								write(original, name, raw)
							}
							const want = "26:new:identity:init:wide:methods=5\n"
							if got := t04RunJava(t, java, original, "NewInferenceDriver"); got != want {
								t.Fatalf("independent original oracle: %q", got)
							}
							t.Log("independent original JVM: " + strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{
										Mode: mode, TargetSourceVersion: 8, SourceCompiler: profile, EnvSnapshot: map[string]string{},
										Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known },
									})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("source status=%s: %v\n%s", result.Status, err, result.Source)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name != owner+".class" {
											write(out, name, raw)
										}
									}
									write(out, owner+".java", []byte(result.Source))
									args := []string{"-proc:none", "--release", "8"}
									if profile == NativeJavac8 {
										args = []string{"-proc:none", "-source", "8", "-target", "8"}
									}
									args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
									if data, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
										t.Fatalf("rebuild without original target: %v\n%s\n%s", err, data, result.Source)
									}
									if got := t04RunJava(t, java, out, "NewInferenceDriver"); got != want {
										t.Fatalf("unchanged original binary caller: %q want %q\n%s", got, want, result.Source)
									}
									t.Log("fully rebuilt target JVM: " + strings.TrimSpace(want))
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("complete declaration ABI changed:\n%s\n%s", a, b)
									}
									for name, raw := range files {
										if !bytes.Equal(raw, before[name]) {
											t.Fatal("original dependency/Driver changed", name)
										}
									}
									if strings.Contains(result.Source, "jdec$rethrow$") || strings.Count(result.Source, "jdec-ordinary-new:") != 1 {
										t.Fatal("helper or live publication annotation escaped")
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A source keyword is application data, not permission to erase type views.
// Number may be a real CHECKCAST receiver or the erased overload selected by
// bytecode even when its operand has already been checked as Integer.
const numericCastRecoveryFixture = `
class NumericState{static String trace="";static final RuntimeException fault=new IllegalStateException("same");}
public class NumericViews{
 public static String select(Object value){return choose((Number)(Integer)value);}
 public static String choose(Number value){NumericState.trace+="N";return "number";}
 public static String choose(Integer value){NumericState.trace+="I";return "integer";}
 public static double read(Object value){return((Number)value).doubleValue();}
 public static String data(){return "assertj (Number)(payload)";}
}
class NumericThrow extends Number{
 public int intValue(){throw new AssertionError("wrong conversion");}
 public long longValue(){throw new AssertionError("wrong conversion");}
 public float floatValue(){throw new AssertionError("wrong conversion");}
 public double doubleValue(){NumericState.trace+="D";throw NumericState.fault;}
}
class NumericDriver{
 public static void main(String[]args){int rows=0;
  for(Object value:new Object[]{null,Integer.valueOf(7),Long.valueOf(9),"wrong",Double.valueOf(2.0)}){
   NumericState.trace="";Throwable actual=null;String result=null;
   try{result=NumericViews.select(value);}catch(Throwable e){actual=e;}
   if(value==null||value instanceof Integer){
    if(actual!=null||!"number".equals(result)||!"N".equals(NumericState.trace))throw new AssertionError("selected Number overload",actual);
   }else if(!(actual instanceof ClassCastException)||!"".equals(NumericState.trace))throw new AssertionError("original Integer check before invocation",actual);
   rows++;
  }
  for(Number value:new Number[]{Integer.valueOf(Integer.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Float.valueOf(Float.NaN),Double.valueOf(Double.NaN),Double.valueOf(-0.0),Double.valueOf(Double.POSITIVE_INFINITY),Float.valueOf(Float.NEGATIVE_INFINITY),new java.math.BigInteger("1234567890123456789012345"),new java.math.BigDecimal("-1.125")}){
   long expected=Double.doubleToRawLongBits(value.doubleValue());NumericState.trace="";
   if(Double.doubleToRawLongBits(NumericViews.read(value))!=expected||!NumericState.trace.equals(""))throw new AssertionError("receiver/IEEE payload");rows++;
  }
  for(Object value:new Object[]{null,new Object(),new NumericThrow()}){
   NumericState.trace="";Throwable actual=null;
   try{NumericViews.read(value);}catch(Throwable e){actual=e;}
   if(value==null?!(actual instanceof NullPointerException):value instanceof NumericThrow?actual!=NumericState.fault:!(actual instanceof ClassCastException))throw new AssertionError("receiver check and exception identity",actual);
   if(!NumericState.trace.equals(value instanceof NumericThrow?"D":""))throw new AssertionError("receiver evaluation priority");rows++;
  }
  if(!NumericViews.data().equals("assertj (Number)(payload)"))throw new AssertionError("literal data changed");rows++;
  System.out.println(rows+":numeric:cast:binding:literal");
 }
}`

func TestAdversarialNumericCastViewsSurviveSourceRecovery(t *testing.T) {
	runCheckedTypeViewsRoundTrip(t, numericCastRecoveryFixture, []string{"NumericViews", "AlternateNumeric"}, "NumericDriver", "18:numeric:cast:binding:literal\n", "numeric")
}

// Compile declarations and an independent binary caller once per profile.
// Rebuild only the target; every original helper/caller byte remains unchanged.
// Type-view repairs must preserve declarations, runtime checks and call tuples,
// as well as values, exception priority, effects and application data.
func runCheckedTypeViewsRoundTrip(t *testing.T, source string, owners []string, driver, want, label string) {
	t.Helper()
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for the independent compiler")
				}
				if version, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(version), "javac 1.8.") {
					t.Fatalf("native8 needs actual javac8: %v / %s", err, version)
				}
			}
			for _, owner := range owners {
				t.Run(owner, func(t *testing.T) {
					fixture := strings.ReplaceAll(source, owners[0], owner)
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if profile == NativeJavac8 {
								files = nativePrivateEnumCompile(t, fixture, owner, debug)
							} else {
								files = nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
							}
							write := func(dir, name string, data []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							for name, raw := range files {
								write(original, name, raw)
							}
							if got := t04RunJava(t, java, original, driver); got != want {
								t.Fatalf("independent original: %q want %q", got, want)
							}
							t.Log("independent original " + label + " JVM: " + strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { data, known := files[name+".class"]; return bytes.Clone(data), known }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("source status=%s: %v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
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
									if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
										t.Fatalf("rebuild without original target: %v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, driver); got != want {
										t.Fatalf("unchanged binary caller: %q want %q\n%s", got, want, result.Source)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("declaration inventory changed:\n%s\n%s", a, b)
									}
									if a, b := arrayProducerCheckedInvocations(t, files[owner+".class"]), arrayProducerCheckedInvocations(t, rebuilt); a != b {
										t.Fatalf("original runtime checks or invocation tuples changed:\n%s\n%s", a, b)
									}
									t.Log("fully rebuilt " + label + " JVM: " + strings.TrimSpace(want))
								})
							}
						})
					}
				})
			}
		})
	}
}

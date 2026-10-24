package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const interfaceLambdaInitializerFixture = `class InitPacketEffects{static String trace="";static InitPacketContract bind(String tag,InitPacketContract value){trace+=tag;return value;}}
interface InitPacketContract{
 InitPacketContract ZERO=InitPacketEffects.bind("A",()->0L);
 InitPacketContract ONE=InitPacketEffects.bind("B",()->1L);
 long read();
 static InitPacketContract captured(long value){return ()->value;}
}
public class InitPacketDriver{public static void main(String[]args){
 if(!InitPacketEffects.trace.equals(""))throw new AssertionError("premature interface initialization");
 InitPacketContract zero=InitPacketContract.ZERO,one=InitPacketContract.ONE;
 if(!InitPacketEffects.trace.equals("AB")||zero==one||zero!=InitPacketContract.ZERO||one!=InitPacketContract.ONE)throw new AssertionError("initializer order/identity");
 int rows=0;for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE}){
  InitPacketContract result=InitPacketContract.captured(value);
  if(zero.read()!=0L||one.read()!=1L||result.read()!=value||!InitPacketEffects.trace.equals("AB"))throw new AssertionError("static/ordinary factory result");rows++;
 }System.out.println(rows+":interface:initializer:factory:order:identity");
}}`

// An interface has no source static block. Its original clinit factories must
// retain their own occurrences when assignments become field initializers;
// ordinary method factories still share the compiler's implementation counter.
func TestAdversarialInterfaceLambdaInitializerKeepsFactoryOrderAndIdentity(t *testing.T) {
	testInterfaceLambdaInitializerFamily(t, interfaceLambdaInitializerFixture, "5:interface:initializer:factory:order:identity\n")
}

func TestAdversarialInterfaceDirectLambdaInitializerKeepsCompleteMemberFamily(t *testing.T) {
	source := strings.NewReplacer(`InitPacketEffects.bind("A",()->0L)`, `()->0L`, `InitPacketEffects.bind("B",()->1L)`, `()->1L`, `trace.equals("AB")`, `trace.equals("")`).Replace(interfaceLambdaInitializerFixture)
	testInterfaceLambdaInitializerFamily(t, source, "5:interface:initializer:factory:order:identity\n")
}

func TestAdversarialInterfaceLambdaInitializerKeepsAbruptCompletionAndCause(t *testing.T) {
	source := `class InitPacketEffects{static String trace="";static final IllegalStateException failure=new IllegalStateException("same original cause");static InitPacketContract bind(InitPacketContract value){trace+="F";throw failure;}}
interface InitPacketContract{InitPacketContract FAILED=InitPacketEffects.bind(()->17L);long read();}
public class InitPacketDriver{public static void main(String[]args){Throwable first=null,second=null;Object original=InitPacketEffects.failure;
 try{Object unused=InitPacketContract.FAILED;}catch(Throwable e){first=e;}
 if(!(first instanceof ExceptionInInitializerError)||first.getCause()!=original||!InitPacketEffects.trace.equals("F"))throw new AssertionError("first initialization cause/order",first);
 try{Object unused=InitPacketContract.FAILED;}catch(Throwable e){second=e;}
 if(!(second instanceof NoClassDefFoundError)||!InitPacketEffects.trace.equals("F"))throw new AssertionError("initialization retried or cause category changed",second);
 System.out.println("interface:initializer:first-failure:identity:no-retry");}}`
	testInterfaceLambdaInitializerFamily(t, source, "interface:initializer:first-failure:identity:no-retry\n")
}

// The archive oracle above exercises lexical ownership and cleanup policies.
// Reopen the public raw-class API separately: it has explicit decompile modes,
// and the compiler must never see the original target class as a fallback.
func TestAdversarialInterfaceLambdaInitializerRawModesKeepOriginalBinaryCaller(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, interfaceLambdaInitializerFixture, "InitPacketDriver", debug, true)
					write := func(dir, name string, raw []byte) {
						t.Helper()
						if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					original := t.TempDir()
					for name, raw := range files {
						write(original, name, raw)
					}
					want := t04RunJava(t, java, original, "InitPacketDriver")
					if want != "5:interface:initializer:factory:order:identity\n" {
						t.Fatal("independent original initializer behavior", want)
					}
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							input := bytes.Clone(files["InitPacketContract.class"])
							result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) {
								raw, found := files[name+".class"]
								return bytes.Clone(raw), found
							}})
							if !bytes.Equal(input, files["InitPacketContract.class"]) || err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatalf("mode=%s status=%s error=%v diagnostics=%+v\n%s", mode, result.Status, err, result.Diagnostics, result.Source)
							}
							out := t.TempDir()
							for name, raw := range files {
								if name != "InitPacketContract.class" {
									write(out, name, raw)
								}
							}
							write(out, "InitPacketContract.java", []byte(result.Source))
							args := []string{"-proc:none", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-parameters", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, filepath.Join(out, "InitPacketContract.java"))
							if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
								t.Fatalf("compile with original target removed: %v\n%s\n%s", err, log, result.Source)
							}
							if got := t04RunJava(t, java, out, "InitPacketDriver"); got != want {
								t.Fatalf("unchanged binary caller %q want %q", got, want)
							}
							candidate, err := os.ReadFile(filepath.Join(out, "InitPacketContract.class"))
							if err != nil {
								t.Fatal(err)
							}
							if constructorInvocationThrowsDeclarations(t, input) != constructorInvocationThrowsDeclarations(t, candidate) || lambdaParameterMetadataTables(t, input) != lambdaParameterMetadataTables(t, candidate) {
								t.Fatal("original full declaration/parameter family changed")
							}
						})
					}
				})
			}
		})
	}
}

func testInterfaceLambdaInitializerFamily(t *testing.T, fixture, want string) {
	t.Helper()
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, prefix := range []string{"InitPacket", "FieldFactory"} {
				t.Run(prefix, func(t *testing.T) {
					source := fixture
					source = strings.ReplaceAll(source, "InitPacket", prefix)
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
						units := map[string]string{prefix + "Driver.java": source}
						if profile == NativeJavac8 {
							return localGetterCompileJavac8(t, units, debug)
						}
						return nativeCompileSourceReleaseClasses(t, units, debug, "8")
					}, profile, compiler, []string{prefix + "Contract"}, prefix+"Driver", want, nil, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A flat physical enum still owns each constant's virtual implementation.
// The original binary driver distinguishes an omitted override from legal
// differences in source spelling, and includes a body with no declarations.
func TestAdversarialFlatEnumConstantDispatchUsesAllocationIdentity(t *testing.T) {
	flatEnumDispatchRoundTrip(t, false)
}

func TestAdversarialFlatEnumConstantAssertionsKeepLexicalStatusOwner(t *testing.T) {
	flatEnumDispatchRoundTrip(t, true)
}

func TestAdversarialFlatEnumConstructorProofSeparatesMetadataAndBodyCode(t *testing.T) {
	for _, variant := range []string{"named parameters", "generic debug", "lambda bootstrap", "legacy self row"} {
		t.Run(variant, func(t *testing.T) { flatEnumDispatchRoundTrip(t, false, variant) })
	}
}

func TestAdversarialFlatEnumNestedOptionalReferencesKeepOriginalPrivateOwner(t *testing.T) {
	flatEnumDispatchRoundTrip(t, false, "nested optional references")
}

func TestAdversarialFlatEnumPrivateReferenceKeepsCheckedImplementationInput(t *testing.T) {
	flatEnumDispatchRoundTrip(t, false, "private reference")
}

func TestAdversarialFlatEnumGenericThrowsKeepsFunctionalArgumentBinding(t *testing.T) {
	for _, variant := range []string{"generic supplier throws", "generic custom factory throws"} {
		t.Run(variant, func(t *testing.T) { flatEnumDispatchRoundTrip(t, false, variant) })
	}
}

func TestAdversarialFlatEnumGenericReceiverKeepsFunctionalThrowsBinding(t *testing.T) {
	flatEnumDispatchRoundTrip(t, false, "generic result receiver")
}

func flatEnumDispatchRoundTrip(t *testing.T, assertions bool, variants ...string) {
	t.Helper()
	javac, java := t04Tools(t)
	variant := ""
	if len(variants) == 1 {
		variant = variants[0]
	}
	for _, root := range []string{"FlatDispatchOwner", "ChangedDispatchOwner"} {
		for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(root+"/"+string(profile)+"/"+debug, func(t *testing.T) {
					source := strings.ReplaceAll(`class FlatDispatchOwner {
 public enum Mode {
  BASE(0,0.0), EMPTY(0,-0.0){}, ADD(1,0.25){public long apply(long x){FlatDispatchEffects.calls++;if(x==17)throw FlatDispatchEffects.failure;return x+step;}}, NEGATE(0,0.5){public long apply(long x){return -x;}};
  final long step; final double bias; Mode(long step,double bias){this.step=step;this.bias=bias;} public long apply(long x){return x;}
 }
}
class FlatDispatchEffects {static int calls;static final IllegalStateException failure=new IllegalStateException("original");}
class FlatDispatchDriver {public static void main(String[]args){
 int rows=0;
 for(FlatDispatchOwner.Mode m:FlatDispatchOwner.Mode.values()){
  if(m.getDeclaringClass()!=FlatDispatchOwner.Mode.class||FlatDispatchOwner.Mode.valueOf(m.name())!=m)throw new AssertionError("enum identity");
  if((m.getClass()==FlatDispatchOwner.Mode.class)!=(m==FlatDispatchOwner.Mode.BASE))throw new AssertionError("constant class identity");
  long bits=Double.doubleToRawLongBits(m.bias);long expectedBits=m==FlatDispatchOwner.Mode.BASE?0L:m==FlatDispatchOwner.Mode.EMPTY?Long.MIN_VALUE:m==FlatDispatchOwner.Mode.ADD?0x3fd0000000000000L:0x3fe0000000000000L;
  if(bits!=expectedBits)throw new AssertionError("wide signed zero binding");
  for(long x:new long[]{Long.MIN_VALUE,Long.MIN_VALUE+1,-1,0,1,Long.MAX_VALUE-1,Long.MAX_VALUE}){
   java.math.BigInteger n=java.math.BigInteger.valueOf(x);
   long want=(m==FlatDispatchOwner.Mode.ADD?n.add(java.math.BigInteger.ONE):m==FlatDispatchOwner.Mode.NEGATE?n.negate():n).longValue();
   if(m.apply(x)!=want)throw new AssertionError("virtual overflow "+m+":"+x); rows++;
  }
 }
 try{FlatDispatchOwner.Mode.ADD.apply(17);throw new AssertionError("missing exception");}catch(IllegalStateException e){if(e!=FlatDispatchEffects.failure)throw new AssertionError("exception identity");}
 if(FlatDispatchEffects.calls!=8)throw new AssertionError("virtual side effects");
 FlatDispatchOwner.Mode[] copy=FlatDispatchOwner.Mode.values();copy[0]=null;
 if(FlatDispatchOwner.Mode.values()[0]!=FlatDispatchOwner.Mode.BASE)throw new AssertionError("array alias");
 System.out.println(rows+":flat:enum:dispatch:empty:overflow:identity");
}}
`, "FlatDispatchOwner", root)
					if variant != "" {
						probe := `int rows=0;for(` + root + `.Mode m:` + root + `.Mode.values()){if(m.getClass()==` + root + `.Mode.class)continue;for(java.lang.reflect.Constructor<?> c:m.getClass().getDeclaredConstructors()){StringBuilder row=new StringBuilder(m.name()+":"+m.getClass().getModifiers()+":"+c.getModifiers());for(java.lang.reflect.Parameter p:c.getParameters())row.append(":"+p.getName()+":"+p.isNamePresent()+":"+p.getModifiers()+":"+p.getParameterizedType().getTypeName());System.out.println(row);}}`
						source = strings.Replace(source, "int rows=0;", probe, 1)
					}
					if variant == "generic debug" {
						for _, old := range []string{"BASE(0,0.0)", "EMPTY(0,-0.0)", "ADD(1,0.25)", "NEGATE(0,0.5)"} {
							source = strings.Replace(source, old, strings.TrimSuffix(old, ")")+",String.class)", 1)
						}
						source = strings.Replace(source, "Mode(long step,double bias)", "Mode(long step,double bias,Class<?> kind)", 1)
					}
					if variant == "nested optional references" {
						source = strings.Replace(source, "return x+step;", `return x+java.util.stream.Stream.of(ADD,EMPTY,BASE).map(Mode::secret).flatMap(optional -> optional.map(java.util.stream.Stream::of).orElse(java.util.stream.Stream.empty())).findFirst().orElseThrow(() -> new IllegalStateException("unreachable")).longValue();`, 1)
						source = strings.Replace(source, "final long step;", `private java.util.Optional<Long> secret(){return java.util.Optional.of(Long.valueOf(step));}final long step;`, 1)
					}
					if variant == "private reference" {
						source = strings.Replace(source, "return x+step;", `return x+((java.util.function.Function<Mode,Long>)Mode::secret).apply(ADD).longValue();`, 1)
						source = strings.Replace(source, "final long step;", `private Long secret(){return Long.valueOf(step);}final long step;`, 1)
					}
					if variant == "generic supplier throws" || variant == "generic custom factory throws" {
						factory, invoke := "java.util.function.Supplier", "get"
						if variant == "generic custom factory throws" {
							factory, invoke = "FlatDispatchFaultFactory", "create"
							source += "\ninterface FlatDispatchFaultFactory<E extends Throwable> { E create(); }\n"
						}
						source = strings.Replace(source, "if(x==17)throw FlatDispatchEffects.failure;return x+step;", "return x+require(x,step,()->FlatDispatchEffects.failure);", 1)
						source = strings.Replace(source, "final long step;", `static <E extends Throwable> long require(long x,long value,`+factory+`<? extends E> failure) throws E {if(x==17)throw failure.`+invoke+`();return value;}final long step;`, 1)
					}
					if variant == "generic result receiver" {
						source += "\nclass FlatDispatchResult<T> { final T value; FlatDispatchResult(T value){this.value=value;} <X extends Throwable> T retrieve(java.util.function.Supplier<? extends X> factory)throws X {if(value==null)throw factory.get();return value;} }\n"
						source = strings.Replace(source, "if(x==17)throw FlatDispatchEffects.failure;return x+step;", "return x+new FlatDispatchResult<Long>(x==17?null:Long.valueOf(step)).retrieve(()->FlatDispatchEffects.failure).longValue();", 1)
					}
					if variant == "lambda bootstrap" {
						source = strings.Replace(source, "return x+step;", "java.util.function.LongUnaryOperator f = value -> value+step;return f.applyAsLong(x);", 1)
					}
					if assertions {
						source = strings.Replace(source, "FlatDispatchEffects.calls++;if(x==17)", `FlatDispatchEffects.calls++;assert x!=9 : "original assertion";if(x==17)`, 1)
						source = strings.Replace(source, "if(FlatDispatchEffects.calls!=8)", `boolean assertion=false;try{`+root+`.Mode.ADD.apply(9);}catch(AssertionError error){assertion=true;if(!"original assertion".equals(error.getMessage()))throw error;}
if(assertion!=`+root+`.class.desiredAssertionStatus())throw new AssertionError("lexical assertion owner");
if(FlatDispatchEffects.calls!=9)`, 1)
					}
					compiler := javac
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Fatal("native original compiler required")
						}
					}
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, source, root, debug, variant == "named parameters")
					if variant == "legacy self row" {
						// Legal InnerClasses metadata need not repeat the ClassFile ENUM
						// bit. Preserve the original runtime metadata independently of Code.
						for name, raw := range files {
							obj := mustParseLowering(t, raw)
							if obj.GetSupperClassName() != root+"$Mode" {
								continue
							}
							for _, attr := range obj.Attributes {
								if rows, ok := attr.(*InnerClassesAttribute); ok {
									for _, row := range rows.Classes {
										binary, _ := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
										if binary == obj.GetClassName() {
											row.InnerClassAccessFlags &= ^uint16(0x4000)
										}
									}
								}
							}
							files[name] = obj.Bytes()
						}
					}
					var originalObservations string
					run := func(t *testing.T, dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da", "-ea:" + root, "-ea:" + root + "$Mode"} {
							log, err := enumFreshOracleRun(t, java, mode, dir, "FlatDispatchDriver")
							if err != nil || !strings.HasSuffix(string(log), "28:flat:enum:dispatch:empty:overflow:identity\n") || variant == "" && string(log) != "28:flat:enum:dispatch:empty:overflow:identity\n" {
								t.Fatal("original binary oracle", err, string(log))
							}
							if originalObservations == "" {
								originalObservations = string(log)
							}
							if string(log) != originalObservations {
								t.Fatal("original metadata and behavior differ", string(log), originalObservations)
							}
						}
					}
					run(t, sourceArtifactWriteFiles(t, files))
					owner := root + "$Mode"
					owned := map[string][]byte{}
					for name, data := range files {
						if strings.HasPrefix(name, owner) {
							owned[name] = data
						}
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							if policy == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if policy == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							jar := filepath.Join(t.TempDir(), "flat-dispatch.jar")
							if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
								t.Fatal(err)
							}
							artifact, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok })
							if err != nil {
								t.Fatal(err)
							}
							defer artifact.Close()
							textBytes, err := artifact.ReadFile(owner + ".class")
							text := string(textBytes)
							if err != nil || legacySourceHasFailureMarker(text) {
								t.Fatal("complete flat enum source", err, text)
							}
							out := sourceArtifactWriteFiles(t, map[string][]byte{root + ".class": files[root+".class"], "FlatDispatchDriver.class": files["FlatDispatchDriver.class"], "FlatDispatchEffects.class": files["FlatDispatchEffects.class"]})
							for _, helperName := range []string{"FlatDispatchFaultFactory.class", "FlatDispatchResult.class"} {
								if helper, found := files[helperName]; found {
									if err := os.WriteFile(filepath.Join(out, helperName), helper, 0600); err != nil {
										t.Fatal(err)
									}
								}
							}
							path := filepath.Join(out, owner+".java")
							if err := os.WriteFile(path, []byte(text), 0600); err != nil {
								t.Fatal(err)
							}
							if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
								t.Fatal("fresh enum without owned original classes", err, string(log), text)
							}
							for name := range owned {
								file := filepath.Join(out, name)
								fresh, err := os.ReadFile(file)
								if err != nil {
									t.Fatal("missing constant class", name, err, text)
								}
								done, err := artifact.FinishClass(name, fresh)
								if err != nil {
									for _, raw := range [][]byte{owned[name], fresh} {
										object := mustParseLowering(t, raw)
										for _, method := range object.Methods {
											n, _ := sourceBridgeUTF8(object, method.NameIndex)
											d, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
											t.Logf("%s method %s%s flags=%04x", name, n, d, method.AccessFlags)
										}
									}
									t.Log(text)
									t.Fatal("constant family completion", name, err)
								}
								if err := os.WriteFile(file, done, 0600); err != nil {
									t.Fatal(err)
								}
							}
							run(t, out)
						})
					}
				})
			}
		}
	}
}

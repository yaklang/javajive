package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Renaming a private implementation, or reordering ordinary method_info
// entries, preserves the original arithmetic and linkage. Neither proves that
// javac will regenerate the same numbered declarations in the emitted order.
func TestAdversarialNativeLambdaParameterIdentityUsesFinalSourceOrder(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original implementation identity oracle")
	}
	_, java := t04Tools(t)
	for _, owner := range []string{"UncapturedLambdaNames", "AlternateLambdaParameters"} {
		t.Run(owner, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					fixture := strings.Replace(uncapturedLambdaParameterFixture, "public class UncapturedLambdaNames{", `public class UncapturedLambdaNames{
 public static int initializations;public static final int stamp=mark();public static final String literal="(int value) -> value";
 private static int mark(){initializations++;return 7;}`, 1)
					// Reflection reads the rebuilt field itself. A direct access to
					// a static final String would be inlined into the original caller.
					fixture = strings.Replace(fixture, "int rows=0;", `if(UncapturedLambdaNames.initializations!=1||UncapturedLambdaNames.stamp!=7||!((String)UncapturedLambdaNames.class.getDeclaredField("literal").get(null)).equals("(int value) -> value"))throw new AssertionError("initializer order or literal data");int rows=0;`, 1)
					files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, strings.ReplaceAll(fixture, "UncapturedLambdaNames", owner), owner, debug)
					write := func(target []byte) string {
						t.Helper()
						dir := t.TempDir()
						for name, raw := range files {
							if name == owner+".class" {
								raw = target
							}
							if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						return dir
					}
					unmodified := t04RunJava(t, java, write(files[owner+".class"]), "LambdaParameterDriver")
					t.Log("independent unmodified identity original JVM:", strings.TrimSpace(unmodified))
					for _, variant := range []string{"renamed ordinal", "swapped ordinals", "factory declaration order"} {
						t.Run(variant, func(t *testing.T) {
							object, err := Parse(bytes.Clone(files[owner+".class"]))
							if err != nil {
								t.Fatal(err)
							}
							unary, pinned := -1, -1
							for i, method := range object.Methods {
								name, _ := sourceBridgeUTF8(object, method.NameIndex)
								if name == "unary" {
									unary = i
								} else if name == "pinned" {
									pinned = i
								}
							}
							if unary < 0 || pinned < 0 || len(object.Methods) <= 8 {
								t.Fatal("independent original factory/declaration count")
							}
							if variant == "factory declaration order" {
								object.Methods[unary], object.Methods[pinned] = object.Methods[pinned], object.Methods[unary]
							} else {
								names := map[string]string{"lambda$unary$0": "lambda$unary$8"}
								if variant == "swapped ordinals" {
									names = map[string]string{"lambda$unary$0": "lambda$unary$6", "lambda$identity$6": "lambda$identity$0"}
								}
								changed := 0
								for _, entry := range object.ConstantPool {
									if text, ok := entry.(*ConstantUtf8Info); ok {
										if replacement, found := names[text.Value]; found {
											text.Value = replacement
											changed++
										}
									}
								}
								if changed != len(names) {
									t.Fatal("original shared UTF8 references changed unexpectedly", changed)
								}
							}
							input := object.Bytes()
							original := t04RunJava(t, java, write(input), "LambdaParameterDriver")
							if !strings.HasPrefix(original, "140:uncaptured:lambda:parameters:") || (original == unmodified) != (variant == "factory declaration order") {
								t.Fatal("original arithmetic/independent declaration observation", original, unmodified)
							}
							t.Log("independent original identity JVM:", strings.TrimSpace(original))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									before := bytes.Clone(input)
									result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) {
										if name == owner {
											return bytes.Clone(input), true
										}
										data, known := files[name+".class"]
										return bytes.Clone(data), known
									}})
									if !bytes.Equal(input, before) {
										t.Fatal("changed original input while scheduling declarations")
									}
									if variant != "renamed ordinal" {
										if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
											t.Fatalf("regenerable declaration order status=%s err=%v diagnostics=%+v", result.Status, err, result.Diagnostics)
										}
										rebuilt := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, result.Source, owner, debug)
										raw, found := rebuilt[owner+".class"]
										if !found || len(rebuilt) != 1 {
											t.Fatal("scheduled target class inventory changed")
										}
										if got := t04RunJava(t, java, write(raw), "LambdaParameterDriver"); got != original {
											t.Fatalf("scheduled unchanged binary caller: %q want %q", got, original)
										}
										if constructorInvocationThrowsDeclarations(t, input) != constructorInvocationThrowsDeclarations(t, raw) || lambdaParameterMetadataTables(t, input) != lambdaParameterMetadataTables(t, raw) {
											t.Fatal("scheduled declaration identity/parameter metadata changed")
										}
										t.Log("fully rebuilt scheduled identity JVM:", strings.TrimSpace(original))
										return
									}
									if result.Status != "unsupported" && result.Status != "partial" {
										t.Fatalf("unrepresentable identity status=%s err=%v diagnostics=%+v", result.Status, err, result.Diagnostics)
									}
									limitation := false
									for _, diagnostic := range result.Diagnostics {
										limitation = limitation || diagnostic.Code == "bootstrap_unknown"
										if diagnostic.Code == "bootstrap_arg_mismatch" {
											t.Fatal("valid original renamed/reordered class is not malformed bootstrap data")
										}
									}
									if !limitation {
										t.Fatal("missing compiler regeneration limitation", result.Diagnostics)
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

func TestAdversarialNativeLambdaParameterIdentitySchedulesWholeFactoryBlocks(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for independent factory block oracle")
	}
	_, java := t04Tools(t)
	const fixture = `import java.util.function.IntUnaryOperator;import java.math.BigInteger;
public class FactoryBlocks {
 public static int trace;
 private static int mark(int phase){trace=trace*10+phase;return phase;}
 public static IntUnaryOperator[] pair(){mark(1);IntUnaryOperator[] result={(final int first)->first+1,(int second)->second-2};mark(2);return result;}
 public static IntUnaryOperator tail(){mark(3);return(int other)->other^5;}
}
class FactoryBlocksDriver {
 public static void main(String[]args)throws Exception{int rows=0;
  for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
   FactoryBlocks.trace=0;IntUnaryOperator[] pair=FactoryBlocks.pair();IntUnaryOperator tail=FactoryBlocks.tail();
   if(pair.length!=2||FactoryBlocks.trace!=123)throw new AssertionError("factory effects or array shape");
   BigInteger input=BigInteger.valueOf(value);
   if(pair[0].applyAsInt(value)!=input.add(BigInteger.ONE).intValue()||pair[1].applyAsInt(value)!=input.subtract(BigInteger.valueOf(2)).intValue()||tail.applyAsInt(value)!=input.xor(BigInteger.valueOf(5)).intValue())throw new AssertionError("factory block arithmetic");rows+=3;
   if(pair==FactoryBlocks.pair()||FactoryBlocks.trace!=12312)throw new AssertionError("array allocation identity or duplicated effects");
  }
  String[] names={"lambda$pair$0","lambda$pair$1","lambda$tail$2"};String[] parameters={"first","second","other"};
  for(int i=0;i<3;i++){java.lang.reflect.Method method=FactoryBlocks.class.getDeclaredMethod(names[i],int.class);java.lang.reflect.Parameter parameter=method.getParameters()[0];if(!method.isSynthetic()||!parameter.isNamePresent()||!parameter.getName().equals(parameters[i])||parameter.getModifiers()!=(i==0?0x1010:0x1000))throw new AssertionError("physical implementation registration/metadata");}
  System.out.println(rows+":lambda:factory:blocks:array");
 }
}`
	for _, owner := range []string{"FactoryBlocks", "AlternateFactoryBlocks"} {
		t.Run(owner, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, strings.ReplaceAll(fixture, "FactoryBlocks", owner), owner, debug)
					object, err := Parse(bytes.Clone(files[owner+".class"]))
					if err != nil {
						t.Fatal(err)
					}
					pair, tail := -1, -1
					for i, method := range object.Methods {
						name, _ := sourceBridgeUTF8(object, method.NameIndex)
						if name == "pair" {
							pair = i
						} else if name == "tail" {
							tail = i
						}
					}
					if pair < 0 || tail < 0 {
						t.Fatal("original factory blocks")
					}
					object.Methods[pair], object.Methods[tail] = object.Methods[tail], object.Methods[pair]
					input := object.Bytes()
					write := func(target []byte) string {
						t.Helper()
						dir := t.TempDir()
						for name, raw := range files {
							if name == owner+".class" {
								raw = target
							}
							if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						return dir
					}
					const want = "15:lambda:factory:blocks:array\n"
					if got := t04RunJava(t, java, write(input), owner+"Driver"); got != want {
						t.Fatal("independent original factory blocks", got)
					}
					t.Log("independent factory block original JVM:", strings.TrimSpace(want))
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							before := bytes.Clone(input)
							result, err := DecompileWithOptions(input, DecompileOptions{Mode: mode, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) {
								if name == owner {
									return bytes.Clone(input), true
								}
								data, known := files[name+".class"]
								return bytes.Clone(data), known
							}})
							if err != nil || result.Status != "complete" || !bytes.Equal(input, before) {
								t.Fatalf("factory block scheduling status=%s err=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
							}
							rebuilt := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, result.Source, owner, debug)
							raw, found := rebuilt[owner+".class"]
							if !found || len(rebuilt) != 1 {
								t.Fatal("factory block target class inventory changed")
							}
							if got := t04RunJava(t, java, write(raw), owner+"Driver"); got != want {
								t.Fatal("unchanged factory block binary caller", got)
							}
							if constructorInvocationThrowsDeclarations(t, input) != constructorInvocationThrowsDeclarations(t, raw) || lambdaParameterMetadataTables(t, input) != lambdaParameterMetadataTables(t, raw) || arrayProducerCheckedInvocations(t, input) != arrayProducerCheckedInvocations(t, raw) {
								t.Fatal("factory block declarations, metadata or ordered invocation tuples changed")
							}
							t.Log("fully rebuilt factory block JVM:", strings.TrimSpace(want))
						})
					}
				})
			}
		})
	}
}

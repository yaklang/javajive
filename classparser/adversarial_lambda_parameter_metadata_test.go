package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Parameter metadata on an implementation method remains observable through
// reflection. A source arrow must regenerate the original names and flags,
// including an unused final formal and wide descriptor parameters. Captured
// operands need a separate source-binder proof; this fixture has none.
const uncapturedLambdaParameterFixture = `
import java.util.function.*;
public class UncapturedLambdaNames{
 public static IntUnaryOperator unary(int l0){return(int value)->value+1;}
 public static IntUnaryOperator pinned(){return(final int item)->item+7;}
 public static IntUnaryOperator unused(){return(final int ignored)->5;}
 public static IntBinaryOperator binary(){return(int first,int second)->first^second;}
 public static LongBinaryOperator wide(){return(final long first,long second)->{long result=first^second;return result;};}
 public static DoubleBinaryOperator fractional(){return(double left,final double right)->left/right;}
 public static Function<Object,Object> identity(){return(Object μ)->μ;}
}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  int rows=0;int[] ints={Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE};
  for(int a:ints){
   if(UncapturedLambdaNames.unary(17).applyAsInt(a)!=a+1||UncapturedLambdaNames.pinned().applyAsInt(a)!=a+7||UncapturedLambdaNames.unused().applyAsInt(a)!=5)throw new AssertionError("unary/final/unused formal");rows++;
   for(int b:ints){if(UncapturedLambdaNames.binary().applyAsInt(a,b)!=(a^b))throw new AssertionError("binary parameter identities");rows++;}
  }
  long[] longs={Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE};
  for(long a:longs)for(long b:longs){if(UncapturedLambdaNames.wide().applyAsLong(a,b)!=(a^b))throw new AssertionError("wide parameter words");rows++;}
  double[] doubles={Double.NEGATIVE_INFINITY,-Double.MAX_VALUE,-1.0,-0.0,0.0,1.0,Double.MAX_VALUE,Double.POSITIVE_INFINITY,Double.NaN};
  for(double a:doubles)for(double b:doubles){if(Double.doubleToLongBits(UncapturedLambdaNames.fractional().applyAsDouble(a,b))!=Double.doubleToLongBits(a/b))throw new AssertionError("floating parameter identities");rows++;}
  for(Object value:new Object[]{null,new Object(),"same",Integer.valueOf(7)}){if(UncapturedLambdaNames.identity().apply(value)!=value)throw new AssertionError("reference parameter identity");rows++;}
  java.util.List<String> declarations=new java.util.ArrayList<String>();
  for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());
   for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());
   declarations.add(row.toString());
  }
  java.util.Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");
  for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));
  System.out.println(rows+":uncaptured:lambda:parameters:"+java.util.Base64.getEncoder().encodeToString(hash.digest()));
 }
}`

func TestAdversarialUncapturedLambdaParameterMetadataRoundTrip(t *testing.T) {
	testUncapturedLambdaParameterMetadataRoundTrip(t, uncapturedLambdaParameterFixture, 7, "140")
}

func TestAdversarialUncapturedLambdaInterfaceParameterMetadataRoundTrip(t *testing.T) {
	source := strings.Replace(uncapturedLambdaParameterFixture, "public class UncapturedLambdaNames", "public interface UncapturedLambdaNames", 1)
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 7, "140")
}

func TestAdversarialUncapturedLambdaGenericParameterMetadataRoundTrip(t *testing.T) {
	const source = `import java.util.function.*;import java.util.Comparator;
public class UncapturedLambdaNames<T>{
 public <T> Function<T,T> identity(){return(final T item)->item;}
 public <T extends Number> Function<T,T> bounded(){return(T item)->item;}
 public <T extends Number> Function<T,T> finalBounded(){return(final T item)->item;}
 public <T> Comparator<T> comparator(){return(final T first,final T second)->first==second?0:first==null?-1:second==null?1:Integer.compare(System.identityHashCode(first),System.identityHashCode(second));}
}
class LambdaParameterDriver{
 public static void main(String[]args)throws Exception{
  int rows=0;UncapturedLambdaNames<String> target=new UncapturedLambdaNames<String>();
  Object[] values={null,new Object(),"same",Integer.valueOf(7),Long.valueOf(11)};
  for(Object value:values){if(target.<Object>identity().apply(value)!=value)throw new AssertionError("method binder borrowed class String binder");rows++;}
  for(Number value:new Number[]{null,Integer.valueOf(7),Long.valueOf(11),Double.valueOf(-0.0),new java.math.BigDecimal("1.25")}){
   if(target.<Number>bounded().apply(value)!=value||target.<Number>finalBounded().apply(value)!=value)throw new AssertionError("bounded method identity/finality");rows++;
  }
  Comparator<Object> order=target.<Object>comparator();
  for(Object a:values)for(Object b:values){int expected=a==b?0:a==null?-1:b==null?1:Integer.compare(System.identityHashCode(a),System.identityHashCode(b));if(order.compare(a,b)!=expected)throw new AssertionError("generic comparator binding/null priority");rows++;}
  java.util.List<String> declarations=new java.util.ArrayList<String>();
  for(java.lang.reflect.Method method:UncapturedLambdaNames.class.getDeclaredMethods()){
   StringBuilder row=new StringBuilder(method.getName()).append(':').append(method.getReturnType().getName()).append(':').append(method.getModifiers());
   for(java.lang.reflect.Parameter parameter:method.getParameters())row.append('/').append(parameter.getType().getName()).append(':').append(parameter.getName()).append(':').append(parameter.getModifiers()).append(':').append(parameter.isNamePresent());
   declarations.add(row.toString());
  }
  java.util.Collections.sort(declarations);java.security.MessageDigest hash=java.security.MessageDigest.getInstance("SHA-256");for(String declaration:declarations)hash.update((declaration+"\n").getBytes("UTF-8"));
  System.out.println(rows+":uncaptured:lambda:parameters:"+java.util.Base64.getEncoder().encodeToString(hash.digest()));
 }
}`
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 4, "35")
}

// A valid classfile may carry MethodParameters on selected methods only.
// Native javac8 cannot regenerate that choice with its class-wide compiler
// flag. In particular, preserving the synthetic tables must not invent a
// parameter name on an unrelated ordinary declaration.
func TestAdversarialUncapturedLambdaParameterCompilerFlagCannotInventNames(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for independent metadata-presence oracle")
	}
	_, java := t04Tools(t)
	for _, owner := range []string{"UncapturedLambdaNames", "AlternateLambdaParameters"} {
		t.Run(owner, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, strings.ReplaceAll(uncapturedLambdaParameterFixture, "UncapturedLambdaNames", owner), owner, debug)
					writeWorld := func() string {
						t.Helper()
						dir := t.TempDir()
						for name, data := range files {
							if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
								t.Fatal(err)
							}
						}
						return dir
					}
					unmodified := t04RunJava(t, java, writeWorld(), "LambdaParameterDriver")
					t.Log("independent all-named original lambda JVM:", strings.TrimSpace(unmodified))
					object, err := Parse(bytes.Clone(files[owner+".class"]))
					if err != nil {
						t.Fatal(err)
					}
					ordinary, table := lambdaParameterUnrelatedOrdinaryTable(t, object)
					removeLambdaParameterTestTable(ordinary, table)
					files[owner+".class"] = object.Bytes()
					original := t04RunJava(t, java, writeWorld(), "LambdaParameterDriver")
					if original == unmodified || !strings.HasPrefix(original, "140:uncaptured:lambda:parameters:") {
						t.Fatal("original metadata absence was not independently observed", original, unmodified)
					}
					t.Log("independent selectively named original lambda JVM:", strings.TrimSpace(original))
					before := bytes.Clone(files[owner+".class"])
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, err := DecompileWithOptions(bytes.Clone(before), DecompileOptions{Mode: mode, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) {
								data, found := files[name+".class"]
								return bytes.Clone(data), found
							}})
							if err != nil || result.Status == "complete" {
								t.Fatalf("unrepresentable parameter presence claimed complete: status=%s err=%v diagnostics=%+v", result.Status, err, result.Diagnostics)
							}
							unsupported := false
							for _, diagnostic := range result.Diagnostics {
								unsupported = unsupported || diagnostic.Code == "bootstrap_unknown"
								if diagnostic.Code == "bootstrap_arg_mismatch" {
									t.Fatal("valid original metadata misclassified as malformed bootstrap", diagnostic)
								}
							}
							if !unsupported || !bytes.Equal(files[owner+".class"], before) {
								t.Fatal("missing source limitation or changed original input", result.Diagnostics)
							}
						})
					}
				})
			}
		})
	}
}

func lambdaParameterUnrelatedOrdinaryTable(t *testing.T, object *ClassObject) (*MemberInfo, *UnparsedAttribute) {
	t.Helper()
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "unary" {
			for _, attribute := range method.Attributes {
				if table, ok := attribute.(*UnparsedAttribute); ok && table.Name == "MethodParameters" {
					return method, table
				}
			}
		}
	}
	t.Fatal("original ordinary fixture declaration must have its parameter table")
	return nil, nil
}

func removeLambdaParameterTestTable(method *MemberInfo, table *UnparsedAttribute) {
	for i, attribute := range method.Attributes {
		if attribute == table {
			method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
			return
		}
	}
}

func testUncapturedLambdaParameterMetadataRoundTrip(t *testing.T, source string, implementations int, states string, parameterFlags ...bool) {
	t.Helper()
	testUncapturedLambdaParameterMetadataRoundTripWithSources(t, source, nil, implementations, states, parameterFlags...)
}

func testUncapturedLambdaParameterMetadataRoundTripWithSources(t *testing.T, source string, sources map[string]string, implementations int, states string, parameterFlags ...bool) {
	t.Helper()
	parameters := true
	if len(parameterFlags) > 1 {
		t.Fatal("at most one original parameter-table flag")
	}
	if len(parameterFlags) == 1 {
		parameters = parameterFlags[0]
	}
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent native8 metadata oracle")
				}
			}
			for _, owner := range []string{"UncapturedLambdaNames", "AlternateLambdaParameters"} {
				t.Run(owner, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							fixture := strings.ReplaceAll(source, "UncapturedLambdaNames", owner)
							if owner == "AlternateLambdaParameters" {
								fixture = strings.NewReplacer("int value", "int var0", "value+1", "var0+1", "int item", "int var1", "item+7", "var1+7", "int ignored", "int l0").Replace(fixture)
							}
							files := compileLambdaParameterMetadataFixtureWithSources(t, compiler, profile, fixture, owner, debug, sources, parameters)
							if profile == NativeJavac8 && parameters {
								if count := strings.Count(lambdaParameterMetadataTables(t, files[owner+".class"]), "lambda$"); count != implementations {
									t.Fatalf("original native8 parameter-bearing implementation declarations: %d want %d", count, implementations)
								}
							}
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
							want := t04RunJava(t, java, original, "LambdaParameterDriver")
							if !strings.HasPrefix(want, states+":uncaptured:lambda:parameters:") {
								t.Fatal("independent original runtime/metadata oracle", want)
							}
							t.Log("independent original lambda-parameters JVM:", strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { data, found := files[name+".class"]; return bytes.Clone(data), found }})
									if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("status=%s err=%v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
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
									if parameters {
										args = append(args, "-parameters")
									}
									args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
									if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
										t.Fatalf("compile without original target: %v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "LambdaParameterDriver"); got != want {
										t.Fatalf("unchanged caller/reflection: %q want %q\n%s", got, want, result.Source)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := lambdaParameterMetadataTables(t, files[owner+".class"]), lambdaParameterMetadataTables(t, rebuilt); a != b {
										t.Fatalf("physical parameter metadata changed:\n%s\n%s", a, b)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("original declaration inventory changed:\n%s\n%s", a, b)
									}
									if a, b := arrayProducerCheckedInvocations(t, files[owner+".class"]), arrayProducerCheckedInvocations(t, rebuilt); a != b {
										t.Fatalf("original checks/invocations changed:\n%s\n%s", a, b)
									}
									t.Log("fully rebuilt lambda-parameters JVM:", strings.TrimSpace(want))
								})
							}
						})
					}
				})
			}
		})
	}
}

func compileLambdaParameterMetadataFixture(t *testing.T, compiler string, profile SourceCompilerProfile, source, owner, debug string, named ...bool) map[string][]byte {
	t.Helper()
	return compileLambdaParameterMetadataFixtureWithSources(t, compiler, profile, source, owner, debug, nil, named...)
}

func compileLambdaParameterMetadataFixtureWithSources(t *testing.T, compiler string, profile SourceCompilerProfile, source, owner, debug string, sources map[string]string, named ...bool) map[string][]byte {
	t.Helper()
	return compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, compiler, profile, source, owner, debug, sources, 8, named...)
}

func compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t *testing.T, compiler string, profile SourceCompilerProfile, source, owner, debug string, sources map[string]string, release int, named ...bool) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, owner+".java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-proc:none", "-parameters", "--release", fmt.Sprint(release)}
	if profile == NativeJavac8 {
		if release < 4 || release > 8 {
			t.Fatal("native javac8 fixture requires source 4 through 8")
		}
		nativeRelease := fmt.Sprint(release)
		if release == 4 {
			nativeRelease = "1.4"
		}
		args = []string{"-proc:none", "-parameters", "-source", nativeRelease, "-target", nativeRelease}
	}
	if len(named) == 1 && !named[0] {
		args = append(args[:1], args[2:]...)
	}
	args = append(args, "-g:"+debug, "-d", dir, file)
	var names []string
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if filepath.Base(name) != name || !strings.HasSuffix(name, ".java") || name == owner+".java" {
			t.Fatal("invalid fixture unit", name)
		}
		unit := filepath.Join(dir, name)
		if err := os.WriteFile(unit, []byte(sources[name]), 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, unit)
	}
	if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
		t.Fatal("independent original compiler", err, string(log))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".class") {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = raw
		}
	}
	return files
}

func TestAdversarialUncapturedLambdaParameterCompilerProfilePreservesAbsence(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for independent compiler parameter-table oracle")
	}
	for _, originalProfile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(originalProfile), func(t *testing.T) {
			compiler, named := modern, true
			profiles := []SourceCompilerProfile{ModernJavac, NativeJavac8}
			if originalProfile == NativeJavac8 {
				compiler, named = native, false
				profiles = []SourceCompilerProfile{NativeJavac8}
			}
			for _, owner := range []string{"UncapturedLambdaNames", "AlternateLambdaParameters"} {
				t.Run(owner, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							files := compileLambdaParameterMetadataFixture(t, compiler, originalProfile, strings.ReplaceAll(uncapturedLambdaParameterFixture, "UncapturedLambdaNames", owner), owner, debug, named)
							if strings.Contains(lambdaParameterMetadataTables(t, files[owner+".class"]), "lambda$") {
								t.Fatal("original implementation parameter tables must be absent")
							}
							write := func(dir, name string, data []byte) {
								t.Helper()
								if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
									t.Fatal(err)
								}
							}
							original := t.TempDir()
							for name, data := range files {
								write(original, name, data)
							}
							want := t04RunJava(t, java, original, "LambdaParameterDriver")
							if !strings.HasPrefix(want, "140:uncaptured:lambda:parameters:") {
								t.Fatal("independent original metadata-absence oracle", want)
							}
							t.Log("independent original absent lambda metadata JVM:", strings.TrimSpace(want))
							for _, profile := range profiles {
								t.Run(string(profile), func(t *testing.T) {
									for _, mode := range []DecompileMode{Precision, Compatibility} {
										t.Run(string(mode), func(t *testing.T) {
											result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) {
												data, found := files[name+".class"]
												return bytes.Clone(data), found
											}})
											if err != nil {
												t.Fatal(err)
											}
											if originalProfile == ModernJavac && profile == NativeJavac8 {
												if result.Status == "complete" {
													t.Fatal("selected compiler would invent absent implementation parameter tables")
												}
												return
											}
											if result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
												t.Fatalf("compatible absent-table profile refused: status=%s diagnostics=%+v", result.Status, result.Diagnostics)
											}
											out := t.TempDir()
											for name, data := range files {
												if name != owner+".class" {
													write(out, name, data)
												}
											}
											write(out, owner+".java", []byte(result.Source))
											args := []string{"-proc:none", "--release", "8"}
											if profile == NativeJavac8 {
												args = []string{"-proc:none", "-source", "8", "-target", "8"}
											}
											if named {
												args = append(args, "-parameters")
											}
											args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
											if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
												t.Fatal("rebuilt compatible profile", err, string(log), result.Source)
											}
											if got := t04RunJava(t, java, out, "LambdaParameterDriver"); got != want {
												t.Fatalf("absent parameter metadata changed: got %q want %q", got, want)
											}
											data, err := os.ReadFile(filepath.Join(out, owner+".class"))
											if err != nil {
												t.Fatal(err)
											}
											if a, b := lambdaParameterMetadataTables(t, files[owner+".class"]), lambdaParameterMetadataTables(t, data); a != b {
												t.Fatalf("original table presence changed:\n%s\n%s", a, b)
											}
											t.Log("fully rebuilt absent lambda metadata JVM:", strings.TrimSpace(want))
										})
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

func lambdaParameterMetadataTables(t *testing.T, raw []byte) string {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		for _, attribute := range method.Attributes {
			table, ok := attribute.(*UnparsedAttribute)
			if !ok || table.Name != "MethodParameters" {
				continue
			}
			if len(table.Info) < 1 || len(table.Info) != 1+4*int(table.Info[0]) {
				t.Fatal("invalid original/rebuilt parameter table", name)
			}
			row := name + descriptor
			for index := 0; index < int(table.Info[0]); index++ {
				entry := table.Info[1+4*index : 5+4*index]
				nameIndex := binary.BigEndian.Uint16(entry[:2])
				text, found := sourceBridgeUTF8(object, nameIndex)
				if nameIndex != 0 && !found {
					t.Fatal("unknown original/rebuilt parameter name", name)
				}
				if nameIndex == 0 {
					row += fmt.Sprintf(" <no-name>:%04x", binary.BigEndian.Uint16(entry[2:]))
				} else {
					row += fmt.Sprintf(" %q:%04x", text, binary.BigEndian.Uint16(entry[2:]))
				}
			}
			rows = append(rows, row)
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

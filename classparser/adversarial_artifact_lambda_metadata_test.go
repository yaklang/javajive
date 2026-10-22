package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Compiler-specific helper names and MethodParameters belong to an explicit
// artifact obligation. Plain source still needs its exact compiler contract.
// The unchanged binary caller supplies the independent timing/overflow oracle.
const artifactLambdaWithAnonymousMetadataFixture = `import java.util.function.LongFunction;
public class ArtifactLambdaOwner {
 static long word;static int ticks;static Object seen;
 static final LongFunction<Runnable> MAKE=n->{word=n*31L+7L;return ()->{ticks++;};};
 static Runnable named(long n){return ()->{word+=n;};}
 static String marker(){return "lambda$static$0";}
 Runnable unrelated(){return new Runnable(){public void run(){seen=ArtifactLambdaOwner.this;ticks+=17;}};}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){int rows=0;
  if(!ArtifactLambdaOwner.marker().equals("lambda$static$0"))throw new AssertionError("shared helper-name/string constant");
  ArtifactLambdaOwner.ticks=0;ArtifactLambdaOwner.seen=null;ArtifactLambdaOwner carrier=new ArtifactLambdaOwner();Runnable separate=carrier.unrelated();
  if(ArtifactLambdaOwner.ticks!=0||ArtifactLambdaOwner.seen!=null)throw new AssertionError("eager anonymous body");separate.run();
  if(ArtifactLambdaOwner.ticks!=17||ArtifactLambdaOwner.seen!=carrier)throw new AssertionError("independent anonymous body/capture");
  for(long n:new long[]{Long.MIN_VALUE,Integer.MIN_VALUE,-1L,0L,1L,Integer.MAX_VALUE,Long.MAX_VALUE}){
   ArtifactLambdaOwner.word=0L;ArtifactLambdaOwner.ticks=0;
   Runnable first=ArtifactLambdaOwner.MAKE.apply(n);
   long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31L)).add(java.math.BigInteger.valueOf(7L)).longValue();
   if(ArtifactLambdaOwner.word!=expected||ArtifactLambdaOwner.ticks!=0)throw new AssertionError("factory timing or overflow");
   ArtifactLambdaOwner.word=123L;first.run();
   if(ArtifactLambdaOwner.word!=123L||ArtifactLambdaOwner.ticks!=1)throw new AssertionError("nested deferred body");
   Runnable next=ArtifactLambdaOwner.named(n);if(ArtifactLambdaOwner.word!=123L)throw new AssertionError("eager named body");next.run();
   long sum=java.math.BigInteger.valueOf(123L).add(java.math.BigInteger.valueOf(n)).longValue();
   first.run();if(ArtifactLambdaOwner.word!=sum||ArtifactLambdaOwner.ticks!=2)throw new AssertionError("capture or repeated invocation");rows++;
  }
  System.out.println(rows+":artifact:helpermetadata:timing:overflow");
 }
}`

func artifactLambdaBaseFixture() string {
	source := artifactLambdaWithAnonymousMetadataFixture
	source = strings.Replace(source, " Runnable unrelated(){return new Runnable(){public void run(){seen=ArtifactLambdaOwner.this;ticks+=17;}};}\n", "", 1)
	source = strings.Replace(source, "  ArtifactLambdaOwner.ticks=0;ArtifactLambdaOwner.seen=null;ArtifactLambdaOwner carrier=new ArtifactLambdaOwner();Runnable separate=carrier.unrelated();\n  if(ArtifactLambdaOwner.ticks!=0||ArtifactLambdaOwner.seen!=null)throw new AssertionError(\"eager anonymous body\");separate.run();\n  if(ArtifactLambdaOwner.ticks!=17||ArtifactLambdaOwner.seen!=carrier)throw new AssertionError(\"independent anonymous body/capture\");\n", "", 1)
	return source
}

func TestAdversarialArtifactLambdaMetadataRetainsCompiledCodeAndOriginalABI(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactLambdaBaseFixture(), []bool{false, true}, "7:artifact:helpermetadata:timing:overflow\n")
}

// Constructor metadata has its own typed allocation obligation. Compare and
// execute every restored child, in both named and unnamed compiler modes.
func TestAdversarialArtifactLambdaMetadataKeepsIndependentAnonymousABI(t *testing.T) {
	testArtifactLambdaMetadata(t, artifactLambdaWithAnonymousMetadataFixture, []bool{false, true}, "7:artifact:helpermetadata:timing:overflow\n")
}

func testArtifactLambdaMetadata(t *testing.T, sourceFixture string, namedModes []bool, want string, validateDeclarations ...func(*testing.T, []byte, []byte)) {
	t.Helper()
	testArtifactLambdaMetadataCompilerDomains(t, sourceFixture, namedModes, want, true, validateDeclarations...)
}

// Compiler controls qualify source generation within each compiler's original
// method universe. Existing artifact tests retain their cross-compiler domain;
// this selector never relaxes RestoreClassMetadata's ABI or fresh Code checks.
func testArtifactLambdaMetadataCompilerDomains(t *testing.T, sourceFixture string, namedModes []bool, want string, crossCompiler bool, validateDeclarations ...func(*testing.T, []byte, []byte)) {
	t.Helper()
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler metadata")
	}
	for _, transition := range []struct {
		name, originalCompiler, compiler string
		originalProfile, profile         SourceCompilerProfile
	}{
		{"native control", native, native, NativeJavac8, NativeJavac8},
		{"modern control", modern, modern, ModernJavac, ModernJavac},
		{"native to modern artifact", native, modern, NativeJavac8, ModernJavac},
	} {
		if !crossCompiler && transition.originalProfile != transition.profile {
			continue
		}
		for _, named := range namedModes {
			label := transition.name + "/unnamed"
			if named {
				label = transition.name + "/parameters"
			}
			for _, owner := range []string{"ArtifactLambdaOwner", "ChangedArtifactOwner"} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(label+"/"+owner+"/"+debug, func(t *testing.T) {
						fixture := strings.ReplaceAll(sourceFixture, "ArtifactLambdaOwner", owner)
						original := compileLambdaParameterMetadataFixture(t, transition.originalCompiler, transition.originalProfile, fixture, owner, debug, named)
						if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, original), "ArtifactLambdaDriver"); got != want {
							t.Fatal("independent original oracle", got)
						}
						jar := filepath.Join(t.TempDir(), "original.jar")
						if err := os.WriteFile(jar, t23Zip(t, original), 0600); err != nil {
							t.Fatal(err)
						}
						artifact, err := NewJarSourceArtifactFromLocal(jar, 8, transition.profile, nil)
						if err != nil {
							t.Fatal(err)
						}
						defer artifact.Close()
						source, err := artifact.ReadFile(owner + ".class")
						if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "// decompile dump failed") {
							t.Fatalf("source %s: %v\n%s", owner, err, source)
						}
						// Unowned authored callers and dependencies retain their original
						// bytes. No original member of the regenerated namespace may
						// satisfy either compilation or subsequent JVM resolution.
						dependencies := map[string][]byte{}
						for name, raw := range original {
							if name != owner+".class" && !strings.HasPrefix(name, owner+"$") {
								dependencies[name] = raw
							}
						}
						out := sourceArtifactWriteFiles(t, dependencies)
						file := filepath.Join(out, owner+".java")
						if err := os.WriteFile(file, source, 0600); err != nil {
							t.Fatal(err)
						}
						args := []string{"-proc:none", "--release", "8"}
						if transition.profile == NativeJavac8 {
							args = []string{"-proc:none", "-source", "8", "-target", "8"}
						}
						if named {
							args = append(args, "-parameters")
						}
						args = append(args, "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, file)
						if log, err := exec.Command(transition.compiler, args...).CombinedOutput(); err != nil {
							t.Fatal("source compilation with owned original absent", err, string(log))
						}
						compiled, err := os.ReadFile(filepath.Join(out, owner+".class"))
						if err != nil {
							t.Fatal(err)
						}
						for _, validate := range validateDeclarations {
							validate(t, original[owner+".class"], compiled)
						}
						restored, err := artifact.RestoreClassMetadata(owner+".class", compiled)
						if err != nil || nativeBinaryShape(t, original[owner+".class"]) != nativeBinaryShape(t, restored) || lambdaParameterMetadataTables(t, original[owner+".class"]) != lambdaParameterMetadataTables(t, restored) {
							t.Fatalf("original helper declarations/parameter metadata: %v", err)
						}
						for _, validate := range validateDeclarations {
							validate(t, original[owner+".class"], restored)
						}
						nativeLexicalExactSignatures(t, owner+".class", original[owner+".class"], restored)
						if constructorInvocationThrowsDeclarations(t, original[owner+".class"]) != constructorInvocationThrowsDeclarations(t, restored) {
							t.Fatal("original complete declarations/Exceptions changed")
						}
						before, e1 := Parse(compiled)
						after, e2 := Parse(restored)
						if e1 != nil || e2 != nil || len(before.Methods) != len(after.Methods) {
							t.Fatal("compiled method universe changed", e1, e2)
						}
						for i, method := range before.Methods {
							for _, attr := range method.Attributes {
								if code, ok := attr.(*CodeAttribute); ok {
									var other *CodeAttribute
									for _, next := range after.Methods[i].Attributes {
										if candidate, ok := next.(*CodeAttribute); ok {
											if other != nil {
												t.Fatal("duplicate rebuilt Code")
											}
											other = candidate
										}
									}
									if other == nil || !bytes.Equal(code.Code, other.Code) || !reflect.DeepEqual(code, other) {
										t.Fatal("artifact imported or changed Code, handlers, frames or debug tables")
									}
								}
							}
						}
						if err := os.WriteFile(filepath.Join(out, owner+".class"), restored, 0600); err != nil {
							t.Fatal(err)
						}
						for name, raw := range original {
							if !strings.HasPrefix(name, owner+"$") {
								continue
							}
							fresh, err := os.ReadFile(filepath.Join(out, name))
							if err != nil {
								t.Fatal("original child was not rebuilt", err)
							}
							child, err := artifact.RestoreClassMetadata(name, fresh)
							if err != nil || nativeBinaryShape(t, raw) != nativeBinaryShape(t, child) || lambdaParameterMetadataTables(t, raw) != lambdaParameterMetadataTables(t, child) {
								t.Fatal("independent child's ABI/parameters changed", name, err)
							}
							nativeLexicalExactSignatures(t, name, raw, child)
							if constructorInvocationThrowsDeclarations(t, raw) != constructorInvocationThrowsDeclarations(t, child) {
								t.Fatal("original child complete declarations/Exceptions changed", name)
							}
							artifactLambdaMetadataCodeUnchanged(t, fresh, child)
							if err := os.WriteFile(filepath.Join(out, name), child, 0600); err != nil {
								t.Fatal("publish restored owned child before JVM validation", err)
							}
						}
						if got := t04RunJava(t, java, out, "ArtifactLambdaDriver"); got != want {
							t.Fatal("algorithm or evaluation timing changed", got)
						}
					})
				}
			}
		}
	}
}

func artifactLambdaMetadataCodeUnchanged(t *testing.T, compiled, restored []byte) {
	t.Helper()
	before, e1 := Parse(compiled)
	after, e2 := Parse(restored)
	if e1 != nil || e2 != nil || len(before.Methods) != len(after.Methods) {
		t.Fatal("compiled child method universe changed", e1, e2)
	}
	for i, method := range before.Methods {
		var code, other *CodeAttribute
		for _, attribute := range method.Attributes {
			if value, ok := attribute.(*CodeAttribute); ok {
				if code != nil {
					t.Fatal("duplicate compiled Code")
				}
				code = value
			}
		}
		for _, attribute := range after.Methods[i].Attributes {
			if value, ok := attribute.(*CodeAttribute); ok {
				if other != nil {
					t.Fatal("duplicate restored Code")
				}
				other = value
			}
		}
		if !reflect.DeepEqual(code, other) {
			t.Fatal("compiled child Code, handlers, stack maps or debug tables changed")
		}
	}
}

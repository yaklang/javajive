package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const independentAnnotationBoundaryFixture = `import java.lang.annotation.*;import java.util.function.IntUnaryOperator;
class AnnotationOuter{AnnotationPeer peer;static IntUnaryOperator unrelated(final String text){return value->value+text.length();}@Retention(RetentionPolicy.RUNTIME)@Target(ElementType.TYPE)public @interface Tag{String value() default "tag";int[] edges() default {-2147483648,-1,0,1,2147483647};Class<?> shape() default String[].class;}}
class AnnotationPeer{AnnotationOuter peer;}
abstract class AnnotationParent{final int seen;AnnotationParent(){seen=read();}abstract int read();}
class AnnotatedOuter{@AnnotationOuter.Tag("chosen")static class Capsule{private final int value;Capsule(int value){this.value=value;}final class Worker extends AnnotationParent{int read(){return Capsule.this.value;}}Worker make(){return new Worker();}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}
class AnnotationDriver{public static void main(String[]args){int rows=0;for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){AnnotatedOuter.Capsule c=new AnnotatedOuter.Capsule(value);AnnotatedOuter.Capsule.Worker w=c.make();if(w.seen!=value||w.read()!=value||w.getClass().getDeclaringClass()!=c.getClass())throw new AssertionError("early named capture");AnnotationOuter.Tag tag=c.getClass().getAnnotation(AnnotationOuter.Tag.class);if(tag==null||tag.annotationType()!=AnnotationOuter.Tag.class||!tag.value().equals("chosen")||tag.shape()!=String[].class||!java.util.Arrays.equals(tag.edges(),new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}))throw new AssertionError("annotation identity/defaults");int[] copy=tag.edges();copy[0]=3;if(tag.edges()[0]!=Integer.MIN_VALUE)throw new AssertionError("annotation array copy");if(AnnotationOuter.unrelated("abc").applyAsInt(2)!=5||AnnotatedOuter.unrelated("xy").applyAsInt(2)!=4)throw new AssertionError("original outer callers");rows++;}System.out.println(rows+":annotation:independent:boundary");}}
`

func TestAdversarialIndependentAnnotationBoundaryKeepsOriginalDefaultsAndCallers(t *testing.T) {
	for _, cyclic := range []bool{false, true} {
		name := "acyclic control"
		fixture := independentAnnotationBoundaryFixture
		if cyclic {
			name = "failed whole-family cycle"
			fixture = strings.Replace(fixture, "AnnotationPeer peer;", "AnnotationPeer.Token peer;static class Token{}", 1)
			fixture = strings.Replace(fixture, "class AnnotationPeer{AnnotationOuter peer;}", "class AnnotationPeer{AnnotationOuter.Token peer;static class Token{}}", 1)
		}
		t.Run(name, func(t *testing.T) { testIndependentAnnotationBoundary(t, fixture) })
	}
}
func testIndependentAnnotationBoundary(t *testing.T, fixture string) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "AnnotationOuter", debug, true)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "AnnotationDriver")
			if want != "5:annotation:independent:boundary\n" {
				t.Fatal("original oracle", want)
			}
			for _, reverse := range []bool{false, true} {
				label := "forward"
				if reverse {
					label = "reverse"
				}
				t.Run(label, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					roots := []string{"AnnotationOuter$Tag", "AnnotatedOuter$Capsule"}
					if reverse {
						roots[0], roots[1] = roots[1], roots[0]
					}
					generated := map[string][]byte{}
					for _, root := range roots {
						raw, err := z.ReadFile(root + ".class")
						if err != nil || strings.Contains(string(raw), DecompileStubMarker) {
							t.Fatalf("complete original boundary %s: %v\n%s", root, err, raw)
						}
						if root == "AnnotatedOuter$Capsule" && !strings.Contains(string(raw), "class Worker") {
							t.Fatalf("annotation dependency blocked named source scope\n%s", raw)
						}
						generated[root] = raw
					}
					out := t.TempDir()
					owned := map[string]bool{"AnnotationOuter$Tag.class": true, "AnnotatedOuter$Capsule.class": true, "AnnotatedOuter$Capsule$Worker.class": true}
					for name, raw := range files {
						if owned[name] {
							continue
						}
						if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{"-proc:none", "--release", "8", "-parameters", "-g:" + debug, "-cp", out, "-d", out}
					for _, root := range []string{"AnnotationOuter$Tag", "AnnotatedOuter$Capsule"} {
						path := filepath.Join(out, root+".java")
						if err := os.WriteFile(path, generated[root], 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, path)
					}
					if log, err := exec.Command(modern, args...).CombinedOutput(); err != nil {
						t.Fatalf("compile complete components without original fallback: %v\n%s\n%s", err, log, generated)
					}
					if got := t04RunJava(t, java, out, "AnnotationDriver"); got != want {
						t.Fatalf("same original caller %q wanted %q", got, want)
					}
					for name, before := range files {
						after, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if !owned[name] {
							if string(before) != string(after) {
								t.Fatal("unowned original changed", name)
							}
						} else if name == "AnnotationOuter$Tag.class" {
							nativeAnonymousPrefixRepresentationShape(t, name, before, after)
						} else {
							nativeStaticIndependentRootRepresentationShape("AnnotatedOuter")(t, name, before, after)
						}
					}
				})
			}
		})
	}
}

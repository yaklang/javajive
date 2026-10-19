package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
)

func TestAdversarialLambdaRegistrationDepthMatchesOriginalSourceRequestBoundary(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler depth boundary")
	}
	_, java := t04Tools(t)
	for _, depth := range []int{methodSourceLambdaDepthLimit - 1, methodSourceLambdaDepthLimit, methodSourceLambdaDepthLimit + 1} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			inner := "DepthBoundaryProbe.token"
			for i := depth - 1; i >= 1; i-- {
				inner = fmt.Sprintf("(java.util.function.Supplier<Object>)(()->{DepthBoundaryProbe.step(%d);return %s;})", i, inner)
			}
			source := fmt.Sprintf(`import java.util.function.*;
public class DepthBoundaryOwner {public static Function<Object,Object> root(){return(Object entry)->{DepthBoundaryProbe.step(0);return %s;};}}
class DepthBoundaryProbe {static final Object token=new Object();static int next;static void step(int expected){if(next++!=expected)throw new AssertionError("original deferred order");}}
class DepthBoundaryDriver {public static void main(String[]args){DepthBoundaryProbe.next=0;Function<Object,Object> root=DepthBoundaryOwner.root();if(DepthBoundaryProbe.next!=0)throw new AssertionError("eager body");Object value=root.apply(null);for(int i=1;i<%d;i++){if(DepthBoundaryProbe.next!=i)throw new AssertionError("deferred scope");value=((Supplier<?>)value).get();}if(value!=DepthBoundaryProbe.token||DepthBoundaryProbe.next!=%d)throw new AssertionError("complete original chain");System.out.println("%d:depth:deferred:identity");}}
`, inner, depth, depth, depth)
			files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, source, "DepthBoundaryOwner", "none")
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, "DepthBoundaryDriver")
			if want != fmt.Sprintf("%d:depth:deferred:identity\n", depth) {
				t.Fatal("independent original deferred depth oracle", want)
			}
			object, err := Parse(bytes.Clone(files["DepthBoundaryOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			// This checks the source-request depth bound itself, with no work
			// budget. The public path retains its independent graph/work caps.
			d.options = DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
			var rebuiltSource string
			var methods []*dumpedMethods
			err = jdecenv.Run(map[string]string{}, func() error {
				var err error
				rebuiltSource, err = d.DumpClass()
				if err == nil {
					methods, err = d.DumpMethods()
				}
				return err
			})
			if depth > methodSourceLambdaDepthLimit {
				if err == nil && !strings.Contains(rebuiltSource, DecompileStubMarker) {
					t.Fatal("source request exceeded its fixed depth boundary")
				}
				return
			}
			if err != nil || strings.Contains(rebuiltSource, DecompileStubMarker) {
				t.Fatal("original request within the fixed depth boundary was not reconstructed", err)
			}
			events, known := d.native8LambdaSourceIdentityEvents(rebuiltSource, nil, methods)
			if !known || len(events) != depth || !nativeLambdaSourceIdentityOrderClosed(events) {
				t.Fatal("exact source depth lost its original factory postorder", known, len(events))
			}
			out := t.TempDir()
			for name, raw := range files {
				if name != "DepthBoundaryOwner.class" {
					if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(out, "DepthBoundaryOwner.java")
			if err := os.WriteFile(path, []byte(rebuiltSource), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-proc:none", "-parameters", "-source", "8", "-target", "8", "-g:none", "-cp", out, "-d", out, path}
			if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
				t.Fatalf("actual compiler depth reconstruction: %v\n%s", err, log)
			}
			if got := t04RunJava(t, java, out, "DepthBoundaryDriver"); got != want {
				t.Fatalf("unchanged deferred caller: %q want %q", got, want)
			}
			rebuilt, err := os.ReadFile(filepath.Join(out, "DepthBoundaryOwner.class"))
			if err != nil {
				t.Fatal(err)
			}
			if lambdaParameterMetadataTables(t, rebuilt) != lambdaParameterMetadataTables(t, files["DepthBoundaryOwner.class"]) || constructorInvocationThrowsDeclarations(t, rebuilt) != constructorInvocationThrowsDeclarations(t, files["DepthBoundaryOwner.class"]) {
				t.Fatal("depth boundary changed original implementation declarations or physical metadata")
			}
		})
	}
}

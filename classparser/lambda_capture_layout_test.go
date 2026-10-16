package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialLambdaCaptureDeclarationLayout(t *testing.T) {
	for _, indent := range []string{"", " ", "  ", "\t", "\t\t", " \t "} {
		for _, typ := range []string{"Object", "long", "java.util.List<String>", "byte[]"} {
			t.Run(strings.ReplaceAll(indent, "\t", "T")+"/"+typ, func(t *testing.T) {
				input := "class C {\n\tvoid run() {\n" + indent + typ + " var1;\n" +
					"\t\twhile (more()) {\n\t\t\tvar1 = source();\n\t\t\tRunnable var2 = () -> {\n" +
					"\t\t\t\tread(var1);\n\t\t\t};\n\t\t}\n\t}\n}\n"
				output := fixLambdaLoopCapture(input)
				if !strings.Contains(output, "final "+typ+" var1_f1 = var1;") || !strings.Contains(output, "read(var1_f1);") {
					t.Fatalf("layout lost the visible declaration or its capture:\n%s", output)
				}
			})
		}
	}
	for _, body := range []string{
		"\t\tRunnable var2 = () -> {\n\t\t\tread(var1);\n\t\t};\nObject var1 = source();\n",
		"\t\tif (flag()) {\nObject var1 = source();\n\t\t}\n\t\tRunnable var2 = () -> {\n\t\t\tread(var1);\n\t\t};\n",
	} {
		input := "class C {\n\tvoid run() {\n" + body + "\t}\n}\n"
		if output := fixLambdaLoopCapture(input); output != input {
			t.Fatalf("layout manufactured an invisible binding:\n%s", output)
		}
	}
}

// The generated source has a lifted mutable slot; legal source instead binds
// a fresh local inside each loop iteration. The unchanged binary caller tests
// null, equal-but-distinct references, heap mutation, deferred nested execution
// and finally effects. A final copy must retain the reference, not its contents.
func TestAdversarialLambdaCaptureLiftedDeclarationRuntime(t *testing.T) {
	const fixture = `import java.util.function.Supplier;
class LayoutCaptureOwner {
 static int effects;
	static Supplier<Supplier<Object>>[] make(Object[] input) {
Object var1 = null;
  Supplier<Supplier<Object>>[] result = new Supplier[input.length];
  try {
   for (int i=0;i<input.length;i++) {
    var1 = input[i];
    Supplier<Supplier<Object>> var2 = () -> {
     effects++;
     Supplier<Object> lv1_3 = () -> {
      effects+=100;
      return var1;
     };
     return lv1_3;
    };
    result[i]=var2;
   }
  } finally { effects+=10000; }
  return result;
 }
}
class LayoutCaptureDriver {
 public static void main(String[]args) {
  int rows=0;
  for(int seed=-16;seed<=16;seed++) {
   int[] first={seed},second={seed};
   Object[] input={first,null,second,first};
   LayoutCaptureOwner.effects=0;
   Supplier<Supplier<Object>>[] closures=LayoutCaptureOwner.make(input);
   if(LayoutCaptureOwner.effects!=10000)throw new AssertionError("eager body or lost cleanup");
   input[0]=second;input[1]=first;first[0]+=37;
   Object[] expected={first,null,second,first};
   for(int i=0;i<4;i++) {
    Supplier<Object> nested=closures[i].get();
    for(int repeat=0;repeat<3;repeat++) {
     Object actual=nested.get();
     if(actual!=expected[i])throw new AssertionError("factory-time reference identity");
     if(actual!=null&&((int[])actual)[0]!=(i==2?seed:seed+37))throw new AssertionError("live heap contents");
     rows++;
    }
   }
   if(LayoutCaptureOwner.effects!=11204)throw new AssertionError("deferred nested effects");
  }
  System.out.println(rows+":layout:snapshot:identity:null:heap:deferred:finally");
 }
}`
	legal := strings.Replace(fixture, "Object var1 = null;", "", 1)
	legal = strings.Replace(legal, "var1 = input[i];", "final Object var1 = input[i];", 1)
	want, original := t04CompileRun(t, "8", "LayoutCaptureDriver", map[string]string{"LayoutCaptureOwner.java": legal})
	if want != "396:layout:snapshot:identity:null:heap:deferred:finally\n" {
		t.Fatalf("independent original oracle: %q", want)
	}
	modern, java := t04Tools(t)
	for _, indent := range []string{"", "  ", "\t"} {
		for _, compiler := range []struct{ name, path string }{{"modern", modern}, {"native8", os.Getenv("JAVA8_JAVAC")}} {
			t.Run(compiler.name+"/"+strings.ReplaceAll(indent, "\t", "T"), func(t *testing.T) {
				if compiler.path == "" {
					t.Skip("JAVA8_JAVAC required for the actual Java 8 compiler")
				}
				root := t.TempDir()
				if err := os.WriteFile(filepath.Join(root, "LayoutCaptureDriver.class"), original["LayoutCaptureDriver"], 0600); err != nil {
					t.Fatal(err)
				}
				input := strings.Replace(fixture, "Object var1 = null;", indent+"Object var1 = null;", 1)
				output := fixLambdaLoopCapture(input)
				output, _, _ = strings.Cut(output, "class LayoutCaptureDriver {")
				path := filepath.Join(root, "LayoutCaptureOwner.java")
				if err := os.WriteFile(path, []byte(output), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-proc:none", "-encoding", "UTF-8", "-source", "8", "-target", "8", "-cp", root, "-d", root, path}
				if raw, err := exec.Command(compiler.path, args...).CombinedOutput(); err != nil {
					t.Fatalf("rebuilt capture compilation: %v\n%s\n%s", err, raw, output)
				}
				if got := t04RunJava(t, java, root, "LayoutCaptureDriver"); got != want {
					t.Fatalf("unchanged caller: %q want %q", got, want)
				}
			})
		}
	}
}

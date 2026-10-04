package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive"
	"github.com/yaklang/javajive/classparser/classes"
)

// These are reviewed, authored single-class samples, not external JARs. The
// original embedded bytecode supplies the oracle; old printed Java goldens do
// not (the old LongTest golden did not even compile). Original and rebuilt
// classes have separate classpaths. Stack-trace source locations are incidental
// to source reconstruction; keep stdout and the exception class/message instead.
func TestEmbeddedSampleGoldenSemantics(t *testing.T) {
	tool := func(name string) string {
		t.Helper()
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		return path
	}
	javac, java := tool("javac"), tool("java")
	command := func(dir, tool string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", tool, args, err, out)
		}
		return string(out)
	}
	write := func(dir, name string, data []byte) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	const driver = `import java.lang.reflect.*;import java.io.*;
public class SampleDriver {
 static void invoke(Object receiver,Method method,Object...args)throws Exception{
  method.setAccessible(true);ByteArrayOutputStream bytes=new ByteArrayOutputStream();PrintStream previous=System.err;System.setErr(new PrintStream(bytes));
  try{Object result=method.invoke(receiver,args);System.out.println("returned:"+result);}catch(InvocationTargetException failure){System.out.println("thrown:"+failure.getCause().getClass().getName()+":"+failure.getCause().getMessage());}finally{System.setErr(previous);}
  String trace=bytes.toString("UTF-8");System.out.println("stderr:"+(trace.isEmpty()?"":trace.split("\n",2)[0]));
 }
 public static void main(String[]args)throws Exception{
  Class<?> cls=Class.forName("org.benf.cfr.reader."+args[0]);cls.getDeclaredMethods();cls.getDeclaredConstructors();Object value=cls.getDeclaredConstructor().newInstance();
  if(args[0].equals("VarArgs")){invoke(value,cls.getDeclaredMethod("invoke"));Method method=cls.getDeclaredMethod("main",String[].class);for(String[] input:new String[][]{{"one","two"},{},null})invoke(value,method,(Object)input);return;}
  if(args[0].equals("TryCatch1")){invoke(value,cls.getDeclaredMethod("main",String[].class),(Object)new String[0]);return;}
  invoke(value,cls.getDeclaredMethod("main"));
  if(args[0].equals("LambdaTest")){Field field=cls.getDeclaredField("a");field.setAccessible(true);System.out.println("field:"+field.getInt(value));}
  if(args[0].equals("TernaryExpressionTest"))invoke(value,cls.getDeclaredMethod("getVar"));
 }
}`
	for _, name := range []string{"LongTest", "TryCatch1", "LambdaTest", "IfTest", "TryCatch", "TernaryExpressionTest", "LogicalOperation", "syntax_test/VarArgs", "syntax_test/SwitchScopeTest"} {
		t.Run(name, func(t *testing.T) {
			base := filepath.Base(name)
			raw, err := classes.FS.ReadFile(name + ".class")
			if err != nil {
				t.Fatal(err)
			}
			original := t.TempDir()
			write(original, "org/benf/cfr/reader/"+base+".class", raw)
			write(original, "SampleDriver.java", []byte(driver))
			command(original, javac, "--release", "8", "-proc:none", "-cp", original, "-d", original, "SampleDriver.java")
			want := command(original, java, "-Xmx128m", "-Xverify:all", "-cp", original, "SampleDriver", base)
			for _, mode := range []javajive.DecompileMode{javajive.Precision, javajive.Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := javajive.DecompileWithOptions(raw, javajive.DecompileOptions{Mode: mode})
					if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 {
						t.Fatalf("source contract: err=%v status=%s stubs=%v diagnostics=%v", err, result.Status, result.StubMethods, result.Diagnostics)
					}
					if strings.Contains(result.Source, "/* yak-decompiler:") {
						t.Fatal("stub source cannot be a golden")
					}
					built := t.TempDir()
					write(built, base+".java", []byte(result.Source))
					write(built, "SampleDriver.java", []byte(driver))
					command(built, javac, "--release", "8", "-proc:none", "-cp", built, "-d", built, base+".java", "SampleDriver.java")
					if got := command(built, java, "-Xmx128m", "-Xverify:all", "-cp", built, "SampleDriver", base); got != want {
						t.Fatalf("original/rebuilt behavior differs\noriginal:%q\nrebuilt:%q\n%s", want, got, result.Source)
					}
				})
			}
		})
	}
}

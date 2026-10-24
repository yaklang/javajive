package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestAdversarialConstructorGenericThrowsCallerSourceFormalIsNotErasedCoverage(t *testing.T) {
	metadata := constructorThrowsBindingMetadata()
	d := &ClassObjectDumper{FuncCtx: &class_context.ClassContext{InvocationMetadata: func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }}}
	for _, row := range []struct {
		name, sig, desc string
		raw, inferred   []string
		want            bool
	}{
		{"checked erasure is not formal X", "<X:Ljava/lang/Exception;>()V^TX;", "()V", []string{"java/lang/Exception"}, []string{"java/io/IOException"}, false},
		{"checked argument erasure is not same DeclId", "<X:Ljava/io/IOException;>(TX;)V^TX;", "(Ljava/io/IOException;)V", []string{"java/io/IOException"}, []string{"java/io/IOException"}, false},
		{"mixed concrete source covers IO", "<X:Ljava/lang/Exception;>()V^TX;^Ljava/io/IOException;", "()V", []string{"java/lang/Exception", "java/io/IOException"}, []string{"java/io/IOException"}, true},
		{"mixed concrete source does not cover Exception", "<X:Ljava/lang/Exception;>()V^TX;^Ljava/io/IOException;", "()V", []string{"java/lang/Exception", "java/io/IOException"}, []string{"java/lang/Exception"}, false},
		{"unchecked independent of formal throws", "<X:Ljava/lang/Exception;>()V^TX;", "()V", []string{"java/lang/Exception"}, []string{"java/lang/RuntimeException"}, true},
		{"Signature omits proper throws", "<X:Ljava/lang/Object;>()V", "()V", []string{"java/lang/Exception"}, []string{"java/io/IOException"}, true},
		{"ordinary checked source declaration", "<X:Ljava/lang/Object;>()V^Ljava/lang/Exception;", "()V", []string{"java/lang/Exception"}, []string{"java/io/IOException"}, true},
		{"source Signature disagrees with raw Exceptions", "<X:Ljava/lang/Exception;>()V^TX;", "()V", []string{"java/io/IOException"}, []string{"java/lang/RuntimeException"}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			if got := d.constructorThrowsCallerSourceCoverage("", row.sig, row.desc, row.raw, row.inferred); got != row.want {
				t.Fatalf("source coverage=%v want %v", got, row.want)
			}
		})
	}
}

func TestAdversarialConstructorInvocationGenericThrowsCallerFormalOriginalJVMRefusal(t *testing.T) {
	source := `class FormalThrowState {static boolean fail;static Throwable seen;static final java.io.IOException io=new java.io.IOException("same");}
class FormalThrowParent {<E extends Throwable>FormalThrowParent(E value)throws E{FormalThrowState.seen=value;if(FormalThrowState.fail)throw value;}}
class FormalThrowCaller extends FormalThrowParent {
 FormalThrowCaller()throws Exception{super(new java.io.IOException("fresh"));}
 FormalThrowCaller(java.io.IOException value)throws Exception{super(value);}
}
class FormalUncheckedParent {<E extends Throwable>FormalUncheckedParent(Throwable value)throws E{FormalThrowState.seen=value;if(FormalThrowState.fail)FormalUncheckedParent.<E>fail(value);} @SuppressWarnings("unchecked")static<E extends Throwable>void fail(Throwable value)throws E{throw(E)value;}}
class FormalUncheckedCaller extends FormalUncheckedParent {FormalUncheckedCaller(java.io.IOException value)throws Exception{super(value);}}
class FormalThrowDriver {
 public static void main(String[]args)throws Exception{
  int rows=0,formals=0;
  for(boolean fail:new boolean[]{false,true})for(boolean argument:new boolean[]{false,true}){
   FormalThrowState.fail=fail;FormalThrowState.seen=null;
   try{if(argument)new FormalThrowCaller(FormalThrowState.io);else new FormalThrowCaller();if(fail)throw new AssertionError("missing throw");}
   catch(Throwable failure){if(!fail||failure!=FormalThrowState.seen||!(failure instanceof java.io.IOException))throw new AssertionError("same original exception",failure);}
   rows++;
  }
  for(boolean fail:new boolean[]{false,true}){
   FormalThrowState.fail=fail;FormalThrowState.seen=null;
   try{new FormalUncheckedCaller(FormalThrowState.io);if(fail)throw new AssertionError("unchecked missing throw");}
   catch(Throwable failure){if(!fail||failure!=FormalThrowState.io||failure!=FormalThrowState.seen)throw new AssertionError("unchecked same exception",failure);}
   rows++;
  }
  for(Class<?> cls:new Class<?>[]{FormalThrowCaller.class,FormalUncheckedCaller.class})for(java.lang.reflect.Constructor<?> c:cls.getDeclaredConstructors()){
   if(c.getExceptionTypes().length!=1||c.getExceptionTypes()[0]!=Exception.class)throw new AssertionError("raw Exceptions ABI");
   java.lang.reflect.Type type=c.getGenericExceptionTypes()[0];
   if(type instanceof java.lang.reflect.TypeVariable){if(!((java.lang.reflect.TypeVariable<?>)type).getName().equals("X"))throw new AssertionError("formal identity");formals++;}
   else if(type!=Exception.class)throw new AssertionError("original generic Exceptions");
  }
  System.out.println(rows+":formal-throws:same:rawException:generic"+formals);
 }
}`
	files := nativePrivateEnumCompile(t, source, "FormalThrowCaller", "none")
	_, java := t04Tools(t)
	write := func(root string) {
		for n, raw := range files {
			if err := os.WriteFile(filepath.Join(root, n), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	original := t.TempDir()
	write(original)
	if got := t04RunJava(t, java, original, "FormalThrowDriver"); got != "6:formal-throws:same:rawException:generic0\n" {
		t.Fatalf("ordinary original JVM: %q", got)
	}
	beforeCode := map[string][]byte{}
	for _, class := range []string{"FormalThrowCaller", "FormalUncheckedCaller"} {
		caller, err := Parse(bytes.Clone(files[class+".class"]))
		if err != nil {
			t.Fatal(err)
		}
		caller.ConstantPoolManager.AddUtf8Info("Signature")
		for _, m := range caller.Methods {
			n, _ := caller.getUtf8(m.NameIndex)
			if n != "<init>" {
				continue
			}
			desc, _ := caller.getUtf8(m.DescriptorIndex)
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					beforeCode[class+desc] = bytes.Clone(code.Code)
				}
			}
			sig := "<X:Ljava/lang/Exception;>" + desc + "^TX;"
			m.Attributes = append(m.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(caller.ConstantPoolManager.AddUtf8Info(sig))})
		}
		files[class+".class"] = caller.Bytes()
	}
	mutant := t.TempDir()
	write(mutant)
	if got := t04RunJava(t, java, mutant, "FormalThrowDriver"); got != "6:formal-throws:same:rawException:generic3\n" {
		t.Fatalf("Signature-only original JVM mutant: %q", got)
	}
	t.Log("Signature-only original JVM: normal/failure, same exception objects, raw Exception and three distinct constructor formals X")
	check, err := Parse(bytes.Clone(files["FormalThrowCaller.class"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range check.Methods {
		n, _ := check.getUtf8(m.NameIndex)
		if n != "<init>" {
			continue
		}
		desc, _ := check.getUtf8(m.DescriptorIndex)
		raw, known := originalMethodExceptions(check, m)
		if !known || len(raw) != 1 || raw[0] != "java/lang/Exception" {
			t.Fatal("original Exceptions changed", raw)
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok && !bytes.Equal(code.Code, beforeCode["FormalThrowCaller"+desc]) {
				t.Fatal("Signature mutant changed original executable bytes")
			}
		}
	}
	candidate := strings.ReplaceAll(source, "FormalThrowCaller()throws Exception", "<X extends Exception>FormalThrowCaller()throws X")
	candidate = strings.ReplaceAll(candidate, "FormalThrowCaller(java.io.IOException value)throws Exception", "<X extends Exception>FormalThrowCaller(java.io.IOException value)throws X")
	compileDir := t.TempDir()
	path := filepath.Join(compileDir, "FormalThrowCaller.java")
	if err := os.WriteFile(path, []byte(candidate), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(os.Getenv("JAVA8_JAVAC"), "-proc:none", "-source", "8", "-target", "8", "-d", compileDir, path).CombinedOutput(); err == nil {
		t.Fatalf("source throws X must not cover concrete IOException: %s", out)
	}
	resolve := func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }
	before := bytes.Clone(files["FormalThrowCaller.class"])
	result, err := DecompileWithOptions(bytes.Clone(before), DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, Resolve: resolve, EnvSnapshot: map[string]string{}})
	if err != nil || !strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatalf("formal source declaration must remain unsupported: %v\n%s", err, result.Source)
	}
	if !bytes.Equal(before, files["FormalThrowCaller.class"]) {
		t.Fatal("original Signature/Exceptions/flags mutated")
	}
	uncheckedOriginal := bytes.Clone(files["FormalUncheckedCaller.class"])
	unchecked, err := DecompileWithOptions(bytes.Clone(uncheckedOriginal), DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, Resolve: resolve, EnvSnapshot: map[string]string{}})
	if err != nil || strings.Contains(unchecked.Source, DecompileStubMarker) {
		t.Fatalf("unchecked inference may retain original throws X: %v\n%s", err, unchecked.Source)
	}
	rebuilt := t.TempDir()
	write(rebuilt)
	if err := os.Remove(filepath.Join(rebuilt, "FormalUncheckedCaller.class")); err != nil {
		t.Fatal(err)
	}
	uncheckedPath := filepath.Join(rebuilt, "FormalUncheckedCaller.java")
	if err := os.WriteFile(uncheckedPath, []byte(unchecked.Source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(os.Getenv("JAVA8_JAVAC"), "-proc:none", "-source", "8", "-target", "8", "-g:none", "-cp", rebuilt, "-d", rebuilt, uncheckedPath).CombinedOutput(); err != nil {
		t.Fatalf("same native8 rebuild with original source throws X: %v\n%s", err, out)
	}
	if got := t04RunJava(t, java, rebuilt, "FormalThrowDriver"); got != "6:formal-throws:same:rawException:generic3\n" {
		t.Fatalf("unchecked source throws formal rebuilt JVM: %q", got)
	}
	next, err := os.ReadFile(filepath.Join(rebuilt, "FormalUncheckedCaller.class"))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := constructorInvocationThrowsDeclarations(t, uncheckedOriginal), constructorInvocationThrowsDeclarations(t, next); a != b {
		t.Fatalf("unchecked source formal Signature/Exceptions/flags changed:\n%s\n%s", a, b)
	}

}

func TestAdversarialConstructorInvocationGenericThrowsOpaqueProducerRemainsUnsupported(t *testing.T) {
	source := `class OpaqueThrowState {static boolean fail;static final java.io.IOException io=new java.io.IOException("same");static<T>T opaque(T value){return value;}}
class OpaqueThrowParent {<E extends Throwable>OpaqueThrowParent(E value)throws E{if(OpaqueThrowState.fail)throw value;}}
class OpaqueThrowChild extends OpaqueThrowParent {OpaqueThrowChild(java.io.IOException value)throws java.io.IOException{super(OpaqueThrowState.opaque(value));}}
class OpaqueThrowDriver {public static void main(String[]args)throws Exception{int rows=0;for(boolean fail:new boolean[]{false,true}){OpaqueThrowState.fail=fail;try{new OpaqueThrowChild(OpaqueThrowState.io);if(fail)throw new AssertionError("missing");}catch(Throwable failure){if(!fail||failure!=OpaqueThrowState.io)throw new AssertionError("same",failure);}rows++;}System.out.println(rows+":opaque:original:same");}}`
	files := nativePrivateEnumCompile(t, source, "OpaqueThrowChild", "none")
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "OpaqueThrowDriver"); got != "2:opaque:original:same\n" {
		t.Fatal("independent opaque original JVM", got)
	}
	resolve := func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }
	result, err := DecompileWithOptions(bytes.Clone(files["OpaqueThrowChild.class"]), DecompileOptions{Mode: Precision, SourceCompiler: NativeJavac8, TargetSourceVersion: 8, Resolve: resolve, EnvSnapshot: map[string]string{}})
	if err != nil || !strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatalf("opaque/poly producer needs its own source inference proof: %v\n%s", err, result.Source)
	}
}

func TestAdversarialConstructorInvocationGenericThrowsHeaderViewOwnsTypedProjection(t *testing.T) {
	for _, mode := range []string{"ordinary", "missing scope", "standalone", "suppressed standalone", "throws recovery off"} {
		t.Run(mode, func(t *testing.T) {
			d := &ClassObjectDumper{FuncCtx: &class_context.ClassContext{TypeParams: []string{"X"}}, options: DecompileOptions{EnvSnapshot: map[string]string{}}}
			want := true
			switch mode {
			case "missing scope":
				d.FuncCtx.TypeParams = nil
				want = false
			case "standalone":
				d.FuncCtx.StandaloneEraseTypeVars = map[string]string{"X": "java.lang.Exception"}
				want = false
			case "suppressed standalone":
				d.FuncCtx.StandaloneEraseTypeVars = map[string]string{"X": "java.lang.Exception"}
				d.FuncCtx.SuppressStandaloneErase = true
			case "throws recovery off":
				d.options.EnvSnapshot["JDEC_THROWS_SIG_RECOVERY_OFF"] = "1"
				want = false
			}
			if got := d.constructorThrowsHeaderView("<X:Ljava/lang/Exception;>()V^TX;"); got != want {
				t.Fatalf("actual typed header decision %v want%v", got, want)
			}
		})
	}
}

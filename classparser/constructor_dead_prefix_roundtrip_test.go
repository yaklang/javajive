package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Coremail's Groovy closure stores an unused call-site array before super.
// The carrier must preserve the call's side effects and thrown object, not
// move them after initialization or infer that a dead result makes it pure.
func TestConstructorDeadPrefixCallPreservesThrowOrder(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	source := `class DeadPrefixEffects {
 static String trace="";static boolean fail;
 static final RuntimeException prefix=new RuntimeException("prefix"),parent=new RuntimeException("parent");
 static Object calls(){trace+="P";if(fail)throw prefix;return new Object();}
}
class DeadPrefixParent {
 final Object value;DeadPrefixParent(Object a,Object b){DeadPrefixEffects.trace+="S";if(b==null)throw DeadPrefixEffects.parent;value=a;}
}
public class DeadPrefixConsumer extends DeadPrefixParent {
 public DeadPrefixConsumer(Object a,Object b){super(a,b);Object ignored=DeadPrefixEffects.calls();DeadPrefixEffects.trace+="B";}
}
class DeadPrefixOracle {
 public static void main(String[]args){for(boolean fail:new boolean[]{false,true})for(Object b:new Object[]{null,new Object()}){
  Object a=new Object();DeadPrefixEffects.trace="";DeadPrefixEffects.fail=fail;DeadPrefixConsumer result=null;Throwable caught=null;
  try{result=new DeadPrefixConsumer(a,b);}catch(Throwable t){caught=t;}
  Throwable expected=fail?DeadPrefixEffects.prefix:b==null?DeadPrefixEffects.parent:null;
  String trace=fail?"P":b==null?"PS":"PSB";
  if(caught!=expected||!trace.equals(DeadPrefixEffects.trace)||(result!=null&&result.value!=a))throw new AssertionError("order/throw identity/value");
  System.out.println(fail+":"+(b==null)+":"+trace);
 }}
}`
	path := filepath.Join(dir, "DeadPrefixConsumer.java")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("original compile: %v\n%s", err, out)
	}
	obj, err := Parse(readClassBytes(t, dir, "DeadPrefixConsumer"))
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, method := range obj.Methods {
		name, _ := obj.getUtf8(method.NameIndex)
		if name != "<init>" {
			continue
		}
		for _, attr := range method.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok {
				continue
			}
			old := code.Code
			if len(old) < 11 || old[0] != 0x2a || old[1] != 0x2b || old[2] != 0x2c || old[3] != 0xb7 || old[6] != 0xb8 || old[9] != 0x4e || len(code.ExceptionTable) != 0 {
				t.Fatalf("unexpected source constructor %x", old)
			}
			code.Code = append(append(append([]byte{}, old[6:10]...), old[:6]...), old[10:]...)
			changed = true
		}
	}
	if !changed {
		t.Fatal("missing constructor")
	}
	if err := os.WriteFile(filepath.Join(dir, "DeadPrefixConsumer.class"), obj.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(cp string) string {
		out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "DeadPrefixOracle").CombinedOutput()
		if err != nil {
			t.Fatalf("bytecode oracle: %v\n%s", err, out)
		}
		return string(out)
	}
	want := run(dir)
	resolve := resolverFromClasses(classMapFromDir(t, dir))
	result, err := DecompileWithOptions(obj.Bytes(), DecompileOptions{Mode: Precision, Resolve: resolve, TargetSourceVersion: 8})
	if err != nil || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
		t.Fatalf("prefix recovery: %v\n%s", err, result.Source)
	}
	rebuilt := t.TempDir()
	path = filepath.Join(rebuilt, "DeadPrefixConsumer.java")
	if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, path).CombinedOutput(); err != nil {
		t.Fatalf("rebuild: %v\n%s\n%s", err, out, result.Source)
	}
	if got := run(rebuilt + string(os.PathListSeparator) + dir); got != want {
		t.Fatalf("changed original behavior: got %q want %q", got, want)
	}
}

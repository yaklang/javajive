package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// The parameter stays unknown to the motion proof. Original parent bytes and
// driver check both exits, original exception identity, and external effects.
// Only the original child's synthetic final capture is moved before SUPER.
const constructorUnknownAbruptCaptureFixture = `class AbruptParent {
 static final java.io.IOException marker=new java.io.IOException("shared");
 static java.io.IOException last;static int trace;final int n;
 AbruptParent(int n)throws java.io.IOException {
  trace=trace*31+1;
  if(n==-1)throw marker;
  if(n==-2){last=new java.io.IOException("fresh");throw last;}
  if(n==-3)throw null;
  this.n=n;trace=trace*31+2;
 }
}
final class AbruptChild extends AbruptParent {
 final Object value;AbruptChild(Object value,int n)throws java.io.IOException {super(n);this.value=value;}
}
class AbruptDriver {
 public static void main(String[]args)throws Exception {
  Object x=new Object();
  for(int n:new int[]{-3,-2,-1,0,7,70000})for(Object value:new Object[]{null,x}) {
   AbruptParent.trace=0;
   try {AbruptChild c=new AbruptChild(value,n);
    if(n<0||c.n!=n||c.value!=value||AbruptParent.trace!=33)throw new AssertionError("normal");
    System.out.println(n+":ok:"+(value==x));
   }catch(java.io.IOException e){
    if(n!=-1&&n!=-2||e!=(n==-1?AbruptParent.marker:AbruptParent.last)||AbruptParent.trace!=1)throw new AssertionError("identity/order");
    System.out.println(n+":"+e.getMessage()+":"+(value==x));
   }catch(NullPointerException e){
    if(n!=-3||AbruptParent.trace!=1)throw new AssertionError("null/order");
    System.out.println(n+":null:"+(value==x));
   }
  }
 }
}`

func TestAdversarialConstructorUnknownInputAbruptCaptureRoundTrip(t *testing.T) {
	for _, prefix := range []string{"Abrupt", "Sigma"} {
		t.Run(prefix, func(t *testing.T) {
			testConstructorUnknownInputAbruptCaptureRoundTrip(t, strings.ReplaceAll(constructorUnknownAbruptCaptureFixture, "Abrupt", prefix), prefix)
		})
	}
}

func testConstructorUnknownInputAbruptCaptureRoundTrip(t *testing.T, fixture, prefix string, expectedOutput ...string) {
	t.Helper()
	javac, java := t04Tools(t)
	child, driver := prefix+"Child", prefix+"Driver"
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files[child+".class"])
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range obj.Fields {
				name, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if name == "value" {
					f.AccessFlags |= 0x1000
				}
			}
			changed := false
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name != "<init>" {
					continue
				}
				var originalCode *CodeAttribute
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						if originalCode != nil {
							t.Fatal("duplicate original code")
						}
						originalCode = c
					}
				}
				if originalCode == nil {
					t.Fatal("missing original code")
				}
				decoder := core.NewDecompiler(originalCode.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := constructorMotionOps(decoder)
				store := -1
				for i, op := range ops {
					member := constructorMotionMember(obj, op, core.OP_PUTFIELD)
					if member != nil && member.Name == child && member.Member == "value" && member.Description == "Ljava/lang/Object;" {
						if i < 2 || ops[i-2].Instr.OpCode != core.OP_ALOAD_0 || ops[i-1].Instr.OpCode != core.OP_ALOAD_1 {
							t.Fatal("capture packet")
						}
						store = int(ops[i-2].CurrentOffset)
					}
				}
				if store < 0 {
					t.Fatal("missing original capture")
				}
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						if store+5 >= len(code.Code) || len(code.ExceptionTable) != 0 {
							t.Fatal("capture boundary")
						}
						raw := append([]byte(nil), code.Code[store:store+5]...)
						raw = append(raw, code.Code[:store]...)
						raw = append(raw, code.Code[store+5:]...)
						code.Code, code.Attributes = raw, nil
						code.AttrLen = uint32(12 + len(raw))
						changed = true
					}
				}
			}
			if !changed {
				t.Fatal("no mutation")
			}
			files[child+".class"] = obj.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, driver)
			expected := "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n"
			if len(expectedOutput) == 1 {
				expected = expectedOutput[0]
			} else if len(expectedOutput) > 1 {
				t.Fatal("ambiguous original oracle")
			}
			if oracle != expected {
				t.Fatalf("original oracle=%q", oracle)
			}
			resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				var source string
				if mode == "legacy" {
					source, err = DecompileWithResolver(files[child+".class"], resolve)
				} else {
					var r DecompileResult
					r, err = DecompileWithOptions(files[child+".class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					source = r.Source
				}
				if err != nil || strings.Contains(source, DecompileStubMarker) {
					t.Fatalf("%s:%v\n%s", mode, err, source)
				}
				out := t.TempDir()
				path := filepath.Join(out, child+".java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatalf("rebuild:%v\n%s\n%s", err, raw, source)
				}
				if got := t04RunJava(t, java, out+string(os.PathListSeparator)+original, driver); got != oracle {
					t.Fatalf("got=%q want=%q", got, oracle)
				}
			}
		})
	}
}

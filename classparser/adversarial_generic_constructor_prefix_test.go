package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Move an authored, receiver-free parameter check before initialization in the
// original classfile. The JVM allows this order; a source carrier must keep it,
// the original generic binding and the exact constructor descriptor.
func TestAdversarialGenericConstructorPrefixBinding(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounded := range []bool{false, true} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(fmt.Sprintf("bounded=%v/%s", bounded, debug), func(t *testing.T) {
				dir := t.TempDir()
				formal := "<T>"
				if bounded {
					formal = "<T extends Number & Comparable<T>>"
				}
				source := `class GenericPrefixEffects {
 static String trace="";static final RuntimeException prefix=new RuntimeException("prefix"),parent=new RuntimeException("parent"),body=new RuntimeException("body");
 static void check(Object marker,int mode){trace+="P";if(marker==null||mode==1)throw prefix;}
}
class GenericPrefixParent<T> {final T value;GenericPrefixParent(T value,int mode){GenericPrefixEffects.trace+="S";if(mode==2)throw GenericPrefixEffects.parent;this.value=value;}}
public class GenericPrefixConsumer` + formal + ` extends GenericPrefixParent<T> {
 public GenericPrefixConsumer(T first,Object marker,int mode){super(first,mode);GenericPrefixEffects.check(marker,mode);GenericPrefixEffects.trace+="B";if(mode==3)throw GenericPrefixEffects.body;}
}
class GenericPrefixOracle {public static void main(String[]args)throws Exception {
 java.lang.reflect.Constructor<?> ctor=GenericPrefixConsumer.class.getDeclaredConstructors()[0];
 if(ctor.getExceptionTypes().length!=0)throw new AssertionError("changed checked ABI");
 for(Object value:new Object[]{null,Integer.valueOf(91)})for(Object marker:new Object[]{null,new Object()})for(int mode=0;mode<4;mode++){
  GenericPrefixEffects.trace="";Object result=null;Throwable failure=null;try{result=ctor.newInstance(value,marker,mode);}catch(java.lang.reflect.InvocationTargetException wrapped){failure=wrapped.getCause();}
  Throwable expected=marker==null||mode==1?GenericPrefixEffects.prefix:mode==2?GenericPrefixEffects.parent:mode==3?GenericPrefixEffects.body:null;
  String trace=expected==GenericPrefixEffects.prefix?"P":expected==GenericPrefixEffects.parent?"PS":"PSB";
  if(failure!=expected||!trace.equals(GenericPrefixEffects.trace)||(result!=null&&((GenericPrefixParent<?>)result).value!=value))throw new AssertionError("generic binding/identity/order");
  System.out.println((value==null)+":"+(marker==null)+":"+mode+":"+trace+":"+(failure==null?"ok":failure.getMessage()));
 }
}}
`
				file := filepath.Join(dir, "GenericPrefixConsumer.java")
				if e := os.WriteFile(file, []byte(source), 0600); e != nil {
					t.Fatal(e)
				}
				if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); e != nil {
					t.Fatalf("original %v\n%s", e, out)
				}
				obj, e := Parse(readClassBytes(t, dir, "GenericPrefixConsumer"))
				if e != nil {
					t.Fatal(e)
				}
				pool := NewConstantPoolWithConstant(&obj.ConstantPool)
				changed := false
				for _, method := range obj.Methods {
					if pool.GetUtf8(int(method.NameIndex)).Value != "<init>" {
						continue
					}
					for _, a := range method.Attributes {
						if code, ok := a.(*CodeAttribute); ok {
							old := code.Code
							// aload_0, aload_1, iload_3, invokespecial (6 bytes), then
							// aload_2, iload_3, invokestatic (5 bytes). Verify before moving.
							if len(old) < 11 || old[0] != 0x2a || old[1] != 0x2b || old[2] != 0x1d || old[3] != 0xb7 || old[6] != 0x2c || old[7] != 0x1d || old[8] != 0xb8 || len(code.ExceptionTable) != 0 {
								t.Fatalf("unexpected constructor %x", old)
							}
							code.Code = append(append(append([]byte{}, old[6:11]...), old[:6]...), old[11:]...)
							changed = true
						}
					}
				}
				if !changed {
					t.Fatal("no original constructor")
				}
				if e = os.WriteFile(filepath.Join(dir, "GenericPrefixConsumer.class"), obj.Bytes(), 0600); e != nil {
					t.Fatal(e)
				}
				run := func(cp string) string {
					out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "GenericPrefixOracle").CombinedOutput()
					if e != nil {
						t.Fatalf("verified oracle %v\n%s", e, out)
					}
					return string(out)
				}
				want := run(dir)
				resolve := resolverFromClasses(classMapFromDir(t, dir))
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					t.Run(string(mode), func(t *testing.T) {
						var result DecompileResult
						var e error
						if mode == "legacy" {
							result.Source, e = DecompileWithResolver(obj.Bytes(), resolve)
						} else {
							result, e = DecompileWithOptions(obj.Bytes(), DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
						}
						if e != nil {
							t.Fatal(e)
						}
						if len(result.StubMethods) > 0 || strings.Contains(result.Source, "yak-decompiler:") {
							t.Fatalf("unimplemented generic prefix\n%s", result.Source)
						}
						rebuilt := t.TempDir()
						path := filepath.Join(rebuilt, "GenericPrefixConsumer.java")
						if e = os.WriteFile(path, []byte(result.Source), 0600); e != nil {
							t.Fatal(e)
						}
						if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, path).CombinedOutput(); e != nil {
							t.Fatalf("rebuild %v\n%s\n%s", e, out, result.Source)
						}
						for repeat := 0; repeat < 2; repeat++ {
							if got := run(rebuilt + string(os.PathListSeparator) + dir); got != want {
								t.Fatalf("output %q want%q", got, want)
							}
						}
					})
				}
			})
		}
	}
}

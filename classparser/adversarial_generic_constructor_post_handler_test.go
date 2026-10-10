package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The receiver-free original prefix remains outside a later handler domain.
// Swap equal-sized original instruction packets, leaving every later handler,
// branch and StackMap offset unchanged. Original JVM order, exception identity,
// generic binding and constructor Exceptions metadata remain independent oracles.
func TestAdversarialGenericConstructorPrefixBeforePostInitHandler(t *testing.T) {
	javac, java := t04Tools(t)
	for _, bounded := range []bool{false, true} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(fmt.Sprintf("bounded=%v/%s", bounded, debug), func(t *testing.T) {
				dir := t.TempDir()
				formal := "<T>"
				if bounded {
					formal = "<T extends Number & Comparable<T>>"
				}
				source := `class HandlerPrefixEffects {
 static String trace="";static final RuntimeException prefix=new RuntimeException("prefix"),parent=new RuntimeException("parent"),body=new RuntimeException("body");
 static void check(Object marker,int mode){trace+="P";if(marker==null||mode==1)throw prefix;}
}
class HandlerPrefixParent<T> {final T value;HandlerPrefixParent(T value,int mode){HandlerPrefixEffects.trace+="S";if(mode==2)throw HandlerPrefixEffects.parent;this.value=value;}}
public class HandlerPrefixConsumer` + formal + ` extends HandlerPrefixParent<T> {
 public HandlerPrefixConsumer(T first,Object marker,int mode){super(first,mode);HandlerPrefixEffects.check(marker,mode);try{HandlerPrefixEffects.trace+="B";if(mode==3)throw HandlerPrefixEffects.body;}catch(RuntimeException caught){if(caught!=HandlerPrefixEffects.body)throw caught;HandlerPrefixEffects.trace+="C";}}
}
class HandlerPrefixOracle {public static void main(String[]args)throws Exception {
 java.lang.reflect.Constructor<?> ctor=HandlerPrefixConsumer.class.getDeclaredConstructors()[0];
 if(ctor.getExceptionTypes().length!=0)throw new AssertionError("changed checked ABI");
 for(Object value:new Object[]{null,Integer.valueOf(91)})for(Object marker:new Object[]{null,new Object()})for(int mode=0;mode<4;mode++){
  HandlerPrefixEffects.trace="";Object result=null;Throwable failure=null;try{result=ctor.newInstance(value,marker,mode);}catch(java.lang.reflect.InvocationTargetException wrapped){failure=wrapped.getCause();}
  Throwable expected=marker==null||mode==1?HandlerPrefixEffects.prefix:mode==2?HandlerPrefixEffects.parent:null;
  String trace=expected==HandlerPrefixEffects.prefix?"P":expected==HandlerPrefixEffects.parent?"PS":mode==3?"PSBC":"PSB";
  if(failure!=expected||!trace.equals(HandlerPrefixEffects.trace)||(result!=null&&((HandlerPrefixParent<?>)result).value!=value))throw new AssertionError("generic binding/identity/order");
  System.out.println((value==null)+":"+(marker==null)+":"+mode+":"+trace+":"+(failure==null?"ok":failure.getMessage()));
 }
}}
`
				file := filepath.Join(dir, "HandlerPrefixConsumer.java")
				if e := os.WriteFile(file, []byte(source), 0600); e != nil {
					t.Fatal(e)
				}
				if out, e := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); e != nil {
					t.Fatalf("original %v\n%s", e, out)
				}
				obj, e := Parse(readClassBytes(t, dir, "HandlerPrefixConsumer"))
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
							if len(old) < 11 || old[0] != 0x2a || old[1] != 0x2b || old[2] != 0x1d || old[3] != 0xb7 || old[6] != 0x2c || old[7] != 0x1d || old[8] != 0xb8 || len(code.ExceptionTable) == 0 {
								t.Fatalf("unexpected constructor %x", old)
							}
							for _, handler := range code.ExceptionTable {
								if handler == nil || handler.StartPc < 11 || handler.HandlerPc < 11 || handler.StartPc >= handler.EndPc || int(handler.EndPc) > len(old) {
									t.Fatal("original handler does not lie wholly beyond swapped packets", handler)
								}
							}
							code.Code = append(append(append([]byte{}, old[6:11]...), old[:6]...), old[11:]...)
							changed = true
						}
					}
				}
				if !changed {
					t.Fatal("no original constructor")
				}
				if e = os.WriteFile(filepath.Join(dir, "HandlerPrefixConsumer.class"), obj.Bytes(), 0600); e != nil {
					t.Fatal(e)
				}
				run := func(cp string) string {
					out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "HandlerPrefixOracle").CombinedOutput()
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
						path := filepath.Join(rebuilt, "HandlerPrefixConsumer.java")
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

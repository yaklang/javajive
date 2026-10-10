package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exceptions attributes do not constrain JVM execution. Preserve the exact
// original constructor ABI while a checked producer outside the later handler
// escapes with its original object; the later body alone catches its own error.
func TestAdversarialCheckedConstructorOperandBeforePostInitHandler(t *testing.T) {
	javac, java := t04Tools(t)
	for _, renamed := range []bool{false, true} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(fmt.Sprintf("renamed=%v/%s", renamed, debug), func(t *testing.T) {
				dir := t.TempDir()
				target, ops := "CheckedHandlerConsumer", "CheckedHandlerOps"
				if renamed {
					target, ops = "OtherHandlerConsumer", "OtherHandlerOps"
				}
				units := map[string]string{
					"CheckedHandlerOps": `public class CheckedHandlerOps {
 public static String trace="";public static final java.io.IOException checked=new java.io.IOException("operand");public static final RuntimeException parent=new RuntimeException("parent"),body=new RuntimeException("body");
 public static Object value(Object v,int mode)throws java.io.IOException{trace+="A";if(mode==1)throw checked;return v;}
}
`,
					"CheckedHandlerConsumer": `class CheckedHandlerParent {final Object value;CheckedHandlerParent(Object v,int mode){CheckedHandlerOps.trace+="S";if(mode==2)throw CheckedHandlerOps.parent;value=v;}}
public class CheckedHandlerConsumer extends CheckedHandlerParent {
 public CheckedHandlerConsumer(Object value,int mode)throws java.io.IOException{super(CheckedHandlerOps.value(value,mode),mode);try{CheckedHandlerOps.trace+="B";if(mode==3)throw CheckedHandlerOps.body;}catch(RuntimeException caught){if(caught!=CheckedHandlerOps.body)throw caught;CheckedHandlerOps.trace+="C";}}
}
`,
					"CheckedHandlerOracle": `public class CheckedHandlerOracle {
 public static void main(String[]args)throws Exception{
 java.lang.reflect.Constructor<?> ctor=Class.forName(args[0]).getDeclaredConstructor(Object.class,int.class);if(ctor.getExceptionTypes().length!=0)throw new AssertionError("constructor checked ABI");
 Object token=new Object();for(Object value:new Object[]{null,token})for(int mode=0;mode<4;mode++){
 CheckedHandlerOps.trace="";Object result=null;Throwable error=null;try{result=ctor.newInstance(value,mode);}catch(java.lang.reflect.InvocationTargetException wrapped){error=wrapped.getCause();}
 Throwable want=mode==1?CheckedHandlerOps.checked:mode==2?CheckedHandlerOps.parent:null;String trace=mode==1?"A":mode==2?"AS":mode==3?"ASBC":"ASB";
 if(error!=want||!trace.equals(CheckedHandlerOps.trace)||(result!=null&&((CheckedHandlerParent)result).value!=value))throw new AssertionError("operand identity/order/domain");
 System.out.println((value==token)+":"+mode+":"+trace+":"+(error==null?"ok":error.getMessage()));
 }
 }
}
`,
				}
				files := []string{}
				for name, source := range units {
					source = strings.ReplaceAll(strings.ReplaceAll(source, "CheckedHandlerConsumer", target), "CheckedHandlerOps", ops)
					name = strings.ReplaceAll(strings.ReplaceAll(name, "CheckedHandlerConsumer", target), "CheckedHandlerOps", ops)
					path := filepath.Join(dir, name+".java")
					if err := os.WriteFile(path, []byte(source), 0600); err != nil {
						t.Fatal(err)
					}
					files = append(files, path)
				}
				args := append([]string{"-proc:none", "--release", "8", debug, "-d", dir}, files...)
				if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
					t.Fatalf("original %v\n%s", err, out)
				}
				obj, err := Parse(readClassBytes(t, dir, target))
				if err != nil {
					t.Fatal(err)
				}
				removed := 0
				for _, method := range obj.Methods {
					name, _ := obj.getUtf8(method.NameIndex)
					if name != "<init>" {
						continue
					}
					attrs := []AttributeInfo{}
					for _, attribute := range method.Attributes {
						if _, ok := attribute.(*ExceptionsAttribute); ok {
							removed++
							continue
						}
						attrs = append(attrs, attribute)
					}
					method.Attributes = attrs
				}
				if removed != 1 {
					t.Fatal("constructor exception fixture changed", removed)
				}
				if err = os.WriteFile(filepath.Join(dir, target+".class"), obj.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				run := func(cp string) string {
					out, err := exec.Command(java, "-Xverify:all", "-cp", cp, "CheckedHandlerOracle", target).CombinedOutput()
					if err != nil {
						t.Fatalf("verified oracle %v\n%s", err, out)
					}
					return string(out)
				}
				want := run(dir)
				if len(strings.Split(strings.TrimSpace(want), "\n")) != 8 {
					t.Fatal("missing original rows", want)
				}
				resolve := resolverFromClasses(classMapFromDir(t, dir))
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					t.Run(string(mode), func(t *testing.T) {
						var result DecompileResult
						var err error
						if mode == "legacy" {
							result.Source, err = DecompileWithResolver(obj.Bytes(), resolve)
						} else {
							result, err = DecompileWithOptions(obj.Bytes(), DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
						}
						if err != nil {
							t.Fatal(err)
						}
						if len(result.StubMethods) > 0 || strings.Contains(result.Source, "yak-decompiler:") {
							t.Fatalf("unsupported checked operand\n%s", result.Source)
						}
						rebuilt := t.TempDir()
						source := filepath.Join(rebuilt, target+".java")
						if err = os.WriteFile(source, []byte(result.Source), 0600); err != nil {
							t.Fatal(err)
						}
						if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, source).CombinedOutput(); err != nil {
							t.Fatalf("rebuild %v\n%s\n%s", err, out, result.Source)
						}
						if got := run(rebuilt + string(os.PathListSeparator) + dir); got != want {
							t.Fatalf("rebuilt %q want %q", got, want)
						}
					})
				}
			})
		}
	}
}

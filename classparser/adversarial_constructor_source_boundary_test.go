package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// JVM permits parameter effects before invokespecial. Java source must express
// those effects in delegation argument evaluation, preserving callee ordering,
// exception identity, original constructor descriptor and declared Exceptions.
func TestAdversarialConstructorSourceBoundaryRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const driver = `import java.lang.reflect.*;
public class ReviewCtorPrefixDriver {
 public static void main(String[]args)throws Throwable{
  Class target=Class.forName(args[0]),state=Class.forName(args[1]);
  Constructor ctor=target.getDeclaredConstructor(Object.class,Object.class,int.class);System.out.println("throws:"+ctor.getExceptionTypes().length);
  Field trace=state.getDeclaredField("trace"),runtime=state.getDeclaredField("runtime"),checked=state.getDeclaredField("checked");trace.setAccessible(true);runtime.setAccessible(true);checked.setAccessible(true);
  Field first=target.getDeclaredField("first");first.setAccessible(true);Object marker=new Object();
  for(Object a:new Object[]{null,marker})for(Object b:new Object[]{null,marker})for(int mode=0;mode<Integer.parseInt(args[2]);mode++){
   trace.set(null,"");try{Object result=ctor.newInstance(a,b,mode);System.out.println("ok:"+(first.get(result)==a)+":"+trace.get(null));}catch(InvocationTargetException wrapped){Throwable x=wrapped.getCause();System.out.println(x.getClass().getName()+":"+(x==runtime.get(null))+":"+(x==checked.get(null))+":"+trace.get(null));}
  }
 }
}
`
	for _, tc := range []struct {
		name, state, kind, source string
		modes                     int
	}{
		{"CtorPrefixReview", "CtorPrefixState", "object", `class CtorPrefixState {
 static String trace=""; static final RuntimeException runtime=new IllegalStateException("prefix");static final Exception checked=new Exception("body");
 static void check(Object value,String name){trace+=name;if(value==null)throw runtime;}
 static void finish(int mode)throws Throwable{trace+="F";if(mode==1)throw checked;if(mode==2)throw runtime;}
}
public class CtorPrefixReview {
 final Object first;
 public CtorPrefixReview(Object first,Object second,int mode)throws Throwable {
  super();CtorPrefixState.check(first,"A");CtorPrefixState.check(second,"B");CtorPrefixState.finish(mode);this.first=first;
 }
 public static void main(String[]args){Object marker=new Object();for(Object a:new Object[]{null,marker})for(Object b:new Object[]{null,marker})for(int mode=0;mode<3;mode++){
  CtorPrefixState.trace="";try{CtorPrefixReview result=new CtorPrefixReview(a,b,mode);System.out.println("ok:"+(result.first==a)+":"+CtorPrefixState.trace);}catch(Throwable x){System.out.println((x==CtorPrefixState.runtime)+":"+(x==CtorPrefixState.checked)+":"+CtorPrefixState.trace);}
 }
 }
}
`, 3},
		{"CtorPrefixBaseReview", "CtorPrefixState", "base", `class CtorPrefixState {
 static String trace=""; static final RuntimeException runtime=new IllegalStateException("prefix");static final Exception checked=new Exception("body");
 static void check(Object value,String name){trace+=name;if(value==null)throw runtime;}
 static void finish(int mode)throws Throwable{trace+="F";if(mode==1)throw checked;if(mode==2)throw runtime;}
}
class CtorPrefixBase {CtorPrefixBase(int mode){CtorPrefixState.trace+="S";if(mode==3)throw CtorPrefixState.runtime;}}
public class CtorPrefixBaseReview extends CtorPrefixBase {
 final Object first;
 public CtorPrefixBaseReview(Object first,Object second,int mode)throws Throwable {
  super(mode);CtorPrefixState.check(first,"A");CtorPrefixState.check(second,"B");CtorPrefixState.finish(mode);this.first=first;
 }
 public static void main(String[]args){Object marker=new Object();for(Object a:new Object[]{null,marker})for(Object b:new Object[]{null,marker})for(int mode=0;mode<4;mode++){
  CtorPrefixState.trace="";try{CtorPrefixBaseReview result=new CtorPrefixBaseReview(a,b,mode);System.out.println("ok:"+(result.first==a)+":"+CtorPrefixState.trace);}catch(Throwable x){System.out.println((x==CtorPrefixState.runtime)+":"+(x==CtorPrefixState.checked)+":"+CtorPrefixState.trace);}
 }
 }
}
`, 4},
		{"CtorPrefixThisReview", "CtorPrefixThisState", "this", `class CtorPrefixThisState {
 static String trace="";static final RuntimeException runtime=new IllegalStateException("prefix");static final Exception checked=new Exception("argument");
 static Object make(Object first,Object second,int mode)throws Throwable{trace+="A";if(first==null)throw runtime;trace+="B";if(second==null)throw runtime;trace+="M";if(mode==1)throw checked;if(mode==2)throw runtime;return first;}
}
public class CtorPrefixThisReview {
 final Object first;
 public CtorPrefixThisReview(Object first,Object second,int mode)throws Throwable{this(CtorPrefixThisState.make(first,second,mode),mode);}
 private CtorPrefixThisReview(Object first,int mode){this.first=first;CtorPrefixThisState.trace+="S";}
 public static void main(String[]args){Object marker=new Object();for(Object a:new Object[]{null,marker})for(Object b:new Object[]{null,marker})for(int mode=0;mode<3;mode++){
  CtorPrefixThisState.trace="";try{CtorPrefixThisReview result=new CtorPrefixThisReview(a,b,mode);System.out.println("ok:"+(result.first==a)+":"+CtorPrefixThisState.trace);}catch(Throwable x){System.out.println((x==CtorPrefixThisState.runtime)+":"+(x==CtorPrefixThisState.checked)+":"+CtorPrefixThisState.trace);}
 }
 }
}
`, 3},
		{"CtorPrefixOrderReview", "CtorPrefixOrderState", "order", `class CtorPrefixOrderState {
 static String trace=""; static final RuntimeException runtime=new IllegalStateException("prefix");static final Exception checked=new Exception("body");
 static void check(Object value,String name){trace+=name;if(value==null)throw runtime;}
 static void finish(int mode){trace+="F";if(mode==1)throw runtime;if(mode==2)throw runtime;}
}
class CtorPrefixOrderBase {CtorPrefixOrderBase(int mode){CtorPrefixOrderState.trace+="S";if(mode==3)throw CtorPrefixOrderState.runtime;}}
public class CtorPrefixOrderReview extends CtorPrefixOrderBase {
 final Object first;
 public CtorPrefixOrderReview(Object first,Object second,int mode) {
  super(mode);CtorPrefixOrderState.check(first,"A");CtorPrefixOrderState.check(second,"B");CtorPrefixOrderState.finish(mode);this.first=first;
 }
 public static void main(String[]args){Object marker=new Object();for(Object a:new Object[]{null,marker})for(Object b:new Object[]{null,marker})for(int mode=0;mode<4;mode++){
  CtorPrefixOrderState.trace="";try{CtorPrefixOrderReview result=new CtorPrefixOrderReview(a,b,mode);System.out.println("ok:"+(result.first==a)+":"+CtorPrefixOrderState.trace);}catch(Throwable x){System.out.println((x==CtorPrefixOrderState.runtime)+":"+(x==CtorPrefixOrderState.checked)+":"+CtorPrefixOrderState.trace);}
 }
 }
}
`, 4},
	} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(tc.kind+"/"+debug, func(t *testing.T) {
				dir := t.TempDir()
				src := filepath.Join(dir, tc.name+".java")
				driverFile := filepath.Join(dir, "ReviewCtorPrefixDriver.java")
				for f, text := range map[string]string{src: tc.source, driverFile: driver} {
					if err := os.WriteFile(f, []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, src, driverFile).CombinedOutput(); err != nil {
					t.Fatalf("original %v\n%s", err, out)
				}
				file := filepath.Join(dir, tc.name+".class")
				obj, err := Parse(readClassBytes(t, dir, tc.name))
				if err != nil {
					t.Fatal(err)
				}
				pool := NewConstantPoolWithConstant(&obj.ConstantPool)
				changed := false
				for _, m := range obj.Methods {
					if pool.GetUtf8(int(m.NameIndex)).Value != "<init>" || pool.GetUtf8(int(m.DescriptorIndex)).Value != "(Ljava/lang/Object;Ljava/lang/Object;I)V" {
						continue
					}
					attrs := []AttributeInfo{}
					removed := 0
					for _, a := range m.Attributes {
						if _, ok := a.(*ExceptionsAttribute); ok {
							removed++
							continue
						}
						if code, ok := a.(*CodeAttribute); ok && tc.kind != "this" {
							old := append([]byte(nil), code.Code...)
							if len(old) != 26 && len(old) != 27 || old[0] != 0x2a || old[1] != 0xb7 && old[2] != 0xb7 || len(code.ExceptionTable) != 0 {
								t.Fatalf("constructor fixture changed: %x", old)
							}
							cut := 4
							if old[1] != 0xb7 {
								cut = 5
							}
							end := cut + 12
							code.Code = append(append(append([]byte{}, old[cut:end]...), old[:cut]...), old[end:]...)
						}
						attrs = append(attrs, a)
					}
					if tc.kind == "order" && removed != 0 || tc.kind != "order" && removed != 1 {
						t.Fatal("constructor Exceptions fixture changed")
					}
					m.Attributes = attrs
					changed = true
				}
				if !changed {
					t.Fatal("original constructor absent")
				}
				if err = os.WriteFile(file, obj.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				want := constructorBoundaryOracle(tc.kind, tc.modes)
				run := func(cp string) string {
					out, e := exec.Command(java, "-Xverify:all", "-cp", cp, "ReviewCtorPrefixDriver", tc.name, tc.state, fmt.Sprint(tc.modes)).CombinedOutput()
					if e != nil {
						t.Fatalf("verified runtime %v\n%s", e, out)
					}
					return string(out)
				}
				if got := run(dir); got != want {
					t.Fatalf("original oracle got %q want %q", got, want)
				}
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
							t.Fatalf("unsupported\n%s", result.Source)
						}
						again, e := DecompileWithResolver(obj.Bytes(), resolve)
						if mode == "legacy" && (e != nil || again != result.Source) {
							t.Fatal("constructor helper order is not deterministic")
						}
						rebuilt := t.TempDir()
						path := filepath.Join(rebuilt, tc.name+".java")
						if e = os.WriteFile(path, []byte(result.Source), 0600); e != nil {
							t.Fatal(e)
						}
						if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, path).CombinedOutput(); e != nil {
							t.Fatalf("rebuild %v\n%s\n%s", e, out, result.Source)
						}
						if got := run(rebuilt + string(os.PathListSeparator) + dir); got != want {
							t.Fatalf("rebuilt %q want %q\n%s", got, want, result.Source)
						}
					})
				}
			})
		}
	}
}

// The oracle is independently specified from effect order and exception owners;
// it does not parse or derive expectations from generated source.
func constructorBoundaryOracle(kind string, modes int) string {
	var out strings.Builder
	out.WriteString("throws:0\n")
	for a := 0; a < 2; a++ {
		for b := 0; b < 2; b++ {
			for mode := 0; mode < modes; mode++ {
				trace, exception := "A", ""
				if a == 0 {
					exception = "java.lang.IllegalStateException:true:false"
				} else {
					trace += "B"
					if b == 0 {
						exception = "java.lang.IllegalStateException:true:false"
					} else {
						if kind == "this" {
							trace += "M"
						} else if kind == "base" || kind == "order" {
							trace += "S"
						}
						if (kind == "base" || kind == "order") && mode == 3 {
							exception = "java.lang.IllegalStateException:true:false"
						} else {
							if kind != "this" {
								trace += "F"
							}
							if mode == 1 && kind != "order" {
								exception = "java.lang.Exception:false:true"
							} else if mode == 1 || mode == 2 {
								exception = "java.lang.IllegalStateException:true:false"
							} else if kind == "this" {
								trace += "S"
							}
						}
					}
				}
				if exception == "" {
					fmt.Fprintf(&out, "ok:true:%s\n", trace)
				} else {
					fmt.Fprintf(&out, "%s:%s\n", exception, trace)
				}
			}
		}
	}
	return out.String()
}

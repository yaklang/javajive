package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Rebuild the entire trusted class family: leaving the original nested/helper
// classes on the execution path would conceal inaccessible flattened calls.
func TestAdversarialPrivateNestBridgeRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `
public class PrivateNestBridgeReview<T extends CharSequence> {
 static String trace="";static RuntimeException failure=new IllegalArgumentException("failure");
 private Object pick(Object value,int mark){trace+="P;";return value;}
 private Object pick(String value,int mark){trace+="WRONG;";return new Object();}
 private T generic(T value){trace+="G;";return value;}
 private Object checked(Object value,java.io.IOException failure)throws java.io.IOException{trace+="C;";if(failure!=null)throw failure;return value;}
 private void touch(Object value){trace+="V;";}
 private static Object staticPick(Object value){trace+="S;";return value;}
 static Object jdec$private$0(Object value){return value;}
 static class Child extends PrivateNestBridgeReview<String>{public Object pick(Object value,int mark){trace+="OVERRIDE;";return new Object();}}
 static class Dormant {static{trace+="INIT;";}private Object pick(Object value){trace+="D;";return value;}}
 static class Reader {
  static Object dormant(Dormant owner,Object value){return owner.pick(value);}
  static Object read(PrivateNestBridgeReview<?> receiver,Object value,int mark){return receiver.pick(value,mark);}
  static String generic(PrivateNestBridgeReview<String> receiver,String value){return receiver.generic(value);}
  static Object checked(PrivateNestBridgeReview<?> receiver,Object value,java.io.IOException error){try{return receiver.checked(value,error);}catch(java.io.IOException caught){trace+="H;";return caught;}}
  static void touch(PrivateNestBridgeReview<?> receiver,Object value){receiver.touch(value);}
  static Object staticRead(Object value){return PrivateNestBridgeReview.staticPick(value);}
 }
 static PrivateNestBridgeReview<?> receiver(int mode,PrivateNestBridgeReview<?> owner){trace+="R;";if(mode==1)throw failure;return mode==2||mode==5?null:owner;}
 static Object arg(int mode,Object value){trace+="A;";if(mode==3||mode==5)throw failure;return value;}
 static int mark(int mode){trace+="M;";if(mode==4)throw failure;return 9;}
 static String run(int mode,PrivateNestBridgeReview<?> owner,Object value){trace="";try{Object result=Reader.read(receiver(mode,owner),arg(mode,value),mark(mode));return "return:"+(result==value)+":"+trace;}catch(Throwable caught){return "throw:"+(caught==failure)+":"+caught.getClass().getSimpleName()+":"+trace;}}
 public static void main(String[] args){Object token=new Object();trace="";try{Reader.dormant(null,arg(0,token));}catch(NullPointerException caught){System.out.println("dormant:"+trace);}for(PrivateNestBridgeReview<?> owner:new PrivateNestBridgeReview<?>[]{new PrivateNestBridgeReview<String>(),new Child()})for(int mode=0;mode<6;mode++)System.out.println(run(mode,owner,token));System.out.println(run(0,new Child(),null));String text=new String("same");trace="";System.out.println((Reader.generic(new Child(),text)==text)+":"+trace);java.io.IOException checked=new java.io.IOException("checked");trace="";System.out.println((Reader.checked(new Child(),token,checked)==checked)+":"+trace);trace="";System.out.println((Reader.checked(new Child(),token,null)==token)+":"+trace);trace="";Reader.touch(new Child(),token);System.out.println(trace);trace="";System.out.println((Reader.staticRead(token)==token)+":"+trace);}
}
`
	for _, release := range []string{"8", "11", "17"} {
		for _, debug := range []string{"-g", "-g:none"} {
			t.Run(release+"/"+debug, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "PrivateNestBridgeReview.java")
				if err := os.WriteFile(path, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", release, debug, "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatalf("original: %v\n%s", err, out)
				}
				want := t04RunJava(t, java, dir, "PrivateNestBridgeReview")
				dormant := "dormant:A;"
				if release == "8" {
					dormant += "INIT;"
				}
				if !strings.Contains(want, dormant) || release != "8" && strings.Contains(want, "INIT;") || strings.Contains(want, "WRONG") || strings.Contains(want, "OVERRIDE") || !strings.Contains(want, "NullPointerException:R;A;M;") || !strings.Contains(want, "true:C;H;") {
					t.Fatalf("independent oracle invalid: %s", want)
				}
				resolve := func(name string) ([]byte, bool) {
					b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".class"))
					return b, e == nil
				}
				units, err := filepath.Glob(filepath.Join(dir, "*.class"))
				if err != nil {
					t.Fatal(err)
				}
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					rebuilt := t.TempDir()
					args := []string{"-proc:none", "--release", release, "-d", rebuilt}
					allSource := ""
					for _, unit := range units {
						raw, e := os.ReadFile(unit)
						if e != nil {
							t.Fatal(e)
						}
						var result DecompileResult
						if mode == "legacy" {
							result.Source, e = DecompileWithResolver(raw, resolve)
						} else {
							result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolve})
						}
						if e != nil {
							t.Fatal(e)
						}
						if len(result.StubMethods) > 0 {
							t.Fatalf("%s stub: %v", unit, result.StubMethods)
						}
						name := strings.TrimSuffix(filepath.Base(unit), ".class")
						p := filepath.Join(rebuilt, name+".java")
						if e = os.WriteFile(p, []byte(result.Source), 0600); e != nil {
							t.Fatal(e)
						}
						args = append(args, p)
						allSource += result.Source
					}
					if !strings.Contains(allSource, "private Object pick(") {
						t.Fatalf("private dispatch target was widened: %s", allSource)
					}
					if release != "8" && !strings.Contains(allSource, "jdec$private$") {
						t.Fatal("direct nest invocation has no proven bridge")
					}
					if out, e := exec.Command(javac, args...).CombinedOutput(); e != nil {
						t.Fatalf("rebuild %s: %v\n%s\n%s", mode, e, out, allSource)
					}
					if got := t04RunJava(t, java, rebuilt, "PrivateNestBridgeReview"); got != want {
						t.Fatalf("%s: got %s want %s\n%s", mode, got, want, allSource)
					}
				}
			})
		}
	}
}

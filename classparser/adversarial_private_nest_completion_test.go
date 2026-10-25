package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const privateNestCompletionFixture = `
public class NestOwner {
 private Object pick(long n,double d,Object value,Object[] array)throws java.io.IOException {
  NestEffects.trace+="P;";if(NestEffects.fail)throw NestEffects.error;
  if(n!=Long.MIN_VALUE||Double.doubleToRawLongBits(d)!=Long.MIN_VALUE||array[0]!=value)throw new AssertionError("typed operands");return value;
 }
 private Object pick(long n,double d,String value,Object[] array){throw new AssertionError("overload retargeted");}
 static class Child extends NestOwner {public Object pick(long n,double d,Object value,Object[] array){throw new AssertionError("private target became virtual");}}
 static class Reader {
  static Object read(NestOwner receiver,int mode,Object value)throws java.io.IOException{return receiver.pick(NestEffects.word(mode),NestEffects.fraction(mode),NestEffects.value(mode,value),NestEffects.array(mode,value));}
 }
}
class NestEffects {
 static String trace="";static boolean fail;static final java.io.IOException error=new java.io.IOException("checked identity");static final RuntimeException failure=new IllegalArgumentException("operand identity");
 static long word(int m){trace+="W;";if(m==1)throw failure;return Long.MIN_VALUE;}
 static double fraction(int m){trace+="D;";if(m==2)throw failure;return -0.0d;}
 static Object value(int m,Object v){trace+="V;";if(m==3)throw failure;return v;}
 static Object[] array(int m,Object v){trace+="A;";if(m==4)throw failure;return new Object[]{v};}
}
class NestDriver {
 public static void main(String[]args)throws Exception{
  if(NestOwner.class.getDeclaredMethods().length!=2||NestOwner.Reader.class.getDeclaredMethods().length!=1||NestOwner.Child.class.getDeclaredMethods().length!=1)throw new AssertionError("original declaration universe");
  if(NestOwner.Reader.class.getNestHost()!=NestOwner.class||NestOwner.Child.class.getNestHost()!=NestOwner.class||NestOwner.class.getNestMembers().length!=3)throw new AssertionError("original reciprocal nest");
  Object token=new Object();int rows=0;
  for(NestOwner h:new NestOwner[]{new NestOwner(),new NestOwner.Child(),null})for(Object v:new Object[]{null,token,"text"})for(int mode=0;mode<=4;mode++)for(boolean fail:new boolean[]{false,true}){
   NestEffects.trace="";NestEffects.fail=fail;Throwable caught=null;Object result=null;
   try{result=NestOwner.Reader.read(h,mode,v);}catch(Throwable t){caught=t;}
   String trace=mode==1?"W;":mode==2?"W;D;":mode==3?"W;D;V;":"W;D;V;A;";
   if(mode!=0){if(caught!=NestEffects.failure)throw new AssertionError("operand failure");}
   else if(h==null){if(!(caught instanceof NullPointerException))throw new AssertionError("null receiver after all operands");}
   else{trace+="P;";if(fail?caught!=NestEffects.error:caught!=null||result!=v)throw new AssertionError("private binding, checked exception or identity");}
   if(!trace.equals(NestEffects.trace))throw new AssertionError("effect order: "+NestEffects.trace);rows++;
  }
  System.out.println(rows+":nest:private:universe:operands:dispatch");
 }
}
`

func TestAdversarialPrivateNestCompletionRetainsOriginalDeclarationsAndCalls(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	javac, java := t04Tools(t)
	for _, root := range []string{"NestOwner", "RenamedPrivateOwner", "MutualPrivateOwner", "NativePrivateOwner", "CachedPrivateOwner", "AnonymousNativeOwner", "ΩPrivateOwner"} {
		for _, release := range []int{11, 17} {
			// The cached-layout counterexample uses javac11's source adapter
			// family. The javac17 sample takes a different admission route.
			if root == "CachedPrivateOwner" && release != 11 {
				continue
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/java%d/%s", root, release, debug), func(t *testing.T) {
					if root == "NativePrivateOwner" || root == "CachedPrivateOwner" || root == "AnonymousNativeOwner" {
						t.Setenv("JDEC_NATIVE_MEMBER_OFF", "")
					}
					fixture := privateNestCompletionFixture
					if root == "AnonymousNativeOwner" {
						fixture = strings.ReplaceAll(fixture, "public class NestOwner {", "public class NestOwner { static Runnable extra(){return new Runnable(){public void run(){NestEffects.trace+=\"R;\";}};}")
						fixture = strings.ReplaceAll(fixture, "NestOwner.class.getDeclaredMethods().length!=2", "NestOwner.class.getDeclaredMethods().length!=3")
						fixture = strings.ReplaceAll(fixture, "NestOwner.class.getNestMembers().length!=3", "NestOwner.class.getNestMembers().length!=4")
						fixture = strings.ReplaceAll(fixture, "Object token=new Object();", "NestEffects.trace=\"\";NestOwner.extra().run();if(!NestEffects.trace.equals(\"R;\"))throw new AssertionError(\"anonymous effect\");Object token=new Object();")
					}
					if root == "MutualPrivateOwner" {
						// The private-call graph is cyclic between distinct declaring
						// classes. Both temporary helper families must disappear.
						fixture = strings.ReplaceAll(fixture, "return value;", "return new Reader().confirm(value);")
						fixture = strings.ReplaceAll(fixture, "static class Reader {", "static class Reader { private Object confirm(Object value){return value;}")
						fixture = strings.ReplaceAll(fixture, "NestOwner.Reader.class.getDeclaredMethods().length!=1", "NestOwner.Reader.class.getDeclaredMethods().length!=2")
					}
					fixture = strings.ReplaceAll(fixture, "NestOwner", root)
					if root == "ΩPrivateOwner" {
						fixture = strings.ReplaceAll(fixture, "pick(", "受取(")
					}
					dir := t.TempDir()
					p := filepath.Join(dir, root+".java")
					if err := os.WriteFile(p, []byte(fixture), 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", fmt.Sprint(release), "-g:"+debug, "-d", dir, p).CombinedOutput(); err != nil {
						t.Fatal("authored original", err, string(log))
					}
					run := func(path string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							out, err := enumFreshOracleRun(t, java, mode, path, "NestDriver")
							if err != nil || string(out) != "90:nest:private:universe:operands:dispatch\n" {
								t.Fatal("independent nest oracle", err, string(out))
							}
						}
					}
					run(dir)
					files := map[string][]byte{}
					owned := map[string][]byte{}
					entries, err := filepath.Glob(filepath.Join(dir, "*.class"))
					if err != nil {
						t.Fatal(err)
					}
					for _, f := range entries {
						b, e := os.ReadFile(f)
						if e != nil {
							t.Fatal(e)
						}
						n := filepath.Base(f)
						files[n] = b
						if n == root+".class" || strings.HasPrefix(n, root+"$") {
							owned[n] = b
						}
					}
					input := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(input, t23Zip(t, owned), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(input, release, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k })
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					if root == "CachedPrivateOwner" {
						object := mustParseLowering(t, owned[root+".class"])
						cached := a.source.jar.nativeMemberEntry(object)
						if cached == nil || cached.family == nil {
							t.Fatal("original native family was not cached")
						}
						// A later speculative physical layout may be fully rendered,
						// but is never returned. It must not replace the proof for
						// the already cached complete owning compilation unit.
						reader := a.source.jar.nativeMemberReader(mustParseLowering(t, owned[root+".class"]))
						reader.options.EnvSnapshot = map[string]string{"JDEC_NATIVE_MEMBER_OFF": "1"}
						provisional, err := reader.DumpClass()
						if err != nil || provisional == cached.source {
							t.Fatal("independent provisional source layout", err)
						}
					}
					external := map[string][]byte{}
					for n, b := range files {
						if owned[n] == nil {
							external[n] = b
						}
					}
					output := sourceArtifactWriteFiles(t, external)
					args := []string{"-proc:none", "--release", fmt.Sprint(release), "-g:" + debug, "-cp", output, "-sourcepath", output, "-d", output}
					for n := range owned {
						source, e := a.ReadFile(n)
						if e != nil || legacySourceHasFailureMarker(string(source)) {
							t.Fatal("returned source", e, string(source))
						}
						path := filepath.Join(output, strings.TrimSuffix(n, ".class")+".java")
						if e := os.WriteFile(path, source, 0600); e != nil {
							t.Fatal(e)
						}
						args = append(args, path)
					}
					if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
						t.Fatal("fresh family", err, string(log))
					}
					if root == "NestOwner" && release == 11 && debug == "none" {
						fresh := map[string][]byte{}
						for n := range owned {
							b, err := os.ReadFile(filepath.Join(output, n))
							if err != nil {
								t.Fatal(err)
							}
							fresh[n] = b
						}
						testPrivateNestCompletionClosure(t, a, owned, fresh)
					}
					for n := range owned {
						path := filepath.Join(output, n)
						fresh, e := os.ReadFile(path)
						if e != nil {
							t.Fatal(e)
						}
						saved := bytes.Clone(fresh)
						done, e := a.FinishClass(n, fresh)
						if e != nil {
							t.Fatal("original private nest contract", n, e)
						}
						if !bytes.Equal(saved, fresh) {
							t.Fatal("input changed")
						}
						again, e := a.FinishClass(n, done)
						if e != nil || !bytes.Equal(again, done) {
							t.Fatal("completion not idempotent", n, e)
						}
						if e := os.WriteFile(path, done, 0600); e != nil {
							t.Fatal(e)
						}
					}
					run(output)
				})
			}
		}
	}
}

package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const constructorMarkerFixture = `public class MarkerHost {
 public static Target make(long n,double d,Object v,boolean fail)throws java.io.IOException {return new Target(n,d,v,fail);}
 public static final class Target {
  final long n;final double d;final Object value;
  private Target(long n,double d,Object v,boolean fail)throws java.io.IOException {
   MarkerEffects.trace+="T";if(fail)throw MarkerEffects.failure;this.n=n;this.d=d;this.value=v;
  }
  public long apply(long x){return x+n;}
  public double bias(){return d;}
  public Object value(){return value;}
 }
}
class MarkerEffects {static String trace="";static final java.io.IOException failure=new java.io.IOException("original");}
class MarkerDriver {public static void main(String[]args)throws Exception {
 int rows=0;Object alias=new Object();
 for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  MarkerHost.Target v=MarkerHost.make(n,-0.0,alias,false);
  if(v.value()!=alias||Double.doubleToRawLongBits(v.bias())!=Long.MIN_VALUE)throw new AssertionError("wide/alias binding");
  for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   if(v.apply(x)!=java.math.BigInteger.valueOf(x).add(java.math.BigInteger.valueOf(n)).longValue())throw new AssertionError("overflow");rows++;
  }
 }
 try{MarkerHost.make(7,0.25,null,true);throw new AssertionError("missing throw");}catch(java.io.IOException e){if(e!=MarkerEffects.failure)throw new AssertionError("failure identity");}
 if(!MarkerEffects.trace.equals("TTTTTT"))throw new AssertionError("constructor effects");
 int privateCount=0,syntheticCount=0;
 for(java.lang.reflect.Constructor<?> c:MarkerHost.Target.class.getDeclaredConstructors()){
  if(c.isSynthetic()){syntheticCount++;if(c.getModifiers()!=0x1000||c.getParameterTypes().length!=5)throw new AssertionError("bridge contract");}
  else {privateCount++;if(!java.lang.reflect.Modifier.isPrivate(c.getModifiers()))throw new AssertionError("private target");}
 }
 if(privateCount!=1||syntheticCount!=1)throw new AssertionError("constructor markers");
 System.out.println(rows+":constructor:synthetic:binding:wide:exception:effects");
}}
`

func TestAdversarialConstructorMarkerPreservesOriginalBinaryCallers(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, root := range []string{"MarkerHost", "ReboundMarkerHost"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(constructorMarkerFixture, "MarkerHost", root)
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, root, debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := enumFreshOracleRun(t, java, mode, dir, "MarkerDriver")
						if err != nil || string(out) != "25:constructor:synthetic:binding:wide:exception:effects\n" {
							t.Fatal("unchanged binary caller", err, string(out))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				owned := map[string][]byte{root + "$Target.class": files[root+"$Target.class"], root + "$1.class": files[root+"$1.class"]}
				jar := filepath.Join(t.TempDir(), "input.jar")
				if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(name string) ([]byte, bool) { b, k := files[name+".class"]; return b, k })
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				external := map[string][]byte{}
				for n, b := range files {
					if _, ok := owned[n]; !ok {
						external[n] = b
					}
				}
				out := sourceArtifactWriteFiles(t, external)
				paths := []string{}
				for _, name := range []string{root + "$1", root + "$Target"} {
					source, err := a.ReadFile(name + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("complete physical source", name, err, string(source))
					}
					path := filepath.Join(out, name+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
				if log, err := exec.Command(modern, append([]string{"-proc:none", "--release", "8", "-g:" + debug, "-cp", out, "-sourcepath", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
					t.Fatal("compile without owned original classes", err, string(log))
				}
				for _, name := range []string{root + "$Target.class", root + "$1.class"} {
					path := filepath.Join(out, name)
					fresh, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(name, fresh)
					if err != nil {
						t.Fatal("complete original constructor contract", name, err)
					}
					if name == root+"$Target.class" {
						artifactLambdaMetadataCodeUnchanged(t, fresh, finished)
						again, err := a.FinishClass(name, finished)
						if err != nil || !bytes.Equal(again, finished) {
							t.Fatal("idempotence", err)
						}
					}
					if err := os.WriteFile(path, finished, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(out)
			})
		}
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const legacyPrivateCallFixture = `public class LegacyCallOwner {
 public static int calls,constructions;private final long seed;
 public LegacyCallOwner(long seed){this(seed,17);}
 private LegacyCallOwner(long seed,int tag){this.seed=seed;constructions+=tag;}
 private static long mix(long left,long right){calls++;return left*31L+right;}
 public long step(long delta){return new Reader().step(this,delta);}
 static class Reader {long step(LegacyCallOwner owner,long delta){return mix(owner.seed,delta);}}
}
class LegacyCallDriver {public static void main(String[]args)throws Exception {
 java.lang.reflect.Method method=LegacyCallOwner.class.getDeclaredMethod("mix",long.class,long.class);
 java.lang.reflect.Constructor<?> ctor=LegacyCallOwner.class.getDeclaredConstructor(long.class,int.class);
 if(!java.lang.reflect.Modifier.isPrivate(method.getModifiers())||!java.lang.reflect.Modifier.isPrivate(ctor.getModifiers()))throw new AssertionError("private method/constructor declarations");
 try{java.lang.invoke.MethodHandles.lookup().findStatic(LegacyCallOwner.class,"mix",java.lang.invoke.MethodType.methodType(long.class,long.class,long.class));throw new AssertionError("private handle became accessible");}catch(IllegalAccessException expected){}
 try{ctor.newInstance(0L,17);throw new AssertionError("private constructor became accessible");}catch(IllegalAccessException expected){}
 int rows=0;LegacyCallOwner.constructions=0;
 for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  LegacyCallOwner owner=new LegacyCallOwner(seed);if(LegacyCallOwner.constructions!=17*(rows/5+1))throw new AssertionError("delegated constructor effect");
  for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   long expected=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(delta)).longValue();int before=LegacyCallOwner.calls;
   if(owner.step(delta)!=expected||LegacyCallOwner.calls!=before+1)throw new AssertionError("private static accessor binding/overflow/effects");rows++;
  }
 }System.out.println(rows+":legacy:private:methods:constructors:accessors:effects");
}}
`

func TestAdversarialLegacyPrivateCallsRetainOriginalAccessAndDelegation(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("original native javac8 is required")
			}
		}
		for _, owner := range []string{"LegacyCallOwner", "RenamedCallOwner"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(string(profile)+"/"+owner+"/"+debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(legacyPrivateCallFixture, "LegacyCallOwner", owner)
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, owner, debug, false)
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							out, err := exec.Command(java, "-Xverify:all", mode, "-cp", dir, "LegacyCallDriver").CombinedOutput()
							if err != nil || string(out) != "25:legacy:private:methods:constructors:accessors:effects\n" {
								t.Fatalf("original-first unchanged binary reader/caller %s: %v\n%s", mode, err, out)
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "owner.jar")
					if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatalf("legacy call owner source: %v\n%s", err, source)
					}
					for _, declaration := range []string{"private static long mix", "private " + owner + "("} {
						if !strings.Contains(string(source), declaration) {
							t.Fatalf("legacy private call declaration was widened: %s\n%s", declaration, source)
						}
					}
					deps := map[string][]byte{}
					for name, raw := range files {
						if name != owner+".class" {
							deps[name] = raw
						}
					}
					out := sourceArtifactWriteFiles(t, deps)
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					args := []string{"-proc:none", "--release", "8", "-g:" + debug}
					if profile == NativeJavac8 {
						args = []string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug}
					}
					args = append(args, "-cp", out, "-sourcepath", out, "-d", out, path)
					if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
						t.Fatal("owner-only source compile", err, string(log))
					}
					fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(owner+".class", fresh)
					if err != nil {
						t.Fatal("complete original field/method/constructor contracts", err)
					}
					if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
						t.Fatal(err)
					}
					run(out)
				})
			}
		}
	}
}

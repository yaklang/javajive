package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nestUnusedPrivateFixture = `public class ClosedPrivateOwner {
 private static long scale=31;private final long bias;
 public ClosedPrivateOwner(long bias){this.bias=bias;}
 private static long product(long value){return value*scale;}
 public long fold(long value){return product(value)+bias;}
 public static class Peer {public static long observe(ClosedPrivateOwner owner,long value){return owner.fold(value);}}
}
class ClosedPrivateOwnerDriver {public static void main(String[]args)throws Throwable {
 for(String name:new String[]{"bias","scale"})if(!java.lang.reflect.Modifier.isPrivate(ClosedPrivateOwner.class.getDeclaredField(name).getModifiers()))throw new AssertionError("field visibility");
 if(!java.lang.reflect.Modifier.isPrivate(ClosedPrivateOwner.class.getDeclaredMethod("product",long.class).getModifiers()))throw new AssertionError("method visibility");
 try{java.lang.invoke.MethodHandles.lookup().findStatic(ClosedPrivateOwner.class,"product",java.lang.invoke.MethodType.methodType(long.class,long.class));throw new AssertionError("external private access widened");}catch(IllegalAccessException expected){}
 int rows=0;for(long bias:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  ClosedPrivateOwner owner=new ClosedPrivateOwner(bias);long expected=java.math.BigInteger.valueOf(value).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(bias)).longValue();
  if(owner.fold(value)!=expected||ClosedPrivateOwner.Peer.observe(owner,value)!=expected)throw new AssertionError("binding/arithmetic");rows++;
 }System.out.println(rows+":nest:private:binding:overflow");
}}
`

func TestAdversarialFlatNestRetainsPrivateMembersWithoutPeerUses(t *testing.T) {
	compiler, java := t04Tools(t)
	for _, owner := range []string{"ClosedPrivateOwner", "RenamedClosedPrivateOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(nestUnusedPrivateFixture, "ClosedPrivateOwner", owner)
				files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, compiler, ModernJavac, fixture, owner, debug, nil, 11, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						log, err := enumFreshOracleRun(t, java, mode, dir, owner+"Driver")
						if err != nil || string(log) != "25:nest:private:binding:overflow\n" {
							t.Fatal("unchanged original-first access observer", err, string(log))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				// Only the host is owned. The original peer is a dependency, not
				// a source ownership token. No lexical folding is available here.
				jar := filepath.Join(t.TempDir(), "host.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
					t.Fatal(err)
				}
				resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
				a, err := NewJarReconstructionArtifactFromLocal(jar, 11, ModernJavac, resolve)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("independent host source", err, string(source))
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
				if log, err := exec.Command(compiler, "-proc:none", "--release", "11", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("source-only host compile", err, string(log))
				}
				fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
				if err != nil {
					t.Fatal(err)
				}
				finished, err := a.FinishClass(owner+".class", fresh)
				if err != nil {
					t.Fatal("preserve private field and method contracts", err)
				}
				if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}

package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const staticHelperFixture = `public class SwitchMain {static long fold(Choice choice,long input){switch(choice){case ALPHA:return input*31+3;case BETA:return input*17+5;case GAMMA:return input*7+11;default:return input*13-19;}}}
enum Choice {ALPHA,BETA,GAMMA}
class HelperDriver {public static void main(String[]args)throws Exception {
 Class<?> helper=Class.forName("SwitchMain$1",false,HelperDriver.class.getClassLoader());
 if(!helper.isSynthetic()||helper.getDeclaredConstructors().length!=0)throw new AssertionError("compiler-introduced source constructor/class marker");
 int rows=0;for(Choice c:Choice.values())for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  long factor,offset;if(c.name().equals("ALPHA")){factor=31;offset=3;}else if(c.name().equals("BETA")){factor=17;offset=5;}else if(c.name().equals("GAMMA")){factor=7;offset=11;}else{factor=13;offset=-19;}
  long want=java.math.BigInteger.valueOf(input).multiply(java.math.BigInteger.valueOf(factor)).add(java.math.BigInteger.valueOf(offset)).longValue();if(SwitchMain.fold(c,input)!=want)throw new AssertionError("switch table/ordinal/overflow");rows++;
 }
 System.out.println(rows+":static:helper:constructor:algorithm");
}}
`

func TestAdversarialStaticHelperHasNoInventedConstructorOrClassMarkerLoss(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, staticHelperFixture, "SwitchMain", debug, false)
			const child = "SwitchMain$1.class"
			if len(files[child]) == 0 {
				t.Fatal("original compiler did not generate the switch helper")
			}
			jar := filepath.Join(t.TempDir(), "static-helper.jar")
			if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{child: files[child]}), 0600); err != nil {
				t.Fatal(err)
			}
			resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, resolve)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			source, err := a.ReadFile(child)
			if err != nil || legacySourceHasFailureMarker(string(source)) {
				t.Fatal("complete static helper source", err, string(source))
			}
			deps := map[string][]byte{}
			for n, b := range files {
				if n != child {
					deps[n] = b
				}
			}
			out := sourceArtifactWriteFiles(t, deps)
			path := filepath.Join(out, "SwitchMain$1.java")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
				t.Fatal("only fresh helper output", err, string(log))
			}
			fresh, err := os.ReadFile(filepath.Join(out, child))
			if err != nil {
				t.Fatal(err)
			}
			finished, err := a.FinishClass(child, fresh)
			if err != nil {
				t.Fatal("source cannot declare absence of a JVM constructor", err)
			}
			for _, world := range []struct{ name, constants, rows string }{{"original", "ALPHA,BETA,GAMMA", "15"}, {"reordered and added", "EXTRA,GAMMA,ALPHA,BETA", "20"}, {"removed", "GAMMA,ALPHA", "10"}} {
				t.Run(world.name, func(t *testing.T) {
					changed := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, "public enum Choice {"+world.constants+"}", "Choice", "none", false)
					for _, side := range []string{"original", "candidate"} {
						runtime := map[string][]byte{}
						for n, b := range files {
							runtime[n] = b
						}
						runtime["Choice.class"] = changed["Choice.class"]
						if side == "candidate" {
							runtime[child] = finished
						}
						dir := sourceArtifactWriteFiles(t, runtime)
						for _, mode := range []string{"-ea", "-da"} {
							log, err := enumFreshOracleRun(t, java, mode, dir, "HelperDriver")
							if err != nil || string(log) != world.rows+":static:helper:constructor:algorithm\n" {
								t.Fatal("unchanged binary caller in evolving enum world", side, mode, err, string(log))
							}
						}
					}
				})
			}
		})
	}
}

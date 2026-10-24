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

// A named lexical member selects the native owning-family renderer. Its root
// must publish the same narrowly scoped temporary-helper receipt as a flat
// declaration. No original family class may appear on the rebuild classpath.
// Exceptions attributes describe Java source declarations, not verifier edges.
// An authored original can therefore propagate the same checked instance with
// an empty declaration. Source recovery needs a temporary generic rethrow, but
// a completed artifact must preserve the original method universe as well as
// the original exception identity, finally order and normal result.
func TestAdversarialNativeFamilyCheckedEscapeKeepsMethodUniverseAndBehavior(t *testing.T) {
	javac, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"EscapeOwner", "RenamedEscapeOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, target := range []int{8, 11} {
				t.Run(fmt.Sprintf("%s/%s/target%d", owner, debug, target), func(t *testing.T) {
					fixture := `class EscapeFailure extends Exception {EscapeFailure(){super("same");}}
class EscapeEffects {static String trace="";static final EscapeFailure checked=new EscapeFailure();static final RuntimeException unchecked=new IllegalStateException("same");static int calculate(int mode)throws EscapeFailure {trace+="C";if(mode==0)throw checked;if(mode==1)throw unchecked;return mode*31+7;}}
public class ` + owner + ` {public static class Child {public static int delta(){return 17;}}public static int exercise(int mode)throws EscapeFailure {try{return EscapeEffects.calculate(mode);}finally{EscapeEffects.trace+="F";}}}
class EscapeDriver {public static void main(String[]args){if(` + owner + `.Child.delta()!=17||` + owner + `.Child.class.getDeclaringClass()!=` + owner + `.class||` + owner + `.class.getDeclaredMethods().length!=1)throw new AssertionError("original method universe");try{if(` + owner + `.class.getDeclaredMethod("exercise",int.class).getExceptionTypes().length!=0)throw new AssertionError("original empty exceptions");}catch(NoSuchMethodException e){throw new AssertionError(e);}int rows=0;for(int mode:new int[]{0,1,2,-1,Integer.MIN_VALUE,Integer.MAX_VALUE}){EscapeEffects.trace="";try{int value=` + owner + `.exercise(mode);int expected=java.math.BigInteger.valueOf(mode).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).intValue();if(mode<2&&mode>=0||value!=expected||!EscapeEffects.trace.equals("CF"))throw new AssertionError("normal value/order");}catch(Throwable e){if(e!=(mode==0?EscapeEffects.checked:mode==1?EscapeEffects.unchecked:null)||!EscapeEffects.trace.equals("CF"))throw new AssertionError("exception identity/order",e);}rows++;}System.out.println(rows+":escape:identity:finally:method-universe");}}`
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
					object := mustParseLowering(t, files[owner+".class"])
					removed := 0
					for _, m := range object.Methods {
						if n, _ := sourceBridgeUTF8(object, m.NameIndex); n == "exercise" {
							attrs := []AttributeInfo{}
							for _, a := range m.Attributes {
								if _, ok := a.(*ExceptionsAttribute); ok {
									removed++
								} else {
									attrs = append(attrs, a)
								}
							}
							m.Attributes = attrs
						}
					}
					if removed != 1 {
						t.Fatal("authored original Exceptions entry")
					}
					files[owner+".class"] = object.Bytes()
					run := func(dir string) {
						t.Helper()
						for _, assertions := range []string{"-ea", "-da"} {
							log, err := exec.Command(java, "-Xverify:all", assertions, "-cp", dir, "EscapeDriver").CombinedOutput()
							if err != nil || string(log) != "6:escape:identity:finally:method-universe\n" {
								t.Fatal("original-first independent escape oracle", err, string(log))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
						t.Fatal(err)
					}
					artifact, err := NewJarReconstructionArtifactFromLocal(jar, target, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer artifact.Close()
					source, err := artifact.source.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("complete source", err, string(source))
					}
					support := map[string][]byte{}
					for n, b := range files {
						if n != owner+".class" && !strings.HasPrefix(n, owner+"$") {
							support[n] = b
						}
					}
					out := sourceArtifactWriteFiles(t, support)
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", fmt.Sprint(target), "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("fresh compilation without original owner", err, string(log), string(source))
					}
					unreturned, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					if result, err := artifact.FinishClass(owner+".class", unreturned); err == nil || result != nil {
						t.Fatal("unreturned native owning source granted executable completion")
					}
					published, err := artifact.ReadFile(owner + ".class")
					if err != nil || !bytes.Equal(source, published) {
						t.Fatal("same complete native source must activate", err)
					}
					for _, binary := range []string{owner, owner + "$Child"} {
						fresh, err := os.ReadFile(filepath.Join(out, binary+".class"))
						if err != nil {
							t.Fatal(err)
						}
						before := bytes.Clone(fresh)
						result, err := artifact.FinishClass(binary+".class", fresh)
						if err != nil || result == nil {
							t.Fatal("temporary exception helper must be discharged", err)
						}
						if !bytes.Equal(fresh, before) {
							t.Fatal("fresh input modified")
						}
						if err := os.WriteFile(filepath.Join(out, binary+".class"), result, 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(out)
				})
			}
		}
	}
}

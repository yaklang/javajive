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

func TestAdversarialOrdinaryStaticClassRetainsConstructorAbsence(t *testing.T) {
	javac, java := t04Tools(t)
	for _, modifiers := range []string{"", "public", "final", "public final"} {
		for _, owner := range []string{"StaticPacket", "AlternativeFacade"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", modifiers, owner, debug), func(t *testing.T) {
					fixture := strings.ReplaceAll(modifiers+` class StaticPacket {static int count=0;static final Object TOKEN=new Object();static int[] ARRAY={1,2,3};static {count++;}public static long apply(long x){count++;return x*31+ARRAY[(int)(x&1)];}}
class StaticDriver{public static void main(String[]args)throws Exception{Class<?> c=Class.forName("StaticPacket",false,StaticDriver.class.getClassLoader());if(c.getDeclaredConstructors().length!=0)throw new AssertionError("invented constructor");Object token=StaticPacket.TOKEN;long[] inputs={Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE};for(long x:inputs)if(StaticPacket.apply(x)!=x*31+StaticPacket.ARRAY[(int)(x&1)])throw new AssertionError("static algorithm");if(StaticPacket.count!=6||token!=StaticPacket.TOKEN)throw new AssertionError("initialization order or identity");System.out.println("static:constructor-absence:overflow:identity:once");}}`, "StaticPacket", owner)
					files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
					// Java source cannot express constructor absence. Remove only the
					// original compiler's unused default constructor to author that
					// legal JVM declaration; all original executable static code stays.
					o := mustParseLowering(t, files[owner+".class"])
					var kept []*MemberInfo
					removed := 0
					for _, m := range o.Methods {
						if n, _ := sourceBridgeUTF8(o, m.NameIndex); n == "<init>" {
							removed++
						} else {
							kept = append(kept, m)
						}
					}
					if removed != 1 {
						t.Fatal("fixture default constructor count", removed)
					}
					o.Methods = kept
					files[owner+".class"] = o.Bytes()
					want := "static:constructor-absence:overflow:identity:once\n"
					if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, files), "StaticDriver"); got != want {
						t.Fatal("original JVM oracle", got)
					}
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil {
						t.Fatal(err)
					}
					out := sourceArtifactWriteFiles(t, map[string][]byte{"StaticDriver.class": files["StaticDriver.class"]})
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatalf("fresh: %v\n%s\n%s", err, log, source)
					}
					fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					saved := bytes.Clone(fresh)
					done, err := a.FinishClass(owner+".class", fresh)
					if err != nil {
						t.Fatal("constructor absence publication", err)
					}
					if !bytes.Equal(fresh, saved) {
						t.Fatal("fresh compiler input mutated")
					}
					f, finished := mustParseLowering(t, fresh), mustParseLowering(t, done)
					if finished.AccessFlags != o.AccessFlags || len(finished.Methods) != len(o.Methods) {
						t.Fatal("class or method universe changed")
					}
					for _, m := range finished.Methods {
						n, _ := sourceBridgeUTF8(finished, m.NameIndex)
						d, _ := sourceBridgeUTF8(finished, m.DescriptorIndex)
						if n == "<init>" {
							t.Fatal("constructor retained")
						}
						matched := false
						for _, fm := range f.Methods {
							fn, _ := sourceBridgeUTF8(f, fm.NameIndex)
							fd, _ := sourceBridgeUTF8(f, fm.DescriptorIndex)
							if n == fn && d == fd {
								matched = true
								x, y := constructorLoweringCode(m), constructorLoweringCode(fm)
								if x != nil && (y == nil || !bytes.Equal(x.Code, y.Code)) {
									t.Fatal("fresh static executable changed")
								}
							}
						}
						if !matched {
							t.Fatal("invented method")
						}
					}
					if err := os.WriteFile(filepath.Join(out, owner+".class"), done, 0600); err != nil {
						t.Fatal(err)
					}
					if got := t04RunJava(t, java, out, "StaticDriver"); got != want {
						t.Fatal("unchanged original caller", got)
					}
					if again, err := a.FinishClass(owner+".class", done); err != nil || !bytes.Equal(again, done) {
						t.Fatal("not idempotent", err)
					}
				})
			}
		}
	}
}

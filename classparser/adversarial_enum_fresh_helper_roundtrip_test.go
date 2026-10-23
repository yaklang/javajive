package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const enumFreshHelperFixture = `public enum PacketValues {FIRST,MIDDLE,LAST;
 public static long fold(long input){long result=input;for(PacketValues value:values())result=result*31+value.ordinal();return result;}
}
class PacketValuesDriver {public static void main(String[]args)throws Exception {
 int rows=0;for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=input;
  for(int ordinal=0;ordinal<3;ordinal++)expected=java.math.BigInteger.valueOf(expected).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(ordinal)).longValue();
  if(PacketValues.fold(input)!=expected)throw new AssertionError("enum order/overflow");
  PacketValues[] first=PacketValues.values(),second=PacketValues.class.getEnumConstants();if(first==second||first.length!=3||second.length!=3)throw new AssertionError("array identity");
  for(int i=0;i<3;i++)if(first[i]!=second[i]||first[i].ordinal()!=i||PacketValues.valueOf(first[i].name())!=first[i])throw new AssertionError("constant identity");
  first[0]=null;if(PacketValues.values()[0]!=PacketValues.FIRST)throw new AssertionError("fresh clone");rows++;
 }
 if(PacketValues.class.getDeclaredMethods().length!=3)throw new AssertionError("extra fresh compiler helper is observable");
 System.out.println(rows+":enum:values:clone:binding:overflow:method-universe");
}}
`

func TestAdversarialEnumFreshHelperPreservesOriginalMethodUniverse(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"PacketValues", "RenamedPacketValues"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(owner+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(enumFreshHelperFixture, "PacketValues", owner)
				driver := owner + "Driver"
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
				run := func(dir string, want bool) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := exec.Command(java, "-Xverify:all", mode, "-cp", dir, driver).CombinedOutput()
						if want && (err != nil || string(out) != "5:enum:values:clone:binding:overflow:method-universe\n") {
							t.Fatalf("original-first enum observer: %v\n%s", err, out)
						}
						if !want && (err == nil || !strings.Contains(string(out), "extra fresh compiler helper is observable")) {
							t.Fatalf("fresh helper negative control: %v\n%s", err, out)
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files), true)
				jar := filepath.Join(t.TempDir(), "enum.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				src, err := a.ReadFile(owner + ".class")
				if err != nil || legacySourceHasFailureMarker(string(src)) {
					t.Fatalf("enum source: %v\n%s", err, src)
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{driver + ".class": files[driver+".class"]})
				path := filepath.Join(out, owner+".java")
				if err := os.WriteFile(path, src, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
					t.Fatal("source only enum compile", err, string(log))
				}
				run(out, false)
				fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
				if err != nil {
					t.Fatal(err)
				}
				if p, err := a.source.RestoreClassMetadata(owner+".class", fresh); err == nil || p != nil {
					t.Fatal("ordinary metadata gate incorrectly permits extra helper")
				}
				finished, err := a.FinishClass(owner+".class", fresh)
				if err != nil {
					t.Fatal("explicit reconstruction must discharge the fresh array helper before metadata", err)
				}
				if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out, true)
			})
		}
	}
}

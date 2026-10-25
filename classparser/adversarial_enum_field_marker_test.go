package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const enumFieldMarkerFixture = `public enum StorageChoice {
 FIRST(Long.MIN_VALUE),LAST(Long.MAX_VALUE);
 private final long bias;private volatile Object cache;
 private static final Object sentinel=new Object();
 StorageChoice(long bias){this.bias=bias;this.cache=new long[]{bias};}
 public long fold(long n){return n*bias+ordinal();}
 public Object token(){return cache;}
 public static Object marker(){return sentinel;}
}
class StorageChoiceDriver {public static void main(String[]args)throws Exception{
 for(String name:new String[]{"bias","cache","sentinel"})if(!StorageChoice.class.getDeclaredField(name).isSynthetic())throw new AssertionError("field marker");
 if(StorageChoice.class.getDeclaredMethods().length!=5)throw new AssertionError("method universe");
 StorageChoice[] values=StorageChoice.values();if(values.length!=2||values[0]!=StorageChoice.FIRST||values[1]!=StorageChoice.LAST)throw new AssertionError("constant binding");
 int rows=0;for(int i=0;i<2;i++){StorageChoice v=values[i];long bias=i==0?Long.MIN_VALUE:Long.MAX_VALUE;
  if(v.token()!=v.token()||((long[])v.token())[0]!=bias)throw new AssertionError("storage identity");
  for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(bias)).add(java.math.BigInteger.valueOf(i)).longValue();if(v.fold(n)!=expected)throw new AssertionError("arithmetic storage");rows++;}
 }
 if(StorageChoice.marker()!=StorageChoice.marker())throw new AssertionError("static identity");
 values[0]=null;if(StorageChoice.values()[0]!=StorageChoice.FIRST)throw new AssertionError("clone");
 System.out.println(rows+":enum:fields:identity:overflow");
}}
`

func TestAdversarialEnumSyntheticStorageKeepsTypedArgumentsAndFieldContract(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, owner := range []string{"StorageChoice", "RenamedStorageChoice"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, entry := range []bool{false, true} {
				label := "direct"
				if entry {
					label = "entry jumps"
				}
				t.Run(owner+"/"+debug+"/"+label, func(t *testing.T) {
					fixture := strings.ReplaceAll(enumFieldMarkerFixture, "StorageChoice", owner)
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, false)
					object := mustParseLowering(t, files[owner+".class"])
					for _, field := range object.Fields {
						name, _ := sourceBridgeUTF8(object, field.NameIndex)
						if name == "bias" || name == "cache" || name == "sentinel" {
							field.AccessFlags |= syntheticFieldFlag
						}
					}
					files[owner+".class"] = object.Bytes()
					if entry {
						files[owner+".class"] = enumEntryPathOriginal(t, files[owner+".class"], false, 2)
					}
					run := func(dir string) {
						t.Helper()
						for _, mode := range []string{"-ea", "-da"} {
							log, err := enumFreshOracleRun(t, java, mode, dir, owner+"Driver")
							if err != nil || string(log) != "10:enum:fields:identity:overflow\n" {
								t.Fatal("unchanged original-first field observer", err, string(log))
							}
						}
					}
					run(sourceArtifactWriteFiles(t, files))
					jar := filepath.Join(t.TempDir(), "storage.jar")
					if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
						t.Fatal(err)
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					source, err := a.ReadFile(owner + ".class")
					if err != nil || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("published enum storage source", err, string(source))
					}
					out := sourceArtifactWriteFiles(t, map[string][]byte{owner + "Driver.class": files[owner+"Driver.class"]})
					path := filepath.Join(out, owner+".java")
					if err := os.WriteFile(path, source, 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
						t.Fatal("source-only enum storage compile", err, string(log))
					}
					fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
					if err != nil {
						t.Fatal(err)
					}
					finished, err := a.FinishClass(owner+".class", fresh)
					if err != nil {
						t.Fatal("complete array and field metadata", err)
					}
					if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
						t.Fatal("idempotent field and array completion", err)
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

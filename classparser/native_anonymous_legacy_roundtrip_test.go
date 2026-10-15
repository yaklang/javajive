package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func nativeCompileJava56(t *testing.T, source, debug, level string) map[string][]byte {
	t.Helper()
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("actual javac8 required for authored Java5/6 input")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "LegacyCaptureOwner.java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "-source", level, "-target", level, "-g:"+debug, "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("authored Java%s: %v\n%s", level, err, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	want, known := map[string]uint16{"5": 49, "6": 50}[level]
	if !known {
		t.Fatal("unsupported authored source level")
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".class") {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			obj, err := Parse(raw)
			if err != nil || obj.MajorVersion != want || obj.MinorVersion != 0 {
				t.Fatalf("actual original class %s: %v", entry.Name(), err)
			}
			files[entry.Name()] = raw
		}
	}
	return files
}

const nativeLegacyStaticCaptureFixture = `class LegacyCaptureOwner {
 static LegacyCaptureBase make(final String text){return new LegacyCaptureBase(){public long read(){
  if(String[].class.getComponentType()!=String.class)throw new AssertionError("class literal binding");
  return text==null?3:text.length();}};}
}
class LegacyCaptureBase {final long during;LegacyCaptureBase(){during=read();}public long read(){return -1;}}
class LegacyCaptureDriver {public static void main(String[]args){
 for(String text:new String[]{null,"","x","longer"}){LegacyCaptureBase value=LegacyCaptureOwner.make(text);long expected=text==null?3:text.length();
  if(value.during!=expected||value.read()!=expected)throw new AssertionError("capture before virtual super callback");
 }System.out.println("legacy:capture:callback:identity");}}
`

const nativeLegacyInstanceCaptureFixture = `class LegacyCaptureOwner {
 int seed=11;
 LegacyCaptureBase make(final long word,final double decimal,final Object[] values){return new LegacyCaptureBase(){public long read(){
  return word^(long)decimal^seed^((Integer)values[0]).intValue();}};}
}
class LegacyCaptureBase {final long during;LegacyCaptureBase(){during=read();}public long read(){return -1;}}
class LegacyCaptureDriver {public static void main(String[]args){
 for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(double decimal:new double[]{Double.NaN,Double.NEGATIVE_INFINITY,3.5}){
  LegacyCaptureOwner owner=new LegacyCaptureOwner();Object[] values=new Object[]{Integer.valueOf(7)};
  LegacyCaptureBase value=owner.make(word,decimal,values);long during=word^(long)decimal^11^7;
  owner.seed=19;values[0]=Integer.valueOf(-3);long delayed=word^(long)decimal^19^-3;
  if(value.during!=during||value.read()!=delayed)throw new AssertionError("wide words/enclosing state/array alias");
 }System.out.println("legacy:capture:callback:identity");}}
`

func TestNativeAnonymousJava56CapturesKeepObservableConstruction(t *testing.T) {
	for _, level := range []string{"5", "6"} {
		t.Run(level, func(t *testing.T) {
			for _, variant := range []string{"static class literal", "instance wide aliases", "renamed instance"} {
				t.Run(variant, func(t *testing.T) {
					source := nativeLegacyStaticCaptureFixture
					if variant != "static class literal" {
						source = nativeLegacyInstanceCaptureFixture
					}
					if variant == "renamed instance" {
						source = strings.NewReplacer("seed", "state", "word", "bits", "decimal", "fraction", "values", "objects").Replace(source)
					}
					testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativeCompileJava56(t, source, debug, level) }, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"LegacyCaptureOwner"}, "LegacyCaptureDriver", "legacy:capture:callback:identity\n", nil, nativeLexicalExactSignatures)
				})
			}
		})
	}
}

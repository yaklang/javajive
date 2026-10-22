package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The parent is a binary dependency outside the source artifact. Its inherited
// field can therefore be visible in an original Fieldref without appearing in
// the artifact's declaration inventory. Source alpha conversion must preserve
// that binding even when the original symbolic owner is the child itself.
const assertionStorageInheritedNameFixture = `class AliasScope{}
class StorageParent {static boolean $jdec$storage$2;}
public class InheritedStorage extends StorageParent {
 static final boolean $assertionsDisabled=!AliasScope.class.desiredAssertionStatus();
 public static boolean inheritedValue(){return InheritedStorage.$jdec$storage$2;}
 public static int algorithm(int n){if(!$assertionsDisabled&&n<0)throw new AssertionError(n);return n*3+5;}
}
class InheritedDriver {
 public static void main(String[]args){
  boolean enabled=InheritedDriver.class.desiredAssertionStatus();
  ClassLoader.getSystemClassLoader().setClassAssertionStatus("AliasScope",!enabled);
  int rows=0;
  for(boolean field:new boolean[]{false,true}){
   StorageParent.$jdec$storage$2=field;
   if(InheritedStorage.inheritedValue()!=field)throw new AssertionError("inherited field binding changed");
   for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
    Throwable error=null;int value=0;
    try{value=InheritedStorage.algorithm(n);}catch(Throwable e){error=e;}
    boolean fail=!enabled&&n<0;
    if(fail!=(error instanceof AssertionError)||error!=null&&!(error instanceof AssertionError))throw new AssertionError("assertion storage binding changed");
    if(!fail&&value!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(3)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("modular arithmetic changed");
    rows++;
   }
  }
  System.out.println(rows+":external-inherited-field:assertion-binding:arithmetic");
 }
}`

func TestAdversarialAssertionStorageKeepsExternalInheritedFieldNames(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, javac, ModernJavac, assertionStorageInheritedNameFixture, "InheritedStorage", debug, true)
			object, err := Parse(files["InheritedStorage.class"])
			if err != nil {
				t.Fatal(err)
			}
			marked, read := false, false
			for _, f := range object.Fields {
				name, _ := sourceBridgeUTF8(object, f.NameIndex)
				if name == nativeAssertionField {
					f.AccessFlags |= syntheticFieldFlag
					marked = true
				}
			}
			for _, constant := range object.ConstantPool {
				ref, ok := constant.(*ConstantFieldrefInfo)
				if !ok {
					continue
				}
				nt, _ := object.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				name, _ := sourceBridgeUTF8(object, nt.NameIndex)
				owner, _ := sourceBridgeClassName(object, ref.ClassIndex)
				read = read || name == "$jdec$storage$2" && owner == "InheritedStorage"
			}
			if !marked || !read {
				t.Fatal("original own-symbolic inherited-field witness absent")
			}
			files["InheritedStorage.class"] = object.Bytes()
			original := sourceArtifactWriteFiles(t, files)
			run := func(dir string) {
				t.Helper()
				for _, mode := range []string{"-ea", "-da"} {
					out, err := exec.Command(java, "-Xverify:all", mode, "-cp", dir, "InheritedDriver").CombinedOutput()
					if err != nil || string(out) != "10:external-inherited-field:assertion-binding:arithmetic\n" {
						t.Fatalf("independent %s oracle: %v\n%s", mode, err, out)
					}
				}
			}
			run(original)
			entries := map[string][]byte{}
			dependencies := map[string][]byte{}
			for name, raw := range files {
				if name != "StorageParent.class" {
					entries[name] = raw
				}
				if name != "InheritedStorage.class" {
					dependencies[name] = raw
				}
			}
			artifact := assertionStorageArtifact(t, entries)
			artifact.jar.declarationResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
			source, err := artifact.ReadFile("InheritedStorage.class")
			if err != nil || strings.Contains(string(source), DecompileStubMarker) {
				t.Fatalf("source: %v\n%s", err, source)
			}
			out := sourceArtifactWriteFiles(t, dependencies)
			path := filepath.Join(out, "InheritedStorage.java")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:"+debug, "-parameters", "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
				t.Fatalf("original subject absent from compiler: %v\n%s", err, log)
			}
			raw, err := os.ReadFile(filepath.Join(out, "InheritedStorage.class"))
			if err != nil {
				t.Fatal(err)
			}
			restored, err := artifact.RestoreClassMetadata("InheritedStorage.class", raw)
			if err != nil {
				t.Fatal(err)
			}
			if nativeBinaryShape(t, files["InheritedStorage.class"]) != nativeBinaryShape(t, restored) {
				t.Fatal("original complete ABI changed")
			}
			if err := os.WriteFile(filepath.Join(out, "InheritedStorage.class"), restored, 0600); err != nil {
				t.Fatal(err)
			}
			run(out)
		})
	}
}

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

// Pre-nestmate JVMs grant private field access only to the declaring class.
// The unchanged reader below therefore uses the original accessor methods.
// Flattening the source owner must not broaden fields merely because its
// InnerClasses table mentions that reader. A subclass deliberately shadows
// both fields, so source spelling alone cannot serve as the binding oracle.
const legacyPrivateFieldFixture = `public class LegacyFieldOwner {
 private long state;private int count;private static Object token;
 public LegacyFieldOwner(long seed){state=seed;}
 public static void token(Object value){token=value;}
 public long step(long delta,Object expected){return new Reader().step(this,delta,expected);}
 static class Reader {long step(LegacyFieldOwner owner,long delta,Object expected){
  if(token!=expected)throw new AssertionError("static identity");
  owner.state=owner.state*31L+delta;owner.count++;return owner.state^owner.count;
 }}
}
class LegacyFieldDerived extends LegacyFieldOwner {
 public long state=777;public int count=888;LegacyFieldDerived(long seed){super(seed);}
}
class Legacy$Unrelated {private int hidden;int value(){return hidden;}}
class LegacyFieldDriver {public static void main(String[]args)throws Exception {
 for(String name:new String[]{"state","count","token"})if(!java.lang.reflect.Modifier.isPrivate(LegacyFieldOwner.class.getDeclaredField(name).getModifiers()))throw new AssertionError("private declaration contract");
 int rows=0;for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,new Object()}){
  LegacyFieldDerived owner=new LegacyFieldDerived(seed);LegacyFieldOwner.token(token);long expected=seed;int count=0;
  for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   expected=java.math.BigInteger.valueOf(expected).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(delta)).longValue();count++;
   if(owner.step(delta,token)!=(expected^count)||owner.state!=777||owner.count!=888)throw new AssertionError("private accessor binding/overflow");rows++;
  }
 }
 System.out.println(rows+":legacy:private:original-reader:shadowing:wide:identity");
}}
`

func TestAdversarialLegacyPrivateFieldsIgnoreLexicalNamesWithoutVmNest(t *testing.T) {
	compiler, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, legacyPrivateFieldFixture, "LegacyFieldOwner", "none", false)
	for _, name := range []string{"LegacyFieldOwner", "Legacy$Unrelated"} {
		for _, version := range []uint16{52, 54, 55, 65} {
			t.Run(fmt.Sprintf("%s/major%d", name, version), func(t *testing.T) {
				object := mustParseLowering(t, bytes.Clone(files[name+".class"]))
				object.MajorVersion = version
				if hasOriginalNestAttribute(object, "NestHost") || hasOriginalNestAttribute(object, "NestMembers") {
					t.Fatal("fixture unexpectedly has VM nest permission")
				}
				source, err := Decompile(object.Bytes())
				declaration := "private long state"
				if name == "Legacy$Unrelated" {
					declaration = "private int hidden"
				}
				if err != nil || !strings.Contains(source, declaration) {
					t.Fatalf("lexical name/InnerClasses cannot widen a private field at version %d: %v\n%s", version, err, source)
				}
			})
		}
	}
}

func TestAdversarialLegacyPrivateFieldsRetainOriginalAccessorBindings(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("original native javac8 is required")
			}
		}
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				files := compileLambdaParameterMetadataFixture(t, compiler, profile, legacyPrivateFieldFixture, "LegacyFieldOwner", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := exec.Command(java, "-Xverify:all", mode, "-cp", dir, "LegacyFieldDriver").CombinedOutput()
						if err != nil || string(out) != "50:legacy:private:original-reader:shadowing:wide:identity\n" {
							t.Fatalf("unchanged original reader and caller %s: %v\n%s", mode, err, out)
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				original := files["LegacyFieldOwner.class"]
				object := mustParseLowering(t, bytes.Clone(original))
				if object.MajorVersion != 52 || !bytes.Contains(original, []byte("access$")) {
					t.Fatal("original pre-nestmate accessor control")
				}
				// Only the owner is reconstructed. Its reader and driver remain
				// byte-identical original dependencies; no owner binary is on the
				// compiler or runtime classpath while reconstructing that owner.
				jar := filepath.Join(t.TempDir(), "owner.jar")
				if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"LegacyFieldOwner.class": original}), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				source, err := a.ReadFile("LegacyFieldOwner.class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatalf("source owner: %v\n%s", err, source)
				}
				for _, declaration := range []string{"private long state", "private int count", "private static Object token"} {
					if !strings.Contains(string(source), declaration) {
						t.Fatalf("legacy private field was widened: %s\n%s", declaration, source)
					}
				}
				deps := map[string][]byte{}
				for name, raw := range files {
					if name != "LegacyFieldOwner.class" {
						deps[name] = raw
					}
				}
				out := sourceArtifactWriteFiles(t, deps)
				path := filepath.Join(out, "LegacyFieldOwner.java")
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
				fresh, err := os.ReadFile(filepath.Join(out, "LegacyFieldOwner.class"))
				if err != nil {
					t.Fatal(err)
				}
				finished, err := a.FinishClass("LegacyFieldOwner.class", fresh)
				if err != nil {
					t.Fatal("private field/method declarations must pass existing contracts", err)
				}
				if err := os.WriteFile(filepath.Join(out, "LegacyFieldOwner.class"), finished, 0600); err != nil {
					t.Fatal(err)
				}
				run(out)
			})
		}
	}
}

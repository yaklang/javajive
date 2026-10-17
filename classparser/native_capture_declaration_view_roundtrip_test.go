package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const captureDeclaredViewFixture3855 = `class CaptureDeclaredEffects{static String trace="";static CaptureDeclaredNarrow factory(Object token,boolean fail){trace+="F";if(fail)throw new IllegalArgumentException("factory");return token==null?null:new CaptureDeclaredNarrow(token);}static String choose(CaptureDeclaredWide x){trace+="W";return "wide";}static String choose(CaptureDeclaredNarrow x){trace+="N";return "narrow";}}
class CaptureDeclaredWide{final Object token;CaptureDeclaredWide(Object token){this.token=token;}}
class CaptureDeclaredNarrow extends CaptureDeclaredWide{CaptureDeclaredNarrow(Object token){super(token);}}
abstract class CaptureDeclaredReader{final Object early;CaptureDeclaredReader(){CaptureDeclaredEffects.trace+="P";early=read();}abstract Object read();abstract String choose();}
class CaptureDeclaredOwner{CaptureDeclaredReader make(Object token,boolean fail){final CaptureDeclaredWide addresses=CaptureDeclaredEffects.factory(token,fail);if(!CaptureDeclaredEffects.choose(addresses).equals("wide"))throw new AssertionError("caller overload");return new CaptureDeclaredReader(){Object read(){return addresses==null?null:addresses.token;}String choose(){return CaptureDeclaredEffects.choose(addresses);}};}}
class CaptureDeclaredDriver{public static void main(String[]args)throws Exception{int rows=0;CaptureDeclaredOwner owner=new CaptureDeclaredOwner();for(Object token:new Object[]{null,new Object(),new String("identity")})for(boolean fail:new boolean[]{false,true}){CaptureDeclaredEffects.trace="";try{CaptureDeclaredReader reader=owner.make(token,fail);if(fail||reader.read()!=token||reader.early!=token||!reader.choose().equals("wide")||!CaptureDeclaredEffects.trace.equals("FWPW"))throw new AssertionError("effects/identity/overload");java.lang.reflect.Field capture=reader.getClass().getDeclaredField("val$addresses");capture.setAccessible(true);if(capture.getType()!=CaptureDeclaredWide.class||(capture.get(reader)==null?null:((CaptureDeclaredWide)capture.get(reader)).token)!=token||reader.getClass().getEnclosingClass()!=CaptureDeclaredOwner.class)throw new AssertionError("capture metadata");}catch(IllegalArgumentException expected){if(!fail||!CaptureDeclaredEffects.trace.equals("F"))throw new AssertionError("exception order");}rows++;}System.out.println(rows+":capture:declaration:overload:identity:early:failure");}}`

func nativeCapturedDeclarationFixture(shape, owner string) string {
	fixture := strings.ReplaceAll(captureDeclaredViewFixture3855, "CaptureDeclaredOwner", owner)
	if shape == "transitive" {
		fixture = strings.Replace(fixture, "class CaptureDeclaredNarrow extends CaptureDeclaredWide", "class CaptureDeclaredMiddle extends CaptureDeclaredWide{CaptureDeclaredMiddle(Object token){super(token);}}class CaptureDeclaredNarrow extends CaptureDeclaredMiddle", 1)
	}
	if shape == "explicit narrow consumer" {
		fixture = strings.Replace(fixture, "CaptureDeclaredNarrow(Object token){super(token);}", "CaptureDeclaredNarrow(Object token){super(token);}Object exact(){return token;}", 1)
		fixture = strings.Replace(fixture, "if(!CaptureDeclaredEffects.choose(addresses)", "if(addresses!=null&&((CaptureDeclaredNarrow)addresses).exact()!=token)throw new AssertionError(\"explicit narrow receiver\");if(!CaptureDeclaredEffects.choose(addresses)", 1)
	}
	return fixture
}
func TestNativeCapturedLocalDeclarationKeepsOriginalWideField(t *testing.T) {
	for _, shape := range []string{"direct", "transitive", "explicit narrow consumer"} {
		t.Run(shape, func(t *testing.T) {
			for _, owner := range []string{"CaptureDeclaredOwner", "DifferentDeclaredOwner"} {
				t.Run(owner, func(t *testing.T) {
					fixture := nativeCapturedDeclarationFixture(shape, owner)
					testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "CaptureDeclaredDriver", "6:capture:declaration:overload:identity:early:failure\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}

func TestNativeCapturedDeclarationViewNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", err, string(version))
	}
	for _, shape := range []string{"direct", "transitive", "explicit narrow consumer"} {
		t.Run(shape, func(t *testing.T) {
			fixture := nativeCapturedDeclarationFixture(shape, "CaptureDeclaredOwner")
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				path := filepath.Join(dir, "CaptureDeclaredOwner.java")
				if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatal("original compile", err, string(out))
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[entry.Name()] = raw
					}
				}
				return files
			}, NativeJavac8, javac, []string{"CaptureDeclaredOwner"}, "CaptureDeclaredDriver", "6:capture:declaration:overload:identity:early:failure\n", nil, nativeLexicalExactSignatures)
		})
	}
}

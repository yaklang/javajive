package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialStaticSharedBooleanWordRoundTrip(t *testing.T) {
	const fixture = `class StaticWordOwner {static int flag;static String trace="";static int produce(int word){trace+="P";return word;}static int shared(int word){return(flag=produce(word));}static int stored(){return flag;}}
class StaticWordDriver {public static void main(String[]args)throws Exception {int rows=0;for(int word:new int[]{0,1,2,3,-1,-2,Integer.MIN_VALUE,Integer.MAX_VALUE}){StaticWordOwner.trace="";int copied=StaticWordOwner.shared(word),stored=StaticWordOwner.stored();if(copied!=word||stored!=(word&1)||!StaticWordOwner.trace.equals("P")||StaticWordOwner.class.getDeclaredField("flag").getBoolean(null)!=(stored==1))throw new AssertionError("static original word/read/lowbit/producer once:"+word+":"+copied+":"+stored);rows++;}System.out.println(rows+":static:word:read:once");}}`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, e := Parse(files["StaticWordOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			descriptor := uint16(obj.ConstantPoolManager.AddUtf8Info("Z"))
			fields, refs := 0, 0
			for _, f := range obj.Fields {
				name, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if name == "flag" {
					f.DescriptorIndex = descriptor
					fields++
				}
			}
			for _, c := range obj.ConstantPool {
				if nt, ok := c.(*ConstantNameAndTypeInfo); ok {
					name, _ := sourceBridgeUTF8(obj, nt.NameIndex)
					desc, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
					if name == "flag" && desc == "I" {
						nt.DescriptorIndex = descriptor
						refs++
					}
				}
			}
			if fields != 1 || refs != 1 {
				t.Fatal("unique original declaration/ref", fields, refs)
			}
			files["StaticWordOwner.class"] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, "StaticWordDriver")
			if oracle != "8:static:word:read:once\n" {
				t.Fatalf("valid original:%q", oracle)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					source, e := z.ReadFile("StaticWordOwner.class")
					if e != nil || strings.Contains(string(source), DecompileStubMarker) {
						t.Fatalf("source:%v\n%s", e, source)
					}
					out := t.TempDir()
					if e := os.WriteFile(filepath.Join(out, "StaticWordDriver.class"), files["StaticWordDriver.class"], 0600); e != nil {
						t.Fatal(e)
					}
					path := filepath.Join(out, "StaticWordOwner.java")
					if e := os.WriteFile(path, source, 0600); e != nil {
						t.Fatal(e)
					}
					if data, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput(); e != nil {
						t.Fatalf("rebuild:%v\n%s\n%s", e, data, source)
					}
					if got := t04RunJava(t, java, out, "StaticWordDriver"); got != oracle {
						t.Fatalf("rebuilt:%q", got)
					}
					raw, e := os.ReadFile(filepath.Join(out, "StaticWordOwner.class"))
					if e != nil {
						t.Fatal(e)
					}
					if want, got := nativeBinaryShape(t, files["StaticWordOwner.class"]), nativeBinaryShape(t, raw); got != want {
						t.Fatalf("original ABI changed\n%s\n%s", want, got)
					}
				})
			}
		})
	}
}

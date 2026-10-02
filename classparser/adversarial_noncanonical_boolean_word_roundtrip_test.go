package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// These mutations preserve instruction widths/branches/StackMapTable and the
// JVM computational int category. Changing I to Z in return descriptors is
// verifier-valid, while IRETURN must narrow only at that consumer. A shared
// local is also consumed by IFEQ and numeric concatenation before returning.
func TestAdversarialNoncanonicalBooleanWordsRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `public class NoncanonicalBooleanWordsReview {
 static boolean cached;static String trace="";static boolean produce(){trace+="P;";return true;}
 static int alias(int word){int saved=word;trace+=saved+";";if(saved!=0)trace+="T;";else trace+="F;";return saved;}
 static int literal(){return 64;}
 static int phi(boolean choose){return choose?(cached=produce()?true:false)?1:0:64;}
 public static void main(String[] args){for(int word:new int[]{2,3,-2,-1,0,1}){trace="";System.out.println(alias(word)+":"+trace);}trace="";System.out.println(literal());for(boolean choose:new boolean[]{false,true}){trace="";cached=false;System.out.println(phi(choose)+":"+cached+":"+trace);}}
}`
	for _, debug := range []string{"-g", "-g:none"} {
		original := t.TempDir()
		path := filepath.Join(original, "NoncanonicalBooleanWordsReview.java")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, path).CombinedOutput(); err != nil {
			t.Fatalf("original source: %v\n%s", err, out)
		}
		base, err := os.ReadFile(filepath.Join(original, "NoncanonicalBooleanWordsReview.class"))
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range []int{2, 3, -2, -1} {
			t.Run(debug+"/"+fmt.Sprint(word), func(t *testing.T) {
				obj, err := Parse(append([]byte(nil), base...))
				if err != nil {
					t.Fatal(err)
				}
				for _, constant := range obj.ConstantPool {
					if u, ok := constant.(*ConstantUtf8Info); ok {
						switch u.Value {
						case "(I)I":
							u.Value = "(I)Z"
						case "()I":
							u.Value = "()Z"
						case "(Z)I":
							u.Value = "(Z)Z"
						}
					}
				}
				changed := 0
				for _, method := range obj.Methods {
					n, _ := obj.getUtf8(method.NameIndex)
					if n != "literal" && n != "phi" {
						continue
					}
					for _, attr := range method.Attributes {
						if code, ok := attr.(*CodeAttribute); ok {
							d := core.NewDecompiler(code.Code, nil)
							if err := d.ParseOpcode(); err != nil {
								t.Fatal(err)
							}
							for _, op := range d.Opcodes() {
								if op.Instr.OpCode == core.OP_BIPUSH && len(op.Data) == 1 && op.Data[0] == 64 {
									code.Code[int(op.CurrentOffset)+1] = byte(int8(word))
									changed++
								}
							}
						}
					}
				}
				if changed != 2 {
					t.Fatalf("mutation shape changed: %d !=2", changed)
				}
				raw := obj.Bytes()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "NoncanonicalBooleanWordsReview.class"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				want := t04RunJava(t, java, dir, "NoncanonicalBooleanWordsReview")
				prefix := "0:2;T;\n1:3;T;\n0:-2;T;\n1:-1;T;\n0:0;F;\n1:1;T;\n"
				bit := word & 1
				if !strings.HasPrefix(want, prefix) || !strings.Contains(want, fmt.Sprintf("%d\n%d:false:\n1:true:P;", bit, bit)) {
					t.Fatalf("original JVM narrowing/branch oracle changed: %q", want)
				}
				for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
					var result DecompileResult
					var e error
					if mode == "legacy" {
						result.Source, e = Decompile(raw)
					} else {
						result, e = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8})
					}
					if e != nil {
						t.Fatal(e)
					}
					if len(result.StubMethods) > 0 {
						t.Fatalf("%s stubs: %v", mode, result.StubMethods)
					}
					rebuilt := t.TempDir()
					src := filepath.Join(rebuilt, "NoncanonicalBooleanWordsReview.java")
					if e = os.WriteFile(src, []byte(result.Source), 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-d", rebuilt, src).CombinedOutput(); e != nil {
						t.Fatalf("%s rebuild: %v\n%s\n%s", mode, e, out, result.Source)
					}
					if got := t04RunJava(t, java, rebuilt, "NoncanonicalBooleanWordsReview"); got != want {
						t.Fatalf("%s got%q want%q\n%s", mode, got, want, result.Source)
					}
				}
			})
		}
	}
}

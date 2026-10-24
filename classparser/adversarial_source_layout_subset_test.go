package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sourceLayoutSubsetFixture = `import java.util.function.IntUnaryOperator;
abstract class LayoutParent{final int seen;LayoutParent(){seen=read();}abstract int read();}
class LayoutGood{private final int value;LayoutGood(int value){this.value=value;}static class Token{}final class Leaf extends LayoutParent{final LayoutOpaque.Token peer;Leaf(LayoutOpaque.Token peer){this.peer=peer;}int read(){return LayoutGood.this.value;}int sum(){return read()+peer.value;}}Leaf make(LayoutOpaque.Token peer){return new Leaf(peer);}}
class LayoutOpaque{LayoutGood.Token peer;static class Token{final int value;Token(int value){this.value=value;}}static IntUnaryOperator unrelated(final String text){return value->value+text.length();}}
class LayoutDriver{public static void main(String[]args){int rows=0;for(int left:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int right:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){LayoutGood good=new LayoutGood(left);LayoutOpaque.Token peer=new LayoutOpaque.Token(right);LayoutGood.Leaf leaf=good.make(peer);int expected=java.math.BigInteger.valueOf(left).add(java.math.BigInteger.valueOf(right)).intValue();if(leaf.seen!=left||leaf.read()!=left||leaf.sum()!=expected||leaf.peer!=peer||leaf.getClass().getDeclaringClass()!=LayoutGood.class)throw new AssertionError("source layout, callback or peer binding");LayoutGood.Leaf nil=good.make(null);if(nil.seen!=left||nil.read()!=left||nil.peer!=null)throw new AssertionError("null peer construction");try{nil.sum();throw new AssertionError("missing peer null check");}catch(NullPointerException wanted){}if(LayoutOpaque.unrelated("abc").applyAsInt(left)!=left+3)throw new AssertionError("unchanged original peer body");rows++;}System.out.println(rows+":source:layout:subset");}}
`

func TestAdversarialSourceLayoutSubsetKeepsOriginalPeerBodyAndEarlyCapture(t *testing.T) {
	for _, renamed := range []bool{false, true} {
		source, good, opaque, driver := sourceLayoutSubsetFixture, "LayoutGood", "LayoutOpaque", "LayoutDriver"
		name := "original names"
		if renamed {
			name = "renamed scopes"
			good = "HealthyScope"
			opaque = "OpaqueScope"
			driver = "SeparateDriver"
			source = strings.NewReplacer("LayoutGood", good, "LayoutOpaque", opaque, "LayoutDriver", driver).Replace(source)
		}
		t.Run(name, func(t *testing.T) {
			testSourceLayoutSubset(t, source, good, opaque, driver, "25:source:layout:subset\n")
		})
	}
}
func testSourceLayoutSubset(t *testing.T, source, good, opaque, driver, expected string) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Skip("original native8 compiler")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, source, good, debug, true)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			want := t04RunJava(t, java, original, driver)
			if want != expected {
				t.Fatal("original mathematical/identity oracle", want)
			}
			for _, reverse := range []bool{false, true} {
				label := "family-first"
				if reverse {
					label = "flat-peer-first"
				}
				t.Run(label, func(t *testing.T) {
					z := nativeArchive(t, files)
					defer z.Close()
					roots := []string{good, opaque + "$Token"}
					if reverse {
						roots[0], roots[1] = roots[1], roots[0]
					}
					generated := map[string][]byte{}
					for _, name := range roots {
						raw, err := z.ReadFile(name + ".class")
						if err != nil || strings.Contains(string(raw), DecompileStubMarker) || name == good && !strings.Contains(string(raw), "class Leaf") {
							t.Fatalf("closed family blocked by opaque peer body: %s %v\n%s", name, err, raw)
						}
						generated[name] = raw
					}
					// This compiler profile cannot regenerate the peer's original
					// lambda MethodParameters. Its body is not cleared by this test.
					peerSource, err := z.ReadFile(opaque + ".class")
					if err != nil || !strings.Contains(string(peerSource), DecompileStubMarker) {
						t.Fatal("opaque peer control unexpectedly cleared", err)
					}
					out := t.TempDir()
					owned := map[string][]byte{}
					for name, raw := range files {
						if strings.HasPrefix(name, good) || name == opaque+"$Token.class" {
							owned[name] = raw
							continue
						}
						if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{"-proc:none", "--release", "8", "-parameters", "-g:" + debug, "-cp", out, "-d", out}
					for _, name := range []string{good, opaque + "$Token"} {
						path := filepath.Join(out, name+".java")
						if err := os.WriteFile(path, generated[name], 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, path)
					}
					if log, err := exec.Command(modern, args...).CombinedOutput(); err != nil {
						t.Fatalf("compile complete owned classes without original fallback: %v\n%s\n%s", err, log, generated)
					}
					if got := t04RunJava(t, java, out, driver); got != want {
						t.Fatalf("same unchanged original caller: %q wanted %q", got, want)
					}
					for name, before := range files {
						after, err := os.ReadFile(filepath.Join(out, name))
						if err != nil {
							t.Fatal(err)
						}
						if owned[name] == nil {
							if string(before) != string(after) {
								t.Fatal("unowned original peer/caller changed", name)
							}
						} else if name == opaque+"$Token.class" {
							nativeLexicalExactSignatures(t, name, before, after)
							nativeAnonymousPrefixRepresentationShape(t, name, before, after)
						} else {
							nativeLexicalExactSignatures(t, name, before, after)
							if nativeBinaryShape(t, before) != nativeBinaryShape(t, after) {
								t.Fatal("owned family physical ABI changed", name)
							}
						}
					}
				})
			}
		})
	}
}

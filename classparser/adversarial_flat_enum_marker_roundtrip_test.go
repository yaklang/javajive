package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Flattening an enum changes the compiler's lexical dummy-parameter owner.
// A null, unused access port must not change the original constructor ABI or
// rename the constant-specific class that happens to share its fresh type.
func TestAdversarialFlatEnumConstructorPortPreservesOriginalABI(t *testing.T) {
	for _, root := range []string{"MarkerEnumHost", "RenamedEnumHost"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, order := range []string{"children-first", "parent-first"} {
				t.Run(root+"/"+debug+"/"+order, func(t *testing.T) {
					flatEnumMemberRoundTrip(t, flatEnumPortFixture(root), root, debug, order == "parent-first")
				})
			}
		}
	}
}

func TestAdversarialFlatPlainEnumRetainsLexicalMembership(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			flatEnumMemberRoundTrip(t, enumArgumentHelperFixture, "ArgumentOwner", debug, false)
		})
	}
}

func TestAdversarialFlatEnumOriginalVersionsKeepUnusedConstructorPorts(t *testing.T) {
	for _, target := range []int{5, 6, 7} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("release%d/%s", target, debug), func(t *testing.T) {
				flatEnumMemberRoundTrip(t, flatEnumPortFixture("VersionPortOwner"), "VersionPortOwner", debug, false, target)
			})
		}
	}
}

func flatEnumMemberRoundTrip(t *testing.T, fixture, root, debug string, parentFirst bool, target ...int) {
	t.Helper()
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	release := 8
	if len(target) > 1 {
		t.Fatal("ambiguous authored compiler target")
	}
	if len(target) == 1 {
		release = target[0]
	}
	files := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, root, debug, nil, release, false)
	if mustParseLowering(t, files[root+"$Mode.class"]).MajorVersion != uint16(44+release) {
		t.Fatal("original compiler target mismatch")
	}
	run := func(dir string) {
		t.Helper()
		for _, mode := range []string{"-ea", "-da"} {
			for _, failure := range []string{"0", "3", "7"} {
				log, err := enumFreshOracleRun(t, java, mode, dir, "ArgumentDriver", failure)
				rows := "0"
				if failure == "0" {
					rows = "10"
				}
				if err != nil || string(log) != rows+":typed:enum:arguments:effects:binding\n" {
					t.Fatal("unchanged original ABI/dispatch/initialization oracle", err, string(log))
				}
			}
		}
	}
	run(sourceArtifactWriteFiles(t, files))
	owner := root + "$Mode"
	owned, external := map[string][]byte{}, map[string][]byte{}
	for name, raw := range files {
		if name == owner+".class" || strings.HasPrefix(name, owner+"$") {
			owned[name] = raw
		} else {
			external[name] = raw
		}
	}
	jar := filepath.Join(t.TempDir(), "flat.jar")
	if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok })
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	source, err := a.ReadFile(owner + ".class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("complete flat enum source", err, string(source))
	}
	out := sourceArtifactWriteFiles(t, external)
	path := filepath.Join(out, owner+".java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(compiler, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
		t.Fatal("fresh flat enum", err, string(log), string(source))
	}
	names := []string{}
	for name := range owned {
		names = append(names, name)
	}
	sort.Strings(names)
	if parentFirst {
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
	}
	// Child names sort before the parent. Each completion must be independent
	// of filesystem iteration order; only the final whole family is executed.
	for _, name := range names {
		path := filepath.Join(out, name)
		fresh, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("missing fresh class", name, err)
		}
		saved := bytes.Clone(fresh)
		done, err := a.FinishClass(name, fresh)
		if err != nil {
			t.Fatal("original enum family constructor contract", name, err)
		}
		if !bytes.Equal(fresh, saved) {
			t.Fatal("fresh input mutated")
		}
		again, err := a.FinishClass(name, done)
		if err != nil || !bytes.Equal(again, done) {
			t.Fatal("non-idempotent completion", name, err)
		}
		if err := os.WriteFile(path, done, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(out)
}

func flatEnumPortFixture(root string) string {
	fixture := strings.ReplaceAll(enumArgumentHelperFixture, "ArgumentOwner", root)
	fixture = strings.Replace(fixture, "FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5)))", "FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5))){public long fold(long input){return baseFold(input)+17;}}", 1)
	fixture = strings.Replace(fixture, "SECOND(Effects.next(7),Long.MAX_VALUE,Double.NaN,new Box(Effects.next(11)))", "SECOND(Effects.next(7),Long.MAX_VALUE,Double.NaN,new Box(Effects.next(11))){public long fold(long input){return baseFold(input);}}", 1)
	fixture = strings.Replace(fixture, "public long fold(long input){return input*wide+tag+box.value+Double.doubleToRawLongBits(real);}", "public abstract long fold(long input);public final long baseFold(long input){return input*wide+tag+box.value+Double.doubleToRawLongBits(real);}", 1)
	fixture = strings.Replace(fixture, "if(modes[i].fold(value)!=expected)", "if(modes[i].fold(value)!=(expected+(i==0?17:0)))", 1)
	fixture = strings.Replace(fixture, "getDeclaredMethods().length!=3", "getDeclaredMethods().length!=4", 1)
	fixture = strings.Replace(fixture, `new AssertionError("method/lexical universe")`, `new AssertionError("method/lexical universe: "+`+root+`.Mode.class.getDeclaredMethods().length+":"+`+root+`.Mode.class.getDeclaringClass())`, 1)
	fixture = strings.Replace(fixture, "int rows=0;for(long value", root+`.Mode.class.getDeclaredConstructor(String.class,int.class,int.class,long.class,double.class,Box.class,Class.forName("`+root+`$1"));
if(modes[0].getClass()==`+root+`.Mode.class||modes[1].getClass()==`+root+`.Mode.class||modes[0].getClass()==modes[1].getClass())throw new AssertionError("distinct constant class identities");int rows=0;for(long value`, 1)
	return fixture
}

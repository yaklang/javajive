package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The outer cache guard and the inner sibling-class conditional are distinct
// joins. A declaration for the outer result must admit every inner leaf, while
// each constructor and cache write stays on the original selected path.
const cachedSiblingPhiFixture = `abstract class CachedPhiValue{abstract int kind();}
class CachedPhiLeft extends CachedPhiValue{int kind(){return 11;}}
class CachedPhiRight extends CachedPhiValue{int kind(){return 23;}}
class CachedPhiEffects{static String trace="";static final RuntimeException failure=new IllegalArgumentException("same");static boolean choose(int mode){trace+="C";if(mode==2)throw failure;return mode==0;}static CachedPhiLeft left(){trace+="L";return new CachedPhiLeft();}static CachedPhiRight right(){trace+="R";return new CachedPhiRight();}}
class CachedPhiOwner{CachedPhiValue resolved;CachedPhiValue resolve(int mode){CachedPhiValue value=resolved;return value==null?(resolved=CachedPhiEffects.choose(mode)?CachedPhiEffects.left():CachedPhiEffects.right()):value;}CachedPhiValue reverse(int mode){CachedPhiValue value=resolved;return value!=null?value:(resolved=CachedPhiEffects.choose(mode)?CachedPhiEffects.right():CachedPhiEffects.left());}}
class CachedPhiDriver{public static void main(String[]args){for(boolean reverse:new boolean[]{false,true})for(int mode=0;mode<3;mode++){CachedPhiOwner owner=new CachedPhiOwner();CachedPhiEffects.trace="";try{CachedPhiValue first=reverse?owner.reverse(mode):owner.resolve(mode);CachedPhiValue second=reverse?owner.reverse(2):owner.resolve(2);System.out.println(reverse+":"+mode+":"+first.kind()+":"+(first==second&&first==owner.resolved)+":"+CachedPhiEffects.trace);}catch(Throwable failure){System.out.println(reverse+":"+mode+":"+(failure==CachedPhiEffects.failure)+":"+(owner.resolved==null)+":"+CachedPhiEffects.trace);}}}}
`

func TestAdversarialCachedSiblingReferencePhiRoundTrip(t *testing.T) {

	const oracle = "false:0:11:true:CL\nfalse:1:23:true:CR\nfalse:2:true:true:C\ntrue:0:23:true:CR\ntrue:1:11:true:CL\ntrue:2:true:true:C\n"
	testSourceTargetReleaseFamilyFixture(t, cachedSiblingPhiFixture, "CachedPhiOwner", "CachedPhiDriver", oracle, "8", []int{8})
}

func TestAdversarialCachedReferencePhiKeepsBothPublicModes(t *testing.T) {
	javac, java := t04Tools(t)
	const oracle = "false:0:11:true:CL\nfalse:1:23:true:CR\nfalse:2:true:true:C\ntrue:0:23:true:CR\ntrue:1:11:true:CL\ntrue:2:true:true:C\n"
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, cachedSiblingPhiFixture, debug)
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "CachedPhiDriver"); got != oracle {
				t.Fatalf("original %q", got)
			}
			resolve := resolverFromClasses(classMapFromDir(t, original))
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(files["CachedPhiOwner.class"], DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
					if err != nil || len(result.StubMethods) != 0 {
						t.Fatalf("%v %+v\n%s", err, result.Diagnostics, result.Source)
					}
					rebuilt := t.TempDir()
					src := filepath.Join(rebuilt, "CachedPhiOwner.java")
					if err := os.WriteFile(src, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, src).CombinedOutput(); err != nil {
						t.Fatalf("compile:%v\n%s\n%s", err, log, result.Source)
					}
					if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "CachedPhiDriver"); got != oracle {
						t.Fatalf("got %q want %q", got, oracle)
					}
				})
			}
		})
	}
}

func TestAdversarialCachedReferencePhiUsesDeclaredNominalFamilies(t *testing.T) {
	const oracle = "false:0:11:true:CL\nfalse:1:23:true:CR\nfalse:2:true:true:C\ntrue:0:23:true:CR\ntrue:1:11:true:CL\ntrue:2:true:true:C\n"
	for _, family := range []string{"interface", "generic base", "unrelated Object"} {
		t.Run(family, func(t *testing.T) {
			fixture := cachedSiblingPhiFixture
			switch family {
			case "interface":
				fixture = strings.Replace(fixture, "abstract class CachedPhiValue{abstract int kind();}", "interface CachedPhiValue{int kind();}", 1)
				fixture = strings.ReplaceAll(fixture, "extends CachedPhiValue{int kind()", "implements CachedPhiValue{public int kind()")
			case "generic base":
				fixture = strings.Replace(fixture, "abstract class CachedPhiValue{", "abstract class CachedPhiValue<T>{", 1)
				fixture = strings.ReplaceAll(fixture, "extends CachedPhiValue{", "extends CachedPhiValue<String>{")
				for _, token := range []string{"resolved", "resolve(", "reverse(", "value="} {
					fixture = strings.ReplaceAll(fixture, "CachedPhiValue "+token, "CachedPhiValue<String> "+token)
				}
			case "unrelated Object":
				fixture = strings.ReplaceAll(fixture, "extends CachedPhiValue{", "{")
				for _, token := range []string{"resolved", "resolve(", "reverse(", "value=", "first=", "second="} {
					fixture = strings.ReplaceAll(fixture, "CachedPhiValue "+token, "Object "+token)
				}
				fixture = strings.ReplaceAll(fixture, "first.kind()", "(first instanceof CachedPhiLeft?11:23)")
			}
			testSourceTargetReleaseFamilyFixture(t, fixture, "CachedPhiOwner", "CachedPhiDriver", oracle, "8", []int{8})
		})
	}
}

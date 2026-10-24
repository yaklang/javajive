package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func nativePrivateEnumScopeFixture(shape, owner string) string {
	fixture := `public class PrivateEnumScope{private enum Phase{FIRST,SECOND,THIRD}private static int step(Phase state,int n){if(n>0)state=Phase.SECOND;switch(state){case FIRST:return 17;case SECOND:return 31;default:return 47;}}public static int run(int n){return step(Phase.FIRST,n);}}
class PrivateEnumDriver{public static void main(String[]a)throws Exception{Class<?> phase=Class.forName("PrivateEnumScope$Phase");if(phase.getDeclaringClass()!=PrivateEnumScope.class||!phase.isMemberClass()||!phase.getSimpleName().equals("Phase"))throw new AssertionError("private enum declaration");if(Class.forName("PrivateEnumScope$1").getEnclosingClass()!=PrivateEnumScope.class)throw new AssertionError("compiler helper enclosing scope");int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){if(PrivateEnumScope.run(n)!=(n>0?31:17))throw new AssertionError("state");rows++;}System.out.println(rows+":private:enum:mutable:lexical");}}`
	switch shape {
	case "direct":
	case "deep":
		fixture = strings.Replace(fixture, "private enum Phase{FIRST,SECOND,THIRD}", "private static class Box{private enum Phase{FIRST,SECOND,THIRD}}", 1)
		fixture = strings.ReplaceAll(fixture, "Phase state", "Box.Phase state")
		fixture = strings.ReplaceAll(fixture, "Phase.FIRST", "Box.Phase.FIRST")
		fixture = strings.ReplaceAll(fixture, "Phase.SECOND", "Box.Phase.SECOND")
		fixture = strings.ReplaceAll(fixture, "PrivateEnumScope$Phase", "PrivateEnumScope$Box$Phase")
		fixture = strings.Replace(fixture, "phase.getDeclaringClass()!=PrivateEnumScope.class", "phase.getDeclaringClass()!=Class.forName(\"PrivateEnumScope$Box\")||phase.getDeclaringClass().getDeclaringClass()!=PrivateEnumScope.class", 1)
	case "two tables":
		fixture = strings.Replace(fixture, "private enum Phase{FIRST,SECOND,THIRD}", "private enum Phase{FIRST,SECOND,THIRD}private enum Other{FIRST,SECOND,THIRD}private static int other(Other state){switch(state){case THIRD:return 5;case FIRST:return 2;default:return 3;}}", 1)
		fixture = strings.Replace(fixture, "return step(Phase.FIRST,n);", "return step(Phase.FIRST,n)+other(Other.THIRD);", 1)
		fixture = strings.Replace(fixture, "(n>0?31:17)", "(n>0?36:22)", 1)
	default:
		panic("unknown authored private enum scope fixture")
	}
	return strings.ReplaceAll(fixture, "PrivateEnumScope", owner)
}

func nativePrivateEnumCompile(t *testing.T, source, owner, debug string) map[string][]byte {
	t.Helper()
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, owner+".java")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
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
}

func TestAdversarialPrivateEnumSwitchArtifactsKeepOriginalLexicalScope(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", err, string(version))
	}
	for _, shape := range []string{"direct", "deep", "two tables"} {
		for _, owner := range []string{"PrivateEnumScope", "ChangedPrivateScope"} {
			t.Run(shape+"/"+owner, func(t *testing.T) {
				fixture := nativePrivateEnumScopeFixture(shape, owner)
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, "PrivateEnumDriver", "5:private:enum:mutable:lexical\n", nil, nativeLexicalExactSignatures,
					func(t *testing.T, name string, original, rebuilt []byte) {
						a, err := Parse(original)
						if err != nil {
							t.Fatal(err)
						}
						if a.AccessFlags&0x1000 == 0 {
							return
						}
						b, err := Parse(rebuilt)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(nativeEnumSwitchOriginalPacketShape(t, a), nativeEnumSwitchOriginalPacketShape(t, b)) || !reflect.DeepEqual(nativeNestedEnumMetadataShape(t, a), nativeNestedEnumMetadataShape(t, b)) {
							t.Fatal("original compiler artifact packet/declaration metadata changed", name)
						}
					})
			})
		}
	}
}

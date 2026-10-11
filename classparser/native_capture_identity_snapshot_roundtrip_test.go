package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const captureIdentitySnapshotFixture3857 = `class SnapshotEffects{static String trace="";static long read(long token){trace+="R";return token;}}
abstract class SnapshotReader{final long early;SnapshotReader(){SnapshotEffects.trace+="P";early=read();}abstract long read();}
class SnapshotOwner{SnapshotReader make(boolean dense,long token){if(dense){final long stride=SnapshotEffects.read(token);return new SnapshotReader(){long read(){return stride;}};}else{final long floor=SnapshotEffects.read(token);return new SnapshotReader(){long read(){return floor;}};}}}
class SnapshotDriver{public static void main(String[]args)throws Exception{int rows=0;for(boolean dense:new boolean[]{true,false})for(long token:new long[]{0,-1,Long.MIN_VALUE,Long.MAX_VALUE}){SnapshotEffects.trace="";SnapshotReader r=new SnapshotOwner().make(dense,token);if(r.read()!=token||r.early!=token||!SnapshotEffects.trace.equals("RP"))throw new AssertionError("word/effect/early callback");java.lang.reflect.Field f=r.getClass().getDeclaredField(dense?"val$stride":"val$floor");f.setAccessible(true);if(f.getType()!=long.class||f.getLong(r)!=token)throw new AssertionError("field identity");rows++;}System.out.println(rows+":snapshot:identity:wide:effects");}}`

func captureIdentitySnapshotFixture(shape, owner string) string {
	fixture := strings.ReplaceAll(captureIdentitySnapshotFixture3857, "SnapshotOwner", owner)
	if shape == "reference" {
		fixture = strings.ReplaceAll(fixture, "long", "Object")
		fixture = strings.Replace(fixture, "new Object[]{0,-1,Long.MIN_VALUE,Long.MAX_VALUE}", `new Object[]{null,new Object(),new String("identity"),new Object()}`, 1)
		fixture = strings.ReplaceAll(fixture, "f.getLong(r)", "f.get(r)")
	}
	if shape == "int" {
		fixture = strings.ReplaceAll(fixture, "long", "int")
		fixture = strings.ReplaceAll(fixture, "Long.MIN_VALUE", "Integer.MIN_VALUE")
		fixture = strings.ReplaceAll(fixture, "Long.MAX_VALUE", "Integer.MAX_VALUE")
		fixture = strings.ReplaceAll(fixture, "f.getLong(r)", "f.getInt(r)")
	}
	return fixture
}
func TestNativeCaptureIdentitySnapshotsKeepDistinctOriginalNames(t *testing.T) {
	for _, shape := range []string{"wide", "int", "reference"} {
		t.Run(shape, func(t *testing.T) {
			for _, owner := range []string{"SnapshotOwner", "ChangedCaptureScope"} {
				t.Run(owner, func(t *testing.T) {
					testNativeIndependentFamilyFixture(t, captureIdentitySnapshotFixture(shape, owner), []string{owner}, "SnapshotDriver", "8:snapshot:identity:wide:effects\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}
func TestNativeCaptureIdentitySnapshotsNativeCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for independent original compiler")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler", err, string(version))
	}
	for _, shape := range []string{"wide", "int", "reference"} {
		t.Run(shape, func(t *testing.T) {
			fixture := captureIdentitySnapshotFixture(shape, "SnapshotOwner")
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				dir := t.TempDir()
				path := filepath.Join(dir, "SnapshotOwner.java")
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
			}, NativeJavac8, javac, []string{"SnapshotOwner"}, "SnapshotDriver", "8:snapshot:identity:wide:effects\n", nil, nativeLexicalExactSignatures)
		})
	}
}

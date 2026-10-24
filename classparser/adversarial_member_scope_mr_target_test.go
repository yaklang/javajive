package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAdversarialMemberScopeUsesSelectedPhysicalRelease(t *testing.T) {
	compiler, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	const fixture = `class ScopeOwner{int count;class Member{Member(){count++;}int read(){return count;}}}
class ScopeDriver{public static void main(String[]args){ScopeOwner owner=new ScopeOwner();ScopeOwner.Member first=owner.new Member();if(first.read()!=1)throw new AssertionError("first capture");ScopeOwner.Member second=owner.new Member();if(first.read()!=2||second.read()!=2)throw new AssertionError("shared receiver identity");System.out.println("2:member:physical:scope");}}`
	for _, release := range []int{9, 10} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("release%d/%s", release, debug), func(t *testing.T) {
				files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "ScopeOwner", debug, false)
				run := func(dir string) {
					t.Helper()
					for _, mode := range []string{"-ea", "-da"} {
						out, err := enumFreshOracleRun(t, java, mode, dir, "ScopeDriver")
						if err != nil || string(out) != "2:member:physical:scope\n" {
							t.Fatal("original receiver behavior", err, string(out))
						}
					}
				}
				run(sourceArtifactWriteFiles(t, files))
				const name = "ScopeOwner$Member.class"
				prefix := fmt.Sprintf("META-INF/versions/%d/", release)
				entries := map[string][]byte{name: files[name], prefix + name: files[name], "META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n")}
				jar := filepath.Join(t.TempDir(), "mr.jar")
				if err := os.WriteFile(jar, t23Zip(t, entries), 0600); err != nil {
					t.Fatal(err)
				}
				a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok })
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				if _, err = a.ReadFile(name); err != nil {
					t.Fatal(err)
				}
				source, err := a.ReadFile(prefix + name)
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatal("physical source", err, string(source))
				}
				view := a.source.jar.sourceReleaseView(release)
				if _, ok := view.sourceConstructorLowering.genericActive["ScopeOwner$Member"]; !ok {
					t.Fatal("missing physical scope receipt")
				}
				deps := map[string][]byte{"ScopeOwner.class": files["ScopeOwner.class"], "ScopeDriver.class": files["ScopeDriver.class"]}
				outer := mustParseLowering(t, deps["ScopeOwner.class"])
				for _, attr := range outer.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						table.Classes = nil
						table.NumberOfClasses = 0
						table.AttrLen = 2
					}
				}
				out := sourceArtifactWriteFiles(t, map[string][]byte{"ScopeOwner.class": outer.Bytes(), "ScopeDriver.class": deps["ScopeDriver.class"]})
				file := filepath.Join(out, "ScopeOwner$Member.java")
				if err = os.WriteFile(file, source, 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "--release", fmt.Sprint(release), "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, file).CombinedOutput(); err != nil {
					t.Fatal("physical target compile", err, string(log), string(source))
				}
				fresh, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatal(err)
				}
				if mustParseLowering(t, fresh).MajorVersion != uint16(release+44) {
					t.Fatal("compiler did not select physical target")
				}
				if b, e := a.FinishClass(name, fresh); e == nil || b != nil {
					t.Fatal("physical target leaked into base namespace")
				}
				other := bytes.Clone(fresh)
				other[7] = 53
				if release == 9 {
					other[7] = 54
				}
				if b, e := a.FinishClass(prefix+name, other); e == nil || b != nil {
					t.Fatal("different physical target admitted")
				}
				finished, err := a.FinishClass(prefix+name, fresh)
				if err != nil {
					t.Fatal("physical scope completion", err)
				}
				if mustParseLowering(t, finished).MajorVersion != uint16(release+44) {
					t.Fatal("changed real compiled target")
				}
				artifactLambdaMetadataCodeUnchanged(t, fresh, finished)
				if again, e := a.FinishClass(prefix+name, finished); e != nil || !bytes.Equal(again, finished) {
					t.Fatal("not idempotent", e)
				}
				if err = os.WriteFile(filepath.Join(out, name), finished, 0600); err != nil {
					t.Fatal(err)
				}
				for n, b := range deps {
					if err = os.WriteFile(filepath.Join(out, n), b, 0600); err != nil {
						t.Fatal(err)
					}
				}
				run(out)
			})
		}
	}
}

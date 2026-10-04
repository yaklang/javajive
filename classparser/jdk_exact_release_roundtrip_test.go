package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A classfile major selects its exact platform declaration profile. Object's
// original no-arg constructor must be proved, never inferred from a neighboring
// release or replaced with an unsupported source body.
func TestAdversarialPlatformExactReleaseConstructorRoundTrip(t *testing.T) {
	const source = `class ExactReleaseSubject {
 final Object reference; final long number;
 ExactReleaseSubject(Object reference,long number){this.reference=reference;this.number=number;}
 Object reference(){return reference;} long number(){return number;}
}
class ExactReleaseDriver {public static void main(String[]args)throws Exception{
 Object token=new Object();int rows=0;
 for(Object reference:new Object[]{null,token})for(long number:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
 ExactReleaseSubject s=new ExactReleaseSubject(reference,number);
 if(s.reference()!=reference||s.number()!=number)throw new AssertionError("constructor identity/value");
 if(ExactReleaseSubject.class.getDeclaredField("reference").get(s)!=reference||ExactReleaseSubject.class.getDeclaredField("number").getLong(s)!=number)throw new AssertionError("field identity/value");rows++;
 }
 System.out.println(rows+":"+ExactReleaseSubject.class.getDeclaredConstructor(Object.class,long.class).getParameterCount());
}}`
	javac, java := t04Tools(t)
	for _, release := range []string{"9", "16"} {
		t.Run(release, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileReleaseClasses(t, source, debug, release)
					original := t.TempDir()
					for name, raw := range files {
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					oracle := t04RunJava(t, java, original, "ExactReleaseDriver")
					if oracle != "10:2\n" {
						t.Fatalf("independent oracle: %q", oracle)
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
							src, err := z.ReadFile("ExactReleaseSubject.class")
							if err != nil || strings.Contains(string(src), DecompileStubMarker) {
								t.Fatalf("exact original profile constructor: %v\n%s", err, src)
							}
							output := t.TempDir()
							if err := os.WriteFile(filepath.Join(output, "ExactReleaseDriver.class"), files["ExactReleaseDriver.class"], 0600); err != nil {
								t.Fatal(err)
							}
							file := filepath.Join(output, "ExactReleaseSubject.java")
							if err := os.WriteFile(file, src, 0600); err != nil {
								t.Fatal(err)
							}
							if out, err := exec.Command(javac, "-proc:none", "--release", release, "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
								t.Fatalf("rebuilt: %v %s\n%s", err, out, src)
							}
							if got := t04RunJava(t, java, output, "ExactReleaseDriver"); got != oracle {
								t.Fatalf("original constructor semantics changed: %q != %q", got, oracle)
							}
							raw, err := os.ReadFile(filepath.Join(output, "ExactReleaseSubject.class"))
							if err != nil {
								t.Fatal(err)
							}
							if want, got := nativeBinaryShape(t, files["ExactReleaseSubject.class"]), nativeBinaryShape(t, raw); got != want {
								t.Fatalf("original ABI changed:\n%s\n%s", want, got)
							}
						})
					}
				})
			}
		})
	}
}

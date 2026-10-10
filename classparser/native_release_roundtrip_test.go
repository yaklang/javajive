package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Rebuild every source unit in each physical namespace. Merely keeping the
// cached base source distinct does not prove that versioned NEW instructions
// can still name a child whose base source unit was folded into its owner.
func TestNativeReleaseOwnershipRoundTrip(t *testing.T) {
	_, java := t04Tools(t)
	javac, _ := t04Tools(t)
	const fixture = `class ReleaseValue{public static class Box{final long n;Box(long value){n=value;}long read(){return n;}}}
 class NativeArchiveOwner{static ReleaseValue.Box make(final String token){return new ReleaseValue.Box(%d){long read(){return token==null?-super.read():super.read()+token.length();}};}}
 class ReleaseDriver{public static void main(String[]args){ReleaseValue.Box a=NativeArchiveOwner.make(null);ReleaseValue.Box b=NativeArchiveOwner.make("scope");System.out.println(a.read()+":"+b.read()+":"+a.n+":"+b.n);}}`
	base := nativeCompileClasses(t, fmt.Sprintf(fixture, 7))
	versions := map[int]map[string][]byte{9: nativeCompileClasses(t, fmt.Sprintf(fixture, 19)), 11: nativeCompileClasses(t, fmt.Sprintf(fixture, 31))}
	files := map[string][]byte{}
	for n, b := range base {
		files[n] = b
	}
	for release, decls := range versions {
		for n, b := range decls {
			// Version 9 retains the base member declaration, and replaces only its user.
			if release == 9 && strings.HasPrefix(n, "ReleaseValue") {
				continue
			}
			files[fmt.Sprintf("META-INF/versions/%d/", release)+n] = b
		}
	}
	files["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")
	for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "no-source-rewrites" {
				t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
			}
			if mode == "no-core-cleanups" {
				t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
			}
			z := nativeArchive(t, files)
			// Read a later namespace first, then the base and earlier namespace. Source
			// ownership and name caches must be independent of this order.
			for _, release := range []int{11, 0, 9} {
				t.Run(fmt.Sprint(release), func(t *testing.T) {
					effective := map[string][]byte{}
					for n, b := range base {
						effective[n] = b
					}
					prefix := ""
					if release > 0 {
						prefix = fmt.Sprintf("META-INF/versions/%d/", release)
						for n, b := range versions[release] {
							if _, found := files[prefix+n]; found {
								effective[n] = b
							}
						}
					}
					original := t.TempDir()
					rebuilt := t.TempDir()
					for n, b := range effective {
						p := filepath.Join(original, n)
						if e := os.WriteFile(p, b, 0600); e != nil {
							t.Fatal(e)
						}
					}
					oracle := t04RunJava(t, java, original, "ReleaseDriver")
					if e := os.WriteFile(filepath.Join(rebuilt, "ReleaseDriver.class"), effective["ReleaseDriver.class"], 0600); e != nil {
						t.Fatal(e)
					}
					paths := []string{}
					for _, n := range []string{"NativeArchiveOwner.class", "NativeArchiveOwner$1.class", "ReleaseValue.class", "ReleaseValue$Box.class"} {
						physical := n
						if _, found := files[prefix+n]; prefix != "" && found {
							physical = prefix + n
						}
						src, e := z.ReadFile(physical)
						if e != nil || strings.Contains(string(src), DecompileStubMarker) {
							t.Fatalf("source %s %v %s", physical, e, src)
						}
						p := filepath.Join(rebuilt, strings.TrimSuffix(n, ".class")+".java")
						if e := os.WriteFile(p, src, 0600); e != nil {
							t.Fatal(e)
						}
						paths = append(paths, p)
					}
					if out, e := exec.Command(javac, append([]string{"-proc:none", "--release", "8", "-cp", rebuilt, "-d", rebuilt}, paths...)...).CombinedOutput(); e != nil {
						t.Fatalf("namespace %d rebuilt: %v %s", release, e, out)
					}
					for _, n := range []string{"NativeArchiveOwner", "NativeArchiveOwner$1", "ReleaseValue", "ReleaseValue$Box"} {
						b, e := os.ReadFile(filepath.Join(rebuilt, n+".class"))
						if e != nil {
							t.Fatal(e)
						}
						if want, got := nativeBinaryShape(t, effective[n+".class"]), nativeBinaryShape(t, b); want != got {
							t.Fatalf("namespace %d ABI %s\n%s\n!=\n%s", release, n, want, got)
						}
					}
					if got := t04RunJava(t, java, rebuilt, "ReleaseDriver"); got != oracle {
						t.Fatalf("namespace %d got %q want %q", release, got, oracle)
					}
				})
			}
		})
	}
}

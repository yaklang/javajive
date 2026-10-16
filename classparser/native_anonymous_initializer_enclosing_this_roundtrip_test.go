package javaclassparser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func enclosingThisInitializerFixture(shape string) (string, string) {
	owner, fixture := lexicalInitializerCaptureShape(shape)
	_, driver, found := strings.Cut(fixture, "\n")
	if !found {
		panic("missing independently compiled input and driver")
	}
	driver = strings.Replace(driver, `new AssertionError("scope")`, `new AssertionError("scope result="+result.getClass().getName()+"/"+result.getClass().isAnonymousClass()+" cursor="+cursor.getClass().getName()+"/"+cursor.getClass().isAnonymousClass()+" method="+cursor.getClass().getEnclosingMethod())`, 1)
	root := fmt.Sprintf(`import java.util.*;class %s<T> {
 final Iterable<T> input;
 %s(Iterable<T> input){this.input=input;}
 class Stage {
  Iterable<T> create(){return new Iterable<T>() {
   public Iterator<T> iterator(){return new Iterator<T>() {
    final Iterator<T> backing=%s.this.input.iterator();
    public boolean hasNext(){return backing.hasNext();}
    public T next(){return backing.next();}
    public void remove(){throw new UnsupportedOperationException();}
   };}
  };}
 }
}
class EnclosingOriginalFactory {
 static <T> Iterable<T> produce(Iterable<T> input){return new %s<T>(input).new Stage().create();}
}
`, owner, owner, owner, owner)
	if shape == "direct-root" {
		root = fmt.Sprintf(`import java.util.*;class %s<T> {
 final Iterable<T> input;
 %s(Iterable<T> input){this.input=input;}
 Iterable<T> create(){return new Iterable<T>() {
  public Iterator<T> iterator(){return new Iterator<T>() {
   final Iterator<T> backing=%s.this.input.iterator();
   public boolean hasNext(){return backing.hasNext();}
   public T next(){return backing.next();}
   public void remove(){throw new UnsupportedOperationException();}
  };}
 };}
}
class EnclosingOriginalFactory {
 static <T> Iterable<T> produce(Iterable<T> input){return new %s<T>(input).create();}
}
`, owner, owner, owner, owner)
	}
	if shape == "method-shadow" {
		// The method's Number-bound T is a different binder from the owner's
		// unbounded T. Qualified THIS must still reach the owner's String data.
		root = fmt.Sprintf(`import java.util.*;class %s<T> {
 final Iterable<T> input;
 %s(Iterable<T> input){this.input=input;}
 class Stage {
  <T extends Number> Iterable<?> create(){return new Iterable<Object>() {
   public Iterator<Object> iterator(){return new Iterator<Object>() {
    final Iterator<?> backing=%s.this.input.iterator();
    public boolean hasNext(){return backing.hasNext();}
    public Object next(){return backing.next();}
    public void remove(){throw new UnsupportedOperationException();}
   };}
  };}
 }
}
class EnclosingOriginalFactory {
 static <T> Iterable<T> produce(Iterable<T> input){return (Iterable<T>)(Iterable<?>)new %s<T>(input).new Stage().<Integer>create();}
}
`, owner, owner, owner, owner)
	}
	if shape == "renamed" {
		root = strings.ReplaceAll(root, "backing", "snapshotCursor")
	}
	if shape == "generic-intermediate" {
		root = strings.Replace(root, "class Stage {", "class Stage<Marker> {", 1)
		root = strings.Replace(root, "new Stage()", "new Stage<Object>()", 1)
	}
	// Keep the factory binary: its original constructor calls independently
	// exercise the regenerated family without making a second source motion
	// problem part of the initializer proof.
	driver = strings.ReplaceAll(driver, owner+".produce(input)", "EnclosingOriginalFactory.produce(input)")
	return owner, root + driver
}

func testNativeAnonymousInitializerEnclosingThis(t *testing.T, native bool) {
	for _, shape := range []string{"reference", "array-elements", "renamed", "generic-intermediate", "direct-root", "method-shadow"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := enclosingThisInitializerFixture(shape)
			want := "6:lexical:initializer:producer:identity:scope\n"
			compiler, _ := t04Tools(t)
			profile := ModernJavac
			if native {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required for independent compiler")
				}
				profile = NativeJavac8
			}
			compile := func(debug string) map[string][]byte {
				if native {
					return nativePrivateEnumCompile(t, fixture, owner, debug)
				}
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			}
			archive := func(t *testing.T, files map[string][]byte) *JarFS {
				// The decompilation JAR contains exactly the owner family. Input,
				// factory and caller stay binary dependencies at both compiler
				// profiles; the runtime/ABI oracle still uses their original bytes.
				family := map[string][]byte{}
				for n, raw := range files {
					if n == owner+".class" || strings.HasPrefix(n, owner+"$") {
						family[n] = raw
					}
				}
				jar := filepath.Join(t.TempDir(), "family.jar")
				if err := os.WriteFile(jar, t23Zip(t, family), 0600); err != nil {
					t.Fatal(err)
				}
				z, err := NewJarFSFromLocalWithCompilerProfile(jar, 8, profile, func(name string) ([]byte, bool) {
					raw, known := files[name+".class"]
					return raw, known
				})
				if err != nil {
					t.Fatal(err)
				}
				return z
			}
			testNativeIndependentCompilerFamilyWithArchive(t, compile, profile, compiler, []string{owner}, "LexicalInitializerDriver", want, nil, archive, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousInitializerEnclosingThis(t *testing.T) {
	testNativeAnonymousInitializerEnclosingThis(t, false)
}

func TestAdversarialAnonymousInitializerEnclosingThisNativeCompiler(t *testing.T) {
	testNativeAnonymousInitializerEnclosingThis(t, true)
}

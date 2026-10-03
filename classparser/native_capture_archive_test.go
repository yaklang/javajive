package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
)

func nativeCompileClasses(t *testing.T, source string) map[string][]byte {
	t.Helper()
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "NativeArchiveOwner.java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original: %v\n%s", err, out)
	}
	files := map[string][]byte{}
	if err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".class") {
			rel, e := filepath.Rel(dir, p)
			if e != nil {
				return e
			}
			raw, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			files[filepath.ToSlash(rel)] = raw
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}
func nativeArchive(t *testing.T, files map[string][]byte) *JarFS {
	t.Helper()
	raw := t23Zip(t, files)
	zip, err := filesys.NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return NewJarFS(zip)
}

func TestNativeAnonymousCacheKeepsArchiveEntryAndPolicyIdentity(t *testing.T) {
	source := `class NativeArchiveOwner {static int marker(){return %d;}static Runnable make(final Object x){return new Runnable(){public void run(){if(x==null)throw new IllegalArgumentException();}};}}`
	base := nativeCompileClasses(t, fmt.Sprintf(source, 1))
	version := nativeCompileClasses(t, fmt.Sprintf(source, 2))
	files := map[string][]byte{}
	for name, raw := range base {
		files[name] = raw
		files[strings.TrimSuffix(name, ".class")+".raw"] = raw
	}
	for name, raw := range version {
		files["META-INF/versions/9/"+name] = raw
	}
	files["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")
	for _, first := range []string{"NativeArchiveOwner.class", "META-INF/versions/9/NativeArchiveOwner.class", "NativeArchiveOwner.raw"} {
		t.Run(first, func(t *testing.T) {
			z := nativeArchive(t, files)
			if _, err := z.ReadFile(first); err != nil {
				t.Fatal(err)
			}
			var wait sync.WaitGroup
			outputs := make(chan string, 8)
			for i := 0; i < 8; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					raw, err := z.ReadFile("NativeArchiveOwner.class")
					if err != nil {
						outputs <- err.Error()
					} else {
						outputs <- string(raw)
					}
				}()
			}
			wait.Wait()
			close(outputs)
			for text := range outputs {
				if !strings.Contains(text, "return 1;") || !strings.Contains(text, "jdec-owned-anonymous-ordinal:1") {
					t.Fatalf("base identity:\n%s", text)
				}
			}
			raw, err := z.ReadFile("META-INF/versions/9/NativeArchiveOwner.class")
			if err != nil || !strings.Contains(string(raw), "return 2;") || strings.Contains(string(raw), "jdec-owned-anonymous-ordinal:") {
				t.Fatalf("version identity: %v\n%s", err, raw)
			}
			raw, err = z.ReadFile("NativeArchiveOwner$1.class")
			if err != nil || !strings.Contains(string(raw), "original anonymous body owned by") {
				t.Fatalf("normal ownership: %v\n%s", err, raw)
			}
			t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
			raw, err = z.ReadFile("NativeArchiveOwner$1.class")
			if err != nil || strings.Contains(string(raw), "original anonymous body owned by") {
				t.Fatalf("policy cache leak: %v\n%s", err, raw)
			}
		})
	}
}

func TestNativeAnonymousOwnershipAndConstructorRefusals(t *testing.T) {
	for _, scenario := range []struct {
		name, source string
		accept       bool
	}{
		{"plain capture", `class NativeArchiveOwner {static Runnable make(final Object x){return new Runnable(){public void run(){if(x==null)throw new IllegalArgumentException();}};}}`, true},
		{"post-super initialization", `class NativeArchiveOwner {static Runnable make(final Object x){return new Runnable(){final Object y=x;public void run(){if(y==null)throw new IllegalArgumentException();}};}}`, false},
		{"nested member owner", `class NativeArchiveOwner {static Runnable make(){return new Runnable(){class Nested{int read(){return 1;}}public void run(){new Nested().read();}};}}`, false},
		{"nested anonymous owner", `class NativeArchiveOwner {static Runnable make(){return new Runnable(){public void run(){new Runnable(){public void run(){}}.run();}};}}`, false},
		{"lexical formal shadow", `class NativeArchiveOwner {static <E> Object make(final java.util.List<E> x){return new Object(){<E> Object get(){return x.get(0);}};}}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			z := nativeArchive(t, nativeCompileClasses(t, scenario.source))
			src, err := z.ReadFile("NativeArchiveOwner.class")
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(string(src), "jdec-owned-anonymous-ordinal:")
			if got != scenario.accept {
				t.Fatalf("ownership verdict %v expected %v:\n%s", got, scenario.accept, src)
			}
			child, err := z.ReadFile("NativeArchiveOwner$1.class")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(child), "original anonymous body owned by") != scenario.accept {
				t.Fatal("suppressed unowned child")
			}
		})
	}
}

func TestNativeAnonymousProofRejectsUnrepresentableClassMetadata(t *testing.T) {
	files := nativeCompileClasses(t, `class NativeArchiveOwner {static Runnable make(final Object x){return new Runnable(){public void run(){if(x==null)throw new IllegalArgumentException();}};}}`)
	for _, scenario := range []string{"class annotation", "type annotation", "class flags", "capture flags", "missing enclosing metadata", "named inner row"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["NativeArchiveOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			owner, method, anon := originalAnonymousOwner(obj)
			if !anon || nativeAnonymousConstructor(obj, owner, method, nil) == nil {
				t.Fatal("positive proof missing")
			}
			switch scenario {
			case "class annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleAnnotationsAttribute{})
			case "type annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleTypeAnnotationsAttribute{})
			case "class flags":
				obj.AccessFlags |= 0x0010
			case "capture flags":
				obj.Fields[0].AccessFlags &^= 0x1000
			case "missing enclosing metadata":
				for i, a := range obj.Attributes {
					if u, ok := a.(*UnparsedAttribute); ok && u.Name == "EnclosingMethod" {
						obj.Attributes = append(obj.Attributes[:i], obj.Attributes[i+1:]...)
						break
					}
				}
			case "named inner row":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						for _, row := range table.Classes {
							if name, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex); known && name == obj.GetClassName() {
								row.InnerNameIndex = obj.ThisClass
								break
							}
						}
					}
				}
			}
			_, _, owned := originalAnonymousOwner(obj)
			if owned && nativeAnonymousConstructor(obj, owner, method, nil) != nil {
				t.Fatal("accepted unrepresentable metadata")
			}
		})
	}
}

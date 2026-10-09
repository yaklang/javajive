package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// InnerClasses describes a binary declaration. Its STATIC bit is not the
// lexical method's access flag: older compilers emit it for anonymous classes
// created in static methods. The original EnclosingMethod tuple selects the
// declaration that owns the type variable, including shadowing and static cuts.
func TestAdversarialFlattenedStaticContextRowsKeepMethodBinding(t *testing.T) {
	for _, prefix := range []string{"ErasureCapture", "SeparateContext"} {
		t.Run(prefix, func(t *testing.T) {
			fixture := strings.ReplaceAll(flattenedMethodBindingFixture, "ErasureCapture", prefix)
			childName := prefix + "Owner$1"
			testIndependentFlatMutatedClosedCalleeFamily(t, fixture, prefix, "10:lexical:method:erasure\n", []string{childName}, func(t *testing.T, files map[string][]byte) {
				for _, name := range []string{prefix + "Owner", childName} {
					obj, err := Parse(bytes.Clone(files[name+".class"]))
					if err != nil {
						t.Fatal(err)
					}
					matches := 0
					for _, attr := range obj.Attributes {
						if table, ok := attr.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								identity, known := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
								if known && identity == childName {
									row.InnerClassAccessFlags |= StaticFlag
									matches++
								}
							}
						}
					}
					if matches != 1 {
						t.Fatal("unique original context row", name, matches)
					}
					files[name+".class"] = obj.Bytes()
				}
			})
		})
	}
}

// This separate oracle uses genuine javac8 output, not a patched class file.
// Modern --release 8 does not reproduce the original InnerClasses encoding.
func TestAdversarialFlattenedNativeCompilerContextRows(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("real javac8 original oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(compiler, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("original compiler identity", err, string(version))
	}
	for _, layout := range []string{"static anonymous", "instance anonymous", "static named"} {
		t.Run(layout, func(t *testing.T) {
			fixture, child := flattenedMethodBindingFixture, "ErasureCaptureOwner$1"
			if layout == "instance anonymous" {
				fixture = strings.ReplaceAll(fixture, "static<T extends ErasureCaptureBound>", "<T extends ErasureCaptureBound>")
				fixture = strings.ReplaceAll(fixture, "ErasureCaptureOwner.make(token)", "new ErasureCaptureOwner<Number>().make(token)")
			}
			if layout == "static named" {
				fixture = strings.ReplaceAll(fixture, "return new ErasureCaptureConverter<Integer,T>(){", "class Entry implements ErasureCaptureConverter<Integer,T>{")
				fixture = strings.ReplaceAll(fixture, "}};}}", "}}return new Entry();}}")
				child = "ErasureCaptureOwner$1Entry"
			}
			compile := func(debug string) map[string][]byte {
				root := t.TempDir()
				path := filepath.Join(root, "OriginalContext.java")
				if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
					t.Fatal(err)
				}
				if log, err := exec.Command(compiler, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
					t.Fatal("authored original javac8", err, string(log))
				}
				entries, err := os.ReadDir(root)
				if err != nil {
					t.Fatal(err)
				}
				files := map[string][]byte{}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".class") {
						b, err := os.ReadFile(filepath.Join(root, entry.Name()))
						if err != nil {
							t.Fatal(err)
						}
						files[entry.Name()] = b
					}
				}
				if layout == "static anonymous" {
					obj, err := Parse(bytes.Clone(files[child+".class"]))
					if err != nil {
						t.Fatal(err)
					}
					staticRows := 0
					for _, attr := range obj.Attributes {
						if table, ok := attr.(*InnerClassesAttribute); ok {
							for _, row := range table.Classes {
								if row.InnerClassInfoIndex == obj.ThisClass && row.InnerClassAccessFlags&StaticFlag != 0 {
									staticRows++
								}
							}
						}
					}
					if staticRows != 1 {
						t.Fatal("actual legacy anonymous context encoding", staticRows)
					}
				}
				return files
			}
			testIndependentFlatCompilerClosedCalleeFamily(t, compile, "ErasureCapture", "10:lexical:method:erasure\n", []string{child}, nil)
		})
	}
}

package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// This is a static compilation/descriptor contract, not foreign JVM execution
// or whole-class behavioral acceptance. Unsupported unrelated methods cannot
// stand in for any of the three constructor bodies checked below.
func testReviewedJoinConstructorContract(t *testing.T) {
	t.Helper()
	const owner = "org/assertj/core/condition/Join"
	raw, err := os.ReadFile("testdata/regression/Join.class")
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarInvoke(t, "testdata/regression/Join.class", "<init>", "([Lorg/assertj/core/api/Condition;)V", 11, core.OP_INVOKESPECIAL, owner, "<init>", "(Ljava/util/stream/Stream;)V")
	// Eight unchanged original declarations from assertj-core 3.24.2,
	// original JAR SHA-256 df3d0b348f1fe806bdddcb10fa4ae63c6679e9888d4bc7055f09848517976aa3.
	// The 20KB fixture includes producer metadata and the Apache 2 license;
	// it is never executed. CI must not borrow a developer's Maven cache or
	// fetch mutable declarations before checking the original binding.
	archive := "testdata/regression/Join-declarations.jar"
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal("required bundled original declaration-only fixture", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(archiveBytes)) != "da884b1ee1f4f0885046b2f91eabc795e1975f668450d2fa0b7ea0519a02924d" {
		t.Fatal("original declaration fixture differs from its locked bytes")
	}
	original, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]*zip.File{}
	for _, f := range original.File {
		if strings.HasSuffix(f.Name, ".class") && !strings.HasPrefix(f.Name, "META-INF/") {
			entries[strings.TrimSuffix(f.Name, ".class")] = f
		}
	}
	resolve := func(name string) ([]byte, bool) {
		f := entries[name]
		if f == nil {
			return nil, false
		}
		r, err := f.Open()
		if err != nil {
			return nil, false
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		return b, err == nil
	}
	locked, known := resolve(owner)
	if !known || !bytes.Equal(locked, raw) {
		t.Fatal("constructor fixture differs from its original declaration archive")
	}
	modern, _ := t04Tools(t)
	compilers := []struct{ name, path string }{{"modern", modern}}
	if native := os.Getenv("JAVA8_JAVAC"); native != "" {
		compilers = append(compilers, struct{ name, path string }{"native8", native})
	}
	want := reviewedJoinConstructorCalls(t, raw)
	parameters := reviewedJoinConstructorParameters(t, raw)
	if len(want) != 3 {
		t.Fatal("original constructor declaration inventory", want)
	}
	for _, off := range []string{"", "1"} {
		t.Run("legacy="+off, func(t *testing.T) {
			t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", off)
			dir := t.TempDir()
			input := filepath.Join(dir, "source-subset.jar")
			if err := os.WriteFile(input, t23Zip(t, map[string][]byte{owner + ".class": raw}), 0600); err != nil {
				t.Fatal(err)
			}
			z, err := NewJarFSFromLocalWithCompilerProfile(input, 8, ModernJavac, resolve)
			if err != nil {
				t.Fatal(err)
			}
			defer z.Close()
			source, err := z.ReadFile(owner + ".class")
			if err != nil {
				t.Fatal(err)
			}
			for _, parameter := range []string{`Condition<\? super T>\.\.\.\s+\w+`, `Iterable<[^{}]+>\s+\w+`, `Stream<[^{}]+>\s+\w+`} {
				body := reviewedSourceMethod(t, string(source), `Join\(`+parameter+`\)\s*\{`)
				if strings.Contains(body, DecompileStubMarker) {
					t.Fatal("original constructor unavailable", body)
				}
			}
			path := filepath.Join(dir, "Join.java")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			for _, compiler := range compilers {
				t.Run(compiler.name, func(t *testing.T) {
					out := t.TempDir()
					empty := t.TempDir()
					args := []string{"-proc:none", "-parameters", "--release", "8"}
					if compiler.name == "native8" {
						args = []string{"-proc:none", "-parameters", "-source", "8", "-target", "8"}
					}
					args = append(args, "-g:none", "-sourcepath", empty, "-classpath", archive, "-d", out, path)
					if log, err := exec.Command(compiler.path, args...).CombinedOutput(); err != nil {
						t.Fatalf("scoped original constructor compilation: %v\n%s\n%s", err, log, source)
					}
					rebuilt, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(owner)+".class"))
					if err != nil {
						t.Fatal(err)
					}
					if got := reviewedJoinConstructorCalls(t, rebuilt); !reflect.DeepEqual(want, got) {
						t.Fatalf("original constructor binding changed: want=%v got=%v", want, got)
					}
					if got := reviewedJoinConstructorParameters(t, rebuilt); !reflect.DeepEqual(parameters, got) {
						t.Fatalf("original constructor parameter metadata changed: want=%v got=%v", parameters, got)
					}
				})
			}
		})
	}
}

func reviewedJoinConstructorParameters(t *testing.T, raw []byte) map[string][]independentMethodParameterName {
	t.Helper()
	result := map[string][]independentMethodParameterName{}
	for key, names := range methodParameterNamesIndependent(t, raw) {
		if strings.HasPrefix(key, "<init>") {
			result[key] = names
		}
	}
	return result
}

func reviewedJoinConstructorCalls(t *testing.T, raw []byte) map[string][]string {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string][]string{}
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		if name != "<init>" {
			continue
		}
		desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		result[desc] = nil
		d := core.NewDecompiler(methodCodeBytes(method), nil)
		if err := d.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		for _, op := range d.Opcodes() {
			if op.Instr.OpCode != core.OP_INVOKESPECIAL {
				continue
			}
			index := uint16(op.Data[0])<<8 | uint16(op.Data[1])
			member, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
			if !ok {
				t.Fatal("constructor invocation member")
			}
			pair, ok := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			if !ok {
				t.Fatal("constructor invocation tuple")
			}
			owner, _ := sourceBridgeClassName(obj, member.ClassIndex)
			callee, _ := sourceBridgeUTF8(obj, pair.NameIndex)
			target, _ := sourceBridgeUTF8(obj, pair.DescriptorIndex)
			if callee == "<init>" {
				result[desc] = append(result[desc], owner+"."+callee+target)
			}
		}
	}
	return result
}

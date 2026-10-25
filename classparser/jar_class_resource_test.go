package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestClassResourceCompletionRequiresIndependentPhysicalPublication(t *testing.T) {
	javac, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, javac, ModernJavac, `public class StoredValue {public long apply(final long left,long right){return left*31L+right;}}`, "StoredValue", "source,lines,vars", true)
	original := files["StoredValue.class"]
	path := filepath.Join(t.TempDir(), "resource.jar")
	if err := os.WriteFile(path, t23Zip(t, map[string][]byte{"bytes/value.bin": original, "bytes/alias.bin": original, "StoredValue.class": original, "bad.bin": []byte("not a class")}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	source, owner, err := a.ReadClassResource("bytes/value.bin")
	if err != nil || owner != "StoredValue" {
		t.Fatal(err, owner)
	}
	fresh := compileLambdaParameterMetadataFixture(t, javac, ModernJavac, string(source), owner, "source,lines,vars", false)["StoredValue.class"]
	for _, variant := range []string{"control", "unpublished", "plain read", "other physical alias", "stale original", "empty source receipt", "foreign owner", "extra method", "changed visibility", "changed descriptor", "oversized compiler output"} {
		t.Run(variant, func(t *testing.T) {
			view, err := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer view.Close()
			name := "bytes/value.bin"
			compiled := bytes.Clone(fresh)
			if variant != "unpublished" && variant != "plain read" {
				if _, _, err := view.ReadClassResource(name); err != nil {
					t.Fatal(err)
				}
			}
			switch variant {
			case "plain read":
				if _, err := view.ReadFile("StoredValue.class"); err != nil {
					t.Fatal(err)
				}
			case "other physical alias":
				name = "bytes/alias.bin"
			case "stale original", "empty source receipt":
				p := view.resources[name]
				if variant == "stale original" {
					p.original[0] ^= 1
				} else {
					p.source = [32]byte{}
				}
				view.resources[name] = p
			case "foreign owner", "extra method", "changed visibility", "changed descriptor":
				o := mustParseLowering(t, compiled)
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if n != "apply" {
						continue
					}
					switch variant {
					case "extra method":
						copy := *m
						copy.NameIndex = uint16(o.ConstantPoolManager.AddUtf8Info("another"))
						o.Methods = append(o.Methods, &copy)
					case "changed visibility":
						m.AccessFlags = 2
					case "changed descriptor":
						m.DescriptorIndex = uint16(o.ConstantPoolManager.AddUtf8Info("(JI)J"))
					}
				}
				if variant == "foreign owner" {
					o.ConstantPool[o.ThisClass-1].(*ConstantClassInfo).NameIndex = uint16(o.ConstantPoolManager.AddUtf8Info("DifferentValue"))
				}
				compiled = o.Bytes()
			case "oversized compiler output":
				compiled = append(compiled, make([]byte, 32<<20)...)
			}
			before := sha256.Sum256(compiled)
			out, err := view.FinishClassResource(name, compiled)
			if variant != "control" {
				if err == nil || out != nil {
					t.Fatal("unproved resource promoted", err)
				}
				if sha256.Sum256(compiled) != before {
					t.Fatal("failed transaction mutated input")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sha256.Sum256(compiled) != before {
				t.Fatal("input mutated")
			}
			left, right := mustParseLowering(t, compiled), mustParseLowering(t, out)
			for i, m := range left.Methods {
				if !reflect.DeepEqual(constructorLoweringCode(m), constructorLoweringCode(right.Methods[i])) {
					t.Fatal("resource completion edited executable Code")
				}
			}
			if path, out, err := view.FinishOutput("StoredValue.class", compiled); err == nil || out != nil || path != "" {
				t.Fatal("ordinary/resource collision promoted")
			}
		})
	}
	for _, name := range []string{"", ".", "../bytes/value.bin", "/bytes/value.bin", "bytes//value.bin", "bytes/./value.bin", "bytes\\value.bin", "StoredValue.class", "StoredValue.CLASS", "META-INF/versions/9/value.bin", "bad.bin", "absent.bin"} {
		t.Run("invalid/"+name, func(t *testing.T) {
			if source, owner, err := a.ReadClassResource(name); err == nil || source != nil || owner != "" {
				t.Fatal("invalid input published", name, err)
			}
			if out, err := a.FinishClassResource(name, fresh); err == nil || out != nil {
				t.Fatal("invalid input finished", name, err)
			}
		})
	}
	duplicate := filepath.Join(t.TempDir(), "duplicate.jar")
	if err := os.WriteFile(duplicate, jarReleaseTestZIP(t, []jarReleaseTestEntry{{"value.bin", original}, {"value.bin", original}}), 0600); err != nil {
		t.Fatal(err)
	}
	dup, err := NewJarReconstructionArtifactFromLocal(duplicate, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer dup.Close()
	if _, _, err := dup.ReadClassResource("value.bin"); err == nil {
		t.Fatal("duplicate physical supplier admitted")
	}
	t.Run("two resources cannot claim one compiler output", func(t *testing.T) {
		jar := filepath.Join(t.TempDir(), "aliases.jar")
		if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{"first.bin": original, "second.bin": original}), 0600); err != nil {
			t.Fatal(err)
		}
		view, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer view.Close()
		for _, name := range []string{"second.bin", "first.bin"} {
			if _, _, err := view.ReadClassResource(name); err != nil {
				t.Fatal(err)
			}
			// Each explicit physical request has its own legitimate receipt.
			if _, err := view.FinishClassResource(name, fresh); err != nil {
				t.Fatal(err)
			}
		}
		// A compiler's binary-name output has no evidence selecting either
		// defining-loader resource, regardless of publication order.
		if name, out, err := view.FinishOutput("StoredValue.class", fresh); err == nil || name != "" || out != nil {
			t.Fatal("ambiguous resource output promoted", name, err)
		}
	})
}

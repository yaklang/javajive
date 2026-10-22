package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func assertionStorageArtifact(t *testing.T, files map[string][]byte) *JarSourceArtifact {
	t.Helper()
	path := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(path, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarSourceArtifactFromLocal(path, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestAdversarialAssertionStorageRequiresPublishedOwnedFieldIdentity(t *testing.T) {
	files, compiled, issuer := assertionStorageCompiledOwner(t)
	_ = issuer
	for _, change := range []string{"original", "unpublished", "wrong owner", "stale original", "wrong source", "duplicate original declaration", "missing alias", "wrong descriptor", "wrong flags", "extra field", "foreign alias reference", "wrong reference descriptor", "canceled", "graph budget", "output budget", "constant pool boundary", "constant pool full"} {
		t.Run(change, func(t *testing.T) {
			a := assertionStorageArtifact(t, files)
			if change != "unpublished" {
				if _, err := a.ReadFile("FieldBindingOwner.class"); err != nil {
					t.Fatal(err)
				}
			}
			plan := issuer.jar.sourceFieldMetadata.aliasActive["FieldBindingOwner"]
			if plan == nil {
				t.Fatal("source identity absent")
			}
			candidate, err := Parse(compiled)
			if err != nil {
				t.Fatal(err)
			}
			var field *MemberInfo
			for _, f := range candidate.Fields {
				n, _ := sourceBridgeUTF8(candidate, f.NameIndex)
				if n == plan.alias {
					field = f
				}
			}
			if field == nil {
				t.Fatal("compiler source identity absent")
			}
			path := "FieldBindingOwner.class"
			original := files[path]
			var work *workbudget.Budget
			switch change {
			case "wrong owner":
				path = "FieldBindingBase.class"
			case "stale original":
				object, _ := Parse(original)
				object.MinorVersion++
				original = object.Bytes()
			case "wrong source":
				if err := a.jar.sourceFieldMetadata.assertionSourceReturned("FieldBindingOwner", []byte("unpublished replacement")); err == nil {
					t.Fatal("changed source retained permission")
				}
				return
			case "duplicate original declaration":
				copy := *field
				copy.NameIndex = uint16(candidate.ConstantPoolManager.AddUtf8Info(nativeAssertionField))
				candidate.Fields = append(candidate.Fields, &copy)
			case "missing alias":
				field.NameIndex = uint16(candidate.ConstantPoolManager.AddUtf8Info("unrelated"))
			case "wrong descriptor":
				field.DescriptorIndex = uint16(candidate.ConstantPoolManager.AddUtf8Info("I"))
			case "wrong flags":
				field.AccessFlags |= 2
			case "extra field":
				copy := *field
				copy.NameIndex = uint16(candidate.ConstantPoolManager.AddUtf8Info("extra"))
				candidate.Fields = append(candidate.Fields, &copy)
			case "foreign alias reference", "wrong reference descriptor":
				changed := false
				for _, c := range candidate.ConstantPool {
					ref, ok := c.(*ConstantFieldrefInfo)
					if !ok || ref == nil {
						continue
					}
					nt, _ := candidate.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					n, _ := sourceBridgeUTF8(candidate, nt.NameIndex)
					if n != plan.alias {
						continue
					}
					if change == "foreign alias reference" {
						ref.ClassIndex = uint16(candidate.ConstantPoolManager.AddNewClassInfo("FieldBindingScope"))
					} else {
						copy := *nt
						copy.DescriptorIndex = uint16(candidate.ConstantPoolManager.AddUtf8Info("I"))
						ref.NameAndTypeIndex = uint16(candidate.ConstantPoolManager.AppendConstantInfo(&copy))
					}
					changed = true
					break
				}
				if !changed {
					t.Fatal("no actual field reference mutated")
				}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "graph budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "constant pool boundary", "constant pool full":
				limit := 65532
				if change == "constant pool full" {
					limit = 65534
				}
				for len(candidate.ConstantPool) < limit {
					candidate.ConstantPoolManager.AppendConstantInfo(NewUtf8FromString("unused"))
				}
			}
			input := candidate.Bytes()
			hash := sha256.Sum256(input)
			var got []byte
			if change == "stale original" || work != nil {
				got, err = a.jar.sourceFieldMetadata.restoreAssertionStorage(original, input, work)
			} else {
				got, err = a.RestoreClassMetadata(path, input)
			}
			want := change == "original" || change == "constant pool boundary"
			if (err == nil) != want || !want && got != nil {
				t.Fatalf("field identity accepted=%v want=%v result=%d err=%v", err == nil, want, len(got), err)
			}
			if sha256.Sum256(input) != hash {
				t.Fatal("restorer mutated compiler input")
			}
			if change == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation classification lost", err)
			}
			if work != nil && work.Err() == nil {
				t.Fatal("resource refusal did not retain budget error", err)
			}
			if want {
				again, err := a.RestoreClassMetadata(path, got)
				if err != nil || !bytes.Equal(got, again) {
					t.Fatal("field metadata restoration is not idempotent", err)
				}
			}
		})
	}
}

func TestAdversarialAssertionStorageRefusesForeignOrInheritedFieldUses(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, change := range []string{"closed", "foreign reference", "inherited reference", "unknown reference", "ordinary field", "mutable flag"} {
		t.Run(change, func(t *testing.T) {
			fixture := assertionStorageFixture + "\nclass PassiveStorageChild extends FieldBindingOwner{}"
			files := assertionStorageOriginalFiles(t, fixture, "FieldBinding", javac, ModernJavac, "none", false)
			if change == "ordinary field" || change == "mutable flag" {
				object, _ := Parse(files["FieldBindingOwner.class"])
				for _, field := range object.Fields {
					n, _ := sourceBridgeUTF8(object, field.NameIndex)
					if n == nativeAssertionField {
						if change == "ordinary field" {
							field.AccessFlags &^= syntheticFieldFlag
						} else {
							field.AccessFlags &^= 0x10
						}
					}
				}
				files["FieldBindingOwner.class"] = object.Bytes()
			}
			if change == "foreign reference" || change == "inherited reference" || change == "unknown reference" {
				object, _ := Parse(files["FieldBindingDriver.class"])
				owner := "FieldBindingOwner"
				if change == "inherited reference" {
					owner = "PassiveStorageChild"
				}
				if change == "unknown reference" {
					owner = "AbsentStorageClass"
				}
				class := uint16(object.ConstantPoolManager.AddNewClassInfo(owner))
				name := uint16(object.ConstantPoolManager.AddUtf8Info(nativeAssertionField))
				desc := uint16(object.ConstantPoolManager.AddUtf8Info("Z"))
				nt := uint16(object.ConstantPoolManager.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc}))
				object.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: class, NameAndTypeIndex: nt}})
				files["FieldBindingDriver.class"] = object.Bytes()
			}
			a := assertionStorageArtifact(t, files)
			source, err := a.ReadFile("FieldBindingOwner.class")
			if err != nil {
				t.Fatal(err)
			}
			active := a.jar.sourceFieldMetadata.aliasActive["FieldBindingOwner"]
			if (active != nil) != (change == "closed") {
				t.Fatalf("storage alias granted=%v for %s\n%s", active != nil, change, source)
			}
		})
	}
}

func assertionStorageCompiledOwner(t *testing.T) (map[string][]byte, []byte, *JarSourceArtifact) {
	t.Helper()
	javac, _ := t04Tools(t)
	files := assertionStorageOriginalFiles(t, assertionStorageFixture, "FieldBinding", javac, ModernJavac, "none", false)
	issuer := assertionStorageArtifact(t, files)
	dependencies := map[string][]byte{}
	for name, raw := range files {
		if name != "FieldBindingOwner.class" && name != "FieldBindingBase.class" {
			dependencies[name] = raw
		}
	}
	out := sourceArtifactWriteFiles(t, dependencies)
	paths := []string{}
	for _, owner := range []string{"FieldBindingBase", "FieldBindingOwner"} {
		source, err := issuer.ReadFile(owner + ".class")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(out, owner+".java")
		if err := os.WriteFile(path, source, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	args := []string{"-proc:none", "--release", "8", "-g:none", "-cp", out, "-sourcepath", out, "-d", out}
	if log, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
		t.Fatal(err, string(log))
	}
	compiled, err := os.ReadFile(filepath.Join(out, "FieldBindingOwner.class"))
	if err != nil {
		t.Fatal(err)
	}
	return files, compiled, issuer
}

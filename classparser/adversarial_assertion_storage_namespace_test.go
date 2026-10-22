package javaclassparser

import (
	"bytes"
	"strings"
	"testing"
)

func TestAdversarialAssertionStorageRebindsOnlyIdenticalPublishedBaseSources(t *testing.T) {
	files, compiled, _ := assertionStorageCompiledOwner(t)
	javac, _ := t04Tools(t)
	for _, change := range []string{"identical original", "unpublished base", "different original", "versioned foreign use", "different compiler", "different source level"} {
		t.Run(change, func(t *testing.T) {
			entries := map[string][]byte{}
			for name, raw := range files {
				entries[name] = raw
			}
			entries["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0\r\nMulti-Release: true\r\n\r\n")
			if change == "different original" {
				fixture := strings.Replace(assertionStorageFixture, "n^0x5a5a5a5a", "n^0x33333333", 1)
				variant := assertionStorageOriginalFiles(t, fixture, "FieldBinding", javac, ModernJavac, "none", false)
				entries["META-INF/versions/9/FieldBindingOwner.class"] = variant["FieldBindingOwner.class"]
			}
			if change == "versioned foreign use" {
				driver, _ := Parse(files["FieldBindingDriver.class"])
				class := uint16(driver.ConstantPoolManager.AddNewClassInfo("FieldBindingOwner"))
				name := uint16(driver.ConstantPoolManager.AddUtf8Info(nativeAssertionField))
				desc := uint16(driver.ConstantPoolManager.AddUtf8Info("Z"))
				nt := uint16(driver.ConstantPoolManager.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc}))
				driver.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: class, NameAndTypeIndex: nt}})
				entries["META-INF/versions/9/FieldBindingDriver.class"] = driver.Bytes()
			}
			a := assertionStorageArtifact(t, entries)
			if change != "unpublished base" {
				if _, err := a.ReadFile("FieldBindingOwner.class"); err != nil {
					t.Fatal(err)
				}
			}
			view := a.jar.sourceReleaseView(9)
			if view == nil {
				t.Fatal("physical source namespace absent")
			}
			if change == "different compiler" {
				view.sourceCompiler = NativeJavac8
			}
			if change == "different source level" {
				view.targetSourceVersion = 9
			}
			before := append([]byte(nil), compiled...)
			got, err := a.RestoreClassMetadata("META-INF/versions/9/FieldBindingOwner.class", compiled)
			want := change == "identical original"
			if (err == nil) != want || !want && got != nil {
				t.Fatalf("namespace source identity accepted=%v want=%v err=%v", err == nil, want, err)
			}
			if !bytes.Equal(before, compiled) {
				t.Fatal("namespace restoration mutated compiler input")
			}
			if want {
				base, err := a.RestoreClassMetadata("FieldBindingOwner.class", compiled)
				if err != nil || !bytes.Equal(got, base) {
					t.Fatal("byte-identical source authority generated a different field binding", err)
				}
				if view.sourceFieldMetadata.aliasActive["FieldBindingOwner"] != nil {
					t.Fatal("implicit compilation fabricated a new source publication")
				}
			}
		})
	}
}

func TestAdversarialAssertionStorageKeepsMethodLocalDeclarationProtocols(t *testing.T) {
	javac, _ := t04Tools(t)
	fixture := `public class LocalStorageOwner{static Runnable make(){return new Runnable(){public void run(){assert Boolean.getBoolean("local.status"):"local";}};} static Object named(){class Local{void check(){assert Boolean.getBoolean("named.status"):"named";}}return new Local();}}`
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, javac, ModernJavac, fixture, "LocalStorageOwner", debug, false)
			with := assertionStorageArtifact(t, files)
			without := assertionStorageArtifact(t, files)
			without.jar.sourceFieldMetadata.assertionIndex.once.Do(func() {
				without.jar.sourceFieldMetadata.assertionIndex.valid = true
				without.jar.sourceFieldMetadata.assertionIndex.aliases = map[string]string{}
			})
			for name := range files {
				if !strings.HasPrefix(name, "LocalStorageOwner$") {
					continue
				}
				actual, err := with.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				control, err := without.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(actual, control) {
					t.Fatalf("named storage capability changed a method-local source declaration %s", name)
				}
				if with.jar.sourceFieldMetadata.aliasActive[strings.TrimSuffix(name, ".class")] != nil {
					t.Fatal("method-local declaration borrowed a flat named-field capability")
				}
			}
		})
	}
}

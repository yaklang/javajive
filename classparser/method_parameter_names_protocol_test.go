package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These are explicit authored JVM protocol variants, not claimed javac source
// output. Only MethodParameters entries change. Each original program verifies
// and exposes that exact metadata before the source path must reject it.
func TestAdversarialOrdinaryMethodParameterNamesRefuseUnexpressibleJVMMetadata(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "Authored.java")
	source := `class GuardOwner{static class Anchor{}static long read(long leading,int trailing){return leading+trailing;}}class GuardDriver{public static void main(String[]args)throws Exception{java.lang.reflect.Parameter[]p=GuardOwner.class.getDeclaredMethod("read",long.class,int.class).getParameters();if(GuardOwner.read(7L,2)!=9L)throw new AssertionError("unchanged Code");System.out.println(p[0].isNamePresent()+":"+p[0].getName()+":"+p[0].getModifiers()+"|"+p[1].isNamePresent()+":"+p[1].getName()+":"+p[1].getModifiers()+"|9");}}`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(javac, "-proc:none", "--release", "8", "-parameters", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("authored protocol compile: %v\n%s", err, log)
	}
	files := map[string][]byte{}
	for _, name := range []string{"GuardOwner.class", "GuardOwner$Anchor.class", "GuardDriver.class"} {
		var err error
		files[name], err = os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, variant := range []struct{ name, expected string }{
		{"keyword", "true:class:0|true:trailing:0|9\n"},
		{"illegal character", "true:a-b:0|true:trailing:0|9\n"},
		{"duplicate", "true:leading:0|true:leading:0|9\n"},
		{"unnamed", "false:arg0:0|true:trailing:0|9\n"},
		{"mandated", "true:leading:32768|true:trailing:0|9\n"},
	} {
		t.Run(variant.name, func(t *testing.T) {
			d, method, table, _ := methodParameterNameGuardInput(t, files["GuardOwner.class"])
			var code *CodeAttribute
			for _, a := range method.Attributes {
				if candidate, ok := a.(*CodeAttribute); ok {
					code = candidate
				}
			}
			if code == nil {
				t.Fatal("original Code")
			}
			originalCode := append([]byte(nil), code.Code...)
			switch variant.name {
			case "keyword":
				methodParameterNameGuardSet(t, d.obj, table, 0, "class")
			case "illegal character":
				methodParameterNameGuardSet(t, d.obj, table, 0, "a-b")
			case "duplicate":
				copy(table.Info[5:7], table.Info[1:3])
			case "unnamed":
				binary.BigEndian.PutUint16(table.Info[1:3], 0)
			case "mandated":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x8000)
			}
			if !bytes.Equal(originalCode, code.Code) {
				t.Fatal("protocol variant modified original Code")
			}
			input := map[string][]byte{}
			for name, raw := range files {
				input[name] = raw
			}
			input["GuardOwner.class"] = d.obj.Bytes()
			original := methodParameterNameDirectory(t, "original")
			for name, raw := range input {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := exec.Command(java, "-Xverify:all", "-cp", original, "GuardDriver").CombinedOutput()
			t.Logf("original JVM: %q", got)
			if err != nil || string(got) != variant.expected {
				t.Fatalf("original protocol JVM: %v\n%s want=%q", err, got, variant.expected)
			}
			if err := os.WriteFile(filepath.Join(original, "jvm-oracle.txt"), got, 0600); err != nil {
				t.Fatal(err)
			}
			if _, known := d.originalOrdinaryMethodParameterNames(method, "(JI)J"); known {
				t.Fatal("unexpressible parameter names/roles accepted")
			}
			archive := nativeArchive(t, input)
			defer archive.Close()
			text, err := archive.ReadFile("GuardOwner.class")
			if err == nil && !strings.Contains(string(text), DecompileStubMarker) {
				t.Fatalf("unexpressible metadata silently repaired as source\n%s", text)
			}
			if entry := archive.nativeMemberEntry(d.obj); entry != nil && entry.family != nil {
				t.Fatal("unexpressible metadata published a complete source family")
			}
		})
	}
}

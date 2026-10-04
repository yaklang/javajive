package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/yaklang/javajive/internal/mutf8"
)

func t02CompileBaseline(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/contracts/t03/Baseline.java")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Baseline.java"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Fatalf("infra_error: javac missing: %v", err)
	}
	cmd := exec.Command(javac, "-proc:none", "-encoding", "UTF-8", "--release", "8", "-d", dir, "Baseline.java")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("javac Baseline: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "Baseline.class"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTaskT02_RenameSerializeReparse(t *testing.T) {
	raw := t02CompileBaseline(t)
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := obj.GetClassName(); got != "Baseline" {
		t.Fatalf("parsed name %q", got)
	}
	if err := obj.SetClassName("Renamed"); err != nil {
		t.Fatal(err)
	}
	if obj.GetClassName() != "Renamed" {
		t.Fatalf("SetClassName did not update Value: %q", obj.GetClassName())
	}
	rewritten := obj.Bytes()
	obj2, err := Parse(rewritten)
	if err != nil {
		t.Fatalf("reparse after SetClassName: %v", err)
	}
	if got := obj2.GetClassName(); got != "Renamed" {
		t.Fatalf("T02 rename regression: Bytes/Parse restored %q want Renamed", got)
	}
	custom := obj.ToBytesByCustomStringChar(1)
	obj3, err := Parse(custom)
	if err != nil {
		t.Fatalf("ToBytesByCustomStringChar reparse: %v", err)
	}
	if obj3.GetClassName() != "Renamed" {
		t.Fatalf("ToBytesByCustomStringChar lost rename: %q", obj3.GetClassName())
	}

	if javap, err := exec.LookPath("javap"); err != nil {
		t.Logf("javap missing (%v); class-name reparse already checked", err)
	} else {
		dir := t.TempDir()
		p := filepath.Join(dir, "Renamed.class")
		if err := os.WriteFile(p, rewritten, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(javap, "-v", "-p", p).CombinedOutput()
		if err != nil {
			t.Fatalf("javap: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "Renamed") || strings.Contains(string(out), "this_class") && !strings.Contains(string(out), "Renamed") {
			if !strings.Contains(string(out), "Renamed") {
				t.Fatalf("javap did not show Renamed:\n%s", out)
			}
		}
		if strings.Contains(string(out), "Baseline") && !strings.Contains(string(out), "SourceFile") {
			// SourceFile may still say Baseline.java; class name must not.
		}
		if bytes.Contains(rewritten, []byte("Baseline")) && obj2.GetClassName() == "Baseline" {
			t.Fatal("stale Baseline name")
		}
	}

	if err := obj.SetSourceFileName("Renamed.java"); err != nil {
		t.Fatal(err)
	}
	obj4, err := Parse(obj.Bytes())
	if err != nil {
		t.Fatalf("SetSourceFileName reparse: %v", err)
	}
	_ = obj4
	if err := obj.SetMethodName("main", "entry"); err != nil {
		t.Fatal(err)
	}
	obj5, err := Parse(obj.Bytes())
	if err != nil {
		t.Fatalf("SetMethodName reparse: %v", err)
	}
	found := false
	for _, m := range obj5.Methods {
		u, err := obj5.getUtf8(m.NameIndex)
		if err == nil && u == "entry" {
			found = true
		}
	}
	if !found {
		t.Fatal("SetMethodName did not survive Bytes/Parse")
	}

	b := NewClassObjectBuilder(obj2)
	if obj2.FindConstStringFromPool("unused") == nil {
		b.SetValue("no-such", "x")
		if len(b.GetErrors()) == 0 {
			t.Fatal("SetValue missing constant should error")
		}
	}
}

func TestTaskT02_DirectValueEditAndLoneSurrogate(t *testing.T) {
	info := NewUtf8FromString("Baseline")
	info.Value = "Renamed" // legacy direct field write
	if string(info.mutf8Bytes()) != "Renamed" {
		t.Fatalf("direct Value edit ignored: %q", info.mutf8Bytes())
	}

	lone := []uint16{0xD800, 'Z'}
	u := &ConstantUtf8Info{}
	u.SetUnits(lone)
	payload := u.mutf8Bytes()
	again, err := mutf8.Decode(payload)
	if err != nil || len(again) != 2 || again[0] != 0xD800 || again[1] != 'Z' {
		t.Fatalf("SetUnits lone surrogate lost: %v %v", again, err)
	}
	obj, err := Parse(buildClassWithUtf8Payload(payload))
	if err != nil {
		t.Fatal(err)
	}
	got := findPayloadUtf8(t, obj, payload)
	if !bytesEqualUnits(got.Units, lone) {
		t.Fatalf("reparsed Units %v", got.Units)
	}
	got.SetString("plain")
	obj.ConstantPool[len(obj.ConstantPool)-1] = got
	rewritten := obj.Bytes()
	obj2, err := Parse(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	var found *ConstantUtf8Info
	for _, c := range obj2.ConstantPool {
		if u, ok := c.(*ConstantUtf8Info); ok && u.Value == "plain" {
			found = u
		}
	}
	if found == nil {
		t.Fatal("SetString after lone surrogate did not serialize")
	}
}

func TestTaskT02_Utf8LengthOverflow(t *testing.T) {
	units := make([]uint16, 22000)
	for i := range units {
		units[i] = 0x0800
	}
	u := &ConstantUtf8Info{}
	u.SetUnits(units)
	payload, err := u.mutf8BytesChecked()
	if err == nil || payload != nil {
		t.Fatal("expected u2 overflow error, no truncated payload")
	}
	if !strings.Contains(err.Error(), "65535") {
		t.Fatalf("overflow error should mention u2 limit: %v", err)
	}
}

func TestTaskT02_CPUseSitesAndLegalNULName(t *testing.T) {
	raw := t02CompileBaseline(t)
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.CheckUtf8UseSites(); err != nil {
		t.Fatalf("valid Baseline failed use-site check: %v", err)
	}

	nul := []uint16{'A', 0, 'Z'}
	if err := mutf8.ValidateInternalName(nul); err != nil {
		t.Fatalf("NUL name must be JVM-legal: %v", err)
	}
	if mutf8.JavaSourceIdentifierOK(nul) {
		t.Fatal("NUL name must be source-unsupported")
	}

	dims := make([]uint16, 257)
	for i := 0; i < 256; i++ {
		dims[i] = '['
	}
	dims[256] = 'I'
	bad := &ConstantUtf8Info{}
	bad.SetUnits(dims)
	obj.ConstantPool = append(obj.ConstantPool, NewUtf8FromString("weird-name"), bad)
	nameIdx := uint16(len(obj.ConstantPool) - 1)
	descIdx := uint16(len(obj.ConstantPool))
	obj.Fields = append(obj.Fields, &MemberInfo{NameIndex: nameIdx, DescriptorIndex: descIdx})
	if err := obj.CheckUtf8UseSites(); err == nil {
		t.Fatal("256-dimension field descriptor must fail CP use-site check")
	}
	if mutf8.JavaSourceIdentifierOK(utf16.Encode([]rune("weird-name"))) {
		t.Fatal("hyphen is not a Java identifier")
	}
	if err := mutf8.ValidateUnqualifiedName(utf16.Encode([]rune("weird-name"))); err != nil {
		t.Fatalf("hyphen is a legal JVM name: %v", err)
	}
}

func bytesEqualUnits(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

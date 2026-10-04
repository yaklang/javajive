package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskT17RecordLayoutHookBeforeMembers(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "t17", "RecordProbe.java"))
	if err != nil {
		t.Fatal(err)
	}
	_, classes := t17CompileRun(t, "17", "RecordProbe", map[string]string{"RecordProbe.java": string(src)})
	raw := classes["RecordProbe"]
	if raw == nil {
		t.Fatal("RecordProbe.class missing")
	}

	old := TryRecordLayout
	oldComplete := RecordReconstructionComplete
	defer func() {
		TryRecordLayout = old
		RecordReconstructionComplete = oldComplete
	}()
	var sawDumper bool
	TryRecordLayout = func(c *ClassObjectDumper) *RecordLayout {
		sawDumper = c != nil && c.obj != nil
		return &RecordLayout{
			Keyword:           "record",
			Components:        "(int x, String y)",
			DropExtendsRecord: true,
			SkipFieldNames:    map[string]bool{"x": true, "y": true},
			SkipMethodKeys:    map[string]bool{"x": true, "y": true},
		}
	}
	RecordReconstructionComplete = func(*ClassObject) bool { return true }

	res, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 17})
	if err != nil && res.Source == "" {
		t.Fatalf("hook decompile: %v", err)
	}
	if !sawDumper {
		t.Fatal("TryRecordLayout was not invoked before dump")
	}
	if !strings.Contains(res.Source, "record RecordProbe") && !strings.Contains(res.Source, " record ") {
		t.Fatalf("expected record keyword from hook, got:\n%s", res.Source)
	}
	if strings.Contains(res.Source, "extends Record") {
		t.Fatalf("DropExtendsRecord ignored:\n%s", res.Source)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	artifactinspect "github.com/yaklang/javajive/inspect"
)

func TestCmdInspectAcceptedFixtureJSON(t *testing.T) {
	fixture := filepath.Join("..", "..", "dex2jar", "testdata", "accept", "arithmetic.dex")
	var stdout bytes.Buffer
	if err := cmdInspectTo([]string{fixture, "--format", "json"}, &stdout); err != nil {
		t.Fatalf("cmdInspectTo: %v", err)
	}
	var report artifactinspect.Report
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&report); err != nil {
		t.Fatalf("decode stdout JSON: %v\n%s", err, stdout.String())
	}
	if decoder.More() {
		t.Fatal("stdout contains more than one JSON value")
	}
	if report.Status != artifactinspect.StatusComplete || report.Artifact.Format != "dex" || len(report.DEXFiles) != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestCmdInspectExternalRejectedFixtureStillEmitsJSON(t *testing.T) {
	// DEX 038 with call-site sections is still rejected, but the report must
	// remain machine-readable on stdout.
	fixture := filepath.Join("..", "..", "dex2jar", "testdata", "external", "pxb1988-dex2jar-dex038.dex")
	var stdout bytes.Buffer
	if err := cmdInspectTo([]string{"--format=json", fixture}, &stdout); err != nil {
		t.Fatalf("cmdInspectTo: %v", err)
	}
	var report artifactinspect.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode stdout JSON: %v", err)
	}
	if report.Status != artifactinspect.StatusPartial || len(report.Diagnostics) == 0 {
		t.Fatalf("report = %#v", report)
	}
	for _, diagnostic := range report.Diagnostics {
		if !strings.Contains(diagnostic.Message, "call_site_id") {
			t.Fatalf("unexpected diagnostic = %#v", diagnostic)
		}
	}
}

func TestCmdInspectArguments(t *testing.T) {
	if err := cmdInspectTo(nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires exactly one") {
		t.Fatalf("nil arguments error = %v", err)
	}
	if err := cmdInspectTo([]string{"--format", "yaml", "input.dex"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "unsupported inspect format") {
		t.Fatalf("unsupported format error = %v", err)
	}
	if err := cmdInspectTo([]string{"-h"}, &bytes.Buffer{}); err != flag.ErrHelp {
		t.Fatalf("help error = %v, want flag.ErrHelp", err)
	}
}

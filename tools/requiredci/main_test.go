package main

import (
	"strings"
	"testing"
)

const validWorkflow = `name: CI
on:
  push:
  pull_request:
jobs:
  contract:
    steps:
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff
      - run: go run ./tools/requiredci -check
      - run: CGO_ENABLED=0 go build ./...
      - run: go run ./tools/requiredci -run contract
  corpus:
    steps:
      - run: go run ./tools/requiredci -run corpus
  algorithm:
    steps:
      - run: go run ./tools/requiredci -run algorithm
  dex:
    steps:
      - run: go run ./tools/requiredci -run dex
`

func TestWorkflowContractAcceptsReviewedShape(t *testing.T) {
	if err := validateWorkflow(validWorkflow); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowContractRejectsForbiddenToolsAndActions(t *testing.T) {
	for _, replacement := range []string{
		validWorkflow + "      - run: python3 check.py\n",
		validWorkflow + "      - run: javac Fixture.java\n",
		strings.Replace(validWorkflow, "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff", "actions/setup-java@cf277c60eb25467037889841efdb72551f06f6c3", 1),
		validWorkflow + "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02\n",
	} {
		if err := validateWorkflow(replacement); err == nil {
			t.Fatalf("accepted forbidden workflow:\n%s", replacement)
		}
	}
}

func TestWorkflowContractRejectsBroadOrMissingInventory(t *testing.T) {
	for _, broken := range []string{
		validWorkflow + "      - run: go test ./...\n",
		strings.Replace(validWorkflow, "go run ./tools/requiredci -run dex", "go test ./dex2jar/...", 1),
		strings.Replace(validWorkflow, "pull_request:", "workflow_dispatch:", 1),
	} {
		if err := validateWorkflow(broken); err == nil {
			t.Fatalf("accepted workflow outside inventory contract:\n%s", broken)
		}
	}
}

func TestSelectedTestsRejectsUnknownNames(t *testing.T) {
	all := []string{"TestA", "TestB"}
	if _, err := selectedTests(suite{Package: "./p", Include: []string{"TestMissing"}}, all); err == nil {
		t.Fatal("accepted missing include")
	}
	if _, err := selectedTests(suite{Package: "./p", Exclude: []string{"TestMissing"}}, all); err == nil {
		t.Fatal("accepted missing exclusion")
	}
}

func TestVerifyEventsFailsClosed(t *testing.T) {
	good := []byte("{\"Action\":\"pass\",\"Test\":\"TestA/sub\"}\n{\"Action\":\"pass\",\"Test\":\"TestA\"}\n")
	if err := verifyEvents("./p", []string{"TestA"}, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		[]byte("{\"Action\":\"skip\",\"Test\":\"TestA/sub\"}\n{\"Action\":\"pass\",\"Test\":\"TestA\"}\n"),
		[]byte("{\"Action\":\"pass\",\"Test\":\"TestOther\"}\n"),
		[]byte("{\"Action\":\"pass\",\"Test\":\"TestA\"}\n{\"Action\":\"pass\",\"Test\":\"TestA\"}\n"),
	} {
		if err := verifyEvents("./p", []string{"TestA"}, bad); err == nil {
			t.Fatalf("accepted incomplete events: %s", bad)
		}
	}
}

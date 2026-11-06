package dex2jar

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

// TestApplicationToJarRejectsInconsistentFileMetadata covers the per-file
// validation the conversion driver performs before lowering any class.
func TestApplicationToJarRejectsInconsistentFileMetadata(t *testing.T) {
	file := &model.File{
		Entry: "classes.dex",
		Header: model.Header{
			StringIDsSize: 1,
			TypeIDsSize:   1,
		},
		Strings: []string{"Lx;"},
		Types:   []model.Type{{Descriptor: "Ly;", DescriptorIndex: 0}},
	}
	app := &model.Application{Files: []*model.File{file}}
	if _, _, err := applicationToJar(app); err == nil || !strings.Contains(err.Error(), "does not match string index") {
		t.Fatalf("applicationToJar() error = %v", err)
	}
}

// TestApplicationToJarValidatesFileOnce covers the success path that skips the
// per-method file validation.
func TestApplicationToJarValidatesFileOnce(t *testing.T) {
	file := &model.File{Entry: "classes.dex"}
	app := &model.Application{Files: []*model.File{file}}
	raw, _, err := applicationToJar(app)
	if err != nil {
		t.Fatalf("applicationToJar() error = %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("applicationToJar() produced an empty archive")
	}
}

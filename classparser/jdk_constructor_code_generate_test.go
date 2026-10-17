package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Manual data generation, not a CI download/install step. The input describes
// the same archive hashes already pinned by jdk_metadata_catalog.json. For
// example: JDEC_CONSTRUCTOR_CATALOG_INPUT=/absolute/input.json go test
// ./classparser -run '^TestGenerateJDKConstructorCodeArchive$'. Runtime replays
// the receiver proof against these original bytes; this is not a silence flag.
func TestGenerateJDKConstructorCodeArchive(t *testing.T) {
	path := os.Getenv("JDEC_CONSTRUCTOR_CATALOG_INPUT")
	if path == "" {
		t.Skip("explicit original archive generation input required")
	}
	var input struct {
		Out           string
		OutReferences string
		Profiles      []struct {
			Release       int
			Path, Prefix  string
			ArchiveSHA256 string `json:"archive_sha256"`
			Notices       map[string]string
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	var declared struct {
		Profiles []struct {
			Release       int
			ArchiveSHA256 string `json:"archive_sha256"`
			Classes       map[string]json.RawMessage
		}
	}
	if err = json.Unmarshal(jdkInvocationCatalogJSON, &declared); err != nil {
		t.Fatal(err)
	}
	expected := map[int]string{}
	declarationRoots := map[int]map[string]json.RawMessage{}
	for _, p := range declared.Profiles {
		expected[p.Release] = p.ArchiveSHA256
		declarationRoots[p.Release] = p.Classes
	}
	if len(input.Profiles) != len(expected) {
		t.Fatal("generation must cover each exact declaration profile")
	}
	seenProfiles := map[int]bool{}
	document := constructorCodeDocument{Schema: 1}
	originalReferences := map[int]map[string]any{}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, profile := range input.Profiles {
		if seenProfiles[profile.Release] || expected[profile.Release] == "" {
			t.Fatal("unknown/duplicate original profile", profile.Release)
		}
		seenProfiles[profile.Release] = true
		data, err := os.ReadFile(profile.Path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != expected[profile.Release] || profile.ArchiveSHA256 != expected[profile.Release] {
			t.Fatal("original archive identity mismatch", profile.Release)
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		// Preserve the original distribution's license/exception notices along
		// with these unchanged classfiles. For jmods they are inside the archive;
		// rt.jar distributions provide them adjacent to the runtime archive.
		notices := map[string][]byte{}
		for _, entry := range zr.File {
			if !strings.HasPrefix(entry.Name, "legal/") || entry.FileInfo().IsDir() {
				continue
			}
			f, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			notices[strings.TrimPrefix(entry.Name, "legal/")] = data
		}
		for name, path := range profile.Notices {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			notices[name] = data
		}
		if len(notices["LICENSE"]) == 0 || len(notices["ASSEMBLY_EXCEPTION"]) == 0 {
			t.Fatal("missing original platform license notices", profile.Release)
		}
		noticeNames := []string{}
		for name := range notices {
			noticeNames = append(noticeNames, name)
		}
		sort.Strings(noticeNames)
		for _, name := range noticeNames {
			entry, err := writer.Create(fmt.Sprintf("%d/legal/%s", profile.Release, name))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(notices[name]); err != nil {
				t.Fatal(err)
			}
		}
		originals := map[string][]byte{}
		objects := map[string]*ClassObject{}
		for _, entry := range zr.File {
			if !strings.HasPrefix(entry.Name, profile.Prefix) || !strings.HasSuffix(entry.Name, ".class") {
				continue
			}
			f, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if int(obj.MajorVersion) != profile.Release+44 {
				continue
			}
			name := obj.GetClassName()
			if _, duplicate := originals[name]; duplicate {
				t.Fatal("duplicate class identity", name)
			}
			originals[name] = raw
			objects[name] = obj
		}
		selected := map[string]bool{}
		names := make([]string, 0, len(objects))
		for name := range objects {
			names = append(names, name)
		}
		sort.Strings(names)
		selected = nativePlatformDeclarationIndexClasses(objects, declarationRoots[profile.Release])
		originalReferences[profile.Release] = map[string]any{}
		p := constructorCodeProfile{Release: profile.Release, ArchiveSHA256: expected[profile.Release], Classes: map[string]string{}}
		for _, name := range names {
			if !selected[name] {
				continue
			}
			raw := originals[name]
			hash := sha256.Sum256(raw)
			p.Classes[name] = hex.EncodeToString(hash[:])
			originalReferences[profile.Release][name] = map[string]any{"archive": filepath.Base(profile.Path), "entry": profile.Prefix + name + ".class", "sha256": p.Classes[name], "major": objects[name].MajorVersion, "parents": append([]string{objects[name].GetSupperClassName()}, objects[name].GetInterfacesName()...)}
			entry, err := writer.Create(fmt.Sprintf("%d/%s.class", profile.Release, name))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(raw); err != nil {
				t.Fatal(err)
			}
		}
		document.Profiles = append(document.Profiles, p)
		t.Logf("release %d: %d original classfiles retained from %d declarations", profile.Release, len(p.Classes), len(objects))
	}
	metadata, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := writer.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write(metadata); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(input.Out, out.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if input.OutReferences != "" {
		data, err := json.Marshal(originalReferences)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(input.OutReferences, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("original constructor evidence archive: %d bytes", out.Len())
}

// Source metadata coverage is separate from constructor observation. All
// platform extension points and complete invocation-catalog declarations need
// original flags and members, including APIs with observing constructors.
// An unavailable primary-archive ancestor cannot be guessed.
func nativePlatformDeclarationIndexClasses(objects map[string]*ClassObject, catalogDeclarations map[string]json.RawMessage) map[string]bool {
	selected := map[string]bool{}
	ancestry := map[string]uint8{}
	var closed func(string) bool
	closed = func(name string) bool {
		if state := ancestry[name]; state != 0 {
			return state == 2
		}
		obj := objects[name]
		if obj == nil || obj.GetClassName() != name {
			return false
		}
		ancestry[name] = 1
		for _, parent := range append([]string{obj.GetSupperClassName()}, obj.GetInterfacesName()...) {
			if parent != "" && !closed(parent) {
				ancestry[name] = 3
				return false
			}
		}
		ancestry[name] = 2
		return true
	}
	for name := range objects {
		obj := objects[name]
		if obj == nil {
			continue
		}
		_, catalog := catalogDeclarations[name]
		platformAPI := (strings.HasPrefix(name, "java/") || strings.HasPrefix(name, "javax/")) && obj.AccessFlags&1 != 0 && obj.AccessFlags&0x0010 == 0
		if (catalog || platformAPI) && closed(name) {
			selected[name] = true
		}
	}

	// Every selected root has a closed original ancestry. Retain those exact
	// declarations as metadata as well, including nonpublic superclass and
	// interface definitions; their presence grants no source accessibility.
	pending := make([]string, 0, len(selected))
	for name := range selected {
		pending = append(pending, name)
	}
	for len(pending) != 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		obj := objects[name]
		dependencies := append([]string{obj.GetSupperClassName()}, obj.GetInterfacesName()...)
		dependencies = append(dependencies, nativePlatformCopyFactoryClasses(obj)...)
		for _, parent := range dependencies {
			if parent != "" && !selected[parent] && closed(parent) {
				selected[parent] = true
				pending = append(pending, parent)
			}
		}
	}
	return selected
}

// A private allocation returned by an original static factory needs original
// constructor bytes too. This is offline indexing, not an effect certificate:
// callbacks/publication remain independently rejected by the runtime proof.
func nativePlatformCopyFactoryClasses(object *ClassObject) []string {
	var result []string
	for _, method := range object.Methods {
		if method == nil || method.AccessFlags&0x0008 == 0 || method.AccessFlags&(0x0100|0x0400) != 0 {
			continue
		}
		var code *CodeAttribute
		count := 0
		for _, attribute := range method.Attributes {
			if body, ok := attribute.(*CodeAttribute); ok {
				code = body
				count++
			}
		}
		if count != 1 || code == nil {
			continue
		}
		decoder := core.NewDecompiler(code.Code, nil)
		if decoder.ParseOpcode() != nil {
			continue
		}
		ops := constructorMotionOps(decoder)
		if len(ops) < 4 || ops[0].Instr.OpCode != core.OP_NEW || ops[1].Instr.OpCode != core.OP_DUP || ops[len(ops)-1].Instr.OpCode != core.OP_ARETURN || len(ops[0].Data) != 2 {
			continue
		}
		name, known := sourceBridgeClassName(object, core.Convert2bytesToInt(ops[0].Data))
		callee := constructorMotionMember(object, ops[len(ops)-2], core.OP_INVOKESPECIAL)
		if known && callee != nil && callee.Member == "<init>" && callee.Name == name {
			result = append(result, name)
		}
	}
	return result
}

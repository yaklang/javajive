package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
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
		Out      string
		Profiles []struct {
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
		}
	}
	if err = json.Unmarshal(jdkInvocationCatalogJSON, &declared); err != nil {
		t.Fatal(err)
	}
	expected := map[int]string{}
	for _, p := range declared.Profiles {
		expected[p.Release] = p.ArchiveSHA256
	}
	if len(input.Profiles) != len(expected) {
		t.Fatal("generation must cover each exact declaration profile")
	}
	seenProfiles := map[int]bool{}
	document := constructorCodeDocument{Schema: 1}
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
		resolver := func(name string) ([]byte, bool) { raw, ok := originals[name]; return raw, ok }
		selected := map[string]bool{}
		names := make([]string, 0, len(objects))
		for name := range objects {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			obj := objects[name]
			// Bound the index to standard platform API namespaces, as with
			// the declaration catalog. Vendor/internal implementation APIs are
			// not extension-point roots; original ancestor bytes are retained.
			if !strings.HasPrefix(name, "java/") && !strings.HasPrefix(name, "javax/") {
				continue
			}
			// Declaration annotation policy also needs original classfiles:
			// Retention/Target cannot be guessed from an annotation's name.
			// These entries carry metadata, never a constructor-silence fact.
			if obj.AccessFlags&0x2001 == 0x2001 {
				selected[name] = true
				continue
			}
			// An external source subclass cannot extend a final/non-public
			// platform class. Retain usable extension points, then their full
			// ancestry below; internal implementation constructors are not roots.
			if obj.AccessFlags&0x0001 == 0 || obj.AccessFlags&0x0010 != 0 {
				continue
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolver, FuncCtx: &class_context.ClassContext{}}
			d.options.TargetSourceVersion = profile.Release
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			for _, method := range obj.Methods {
				n, _ := obj.getUtf8(method.NameIndex)
				desc, _ := obj.getUtf8(method.DescriptorIndex)
				if n != "<init>" || method.AccessFlags&(0x0001|0x0004) == 0 {
					continue
				}
				remaining := 512
				if d.constructorChainDoesNotObserve(name, desc, map[string]bool{}, map[string]bool{}, &remaining, 0) {
					selected[name] = true
					break
				}
			}
		}
		// Preserve original ancestry declarations, including interfaces with no
		// constructor. They are evidence for access/member completeness, not a
		// claim that every constructor they contain can commute with captures.
		pending := []string{}
		for name := range selected {
			pending = append(pending, name)
		}
		for len(pending) > 0 {
			name := pending[0]
			pending = pending[1:]
			obj := objects[name]
			parents := append([]string{obj.GetSupperClassName()}, obj.GetInterfacesName()...)
			for _, parent := range parents {
				if parent == "" || selected[parent] {
					continue
				}
				if objects[parent] == nil {
					t.Fatal("missing original ancestor", parent)
				}
				selected[parent] = true
				pending = append(pending, parent)
			}
		}
		p := constructorCodeProfile{Release: profile.Release, ArchiveSHA256: expected[profile.Release], Classes: map[string]string{}}
		for _, name := range names {
			if !selected[name] {
				continue
			}
			raw := originals[name]
			hash := sha256.Sum256(raw)
			p.Classes[name] = hex.EncodeToString(hash[:])
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
	t.Logf("original constructor evidence archive: %d bytes", out.Len())
}

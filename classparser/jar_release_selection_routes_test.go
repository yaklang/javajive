package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/filesys"
)

// Reviewed authored javac -proc:none fixtures, preserved unchanged from the
// original-first base8/MR9 binary-driver oracle. Their source methods are
// Choice.sentinel()/marker(); driver verifies both values and exactly 2 methods.
const jarReleaseChoiceBaseHex = "cafebabe0000003400100a000200030700040c000500060100106a6176612f6c616e672f4f626a6563740100063c696e69743e010003282956080008010005424153453807000a01000e6d7270726f62652f43686f696365010004436f646501000873656e74696e656c01001428294c6a6176612f6c616e672f537472696e673b0100066d61726b65720100032829490021000900020000000000030001000500060001000b0000001100010001000000052ab70001b1000000000009000c000d0001000b0000000f00010000000000031207b0000000000009000e000f0001000b0000000f00010000000000031008ac000000000000"
const jarReleaseChoice9Hex = "cafebabe0000003500100a000200030700040c000500060100106a6176612f6c616e672f4f626a6563740100063c696e69743e0100032829560800080100034d523907000a01000e6d7270726f62652f43686f696365010004436f646501000873656e74696e656c01001428294c6a6176612f6c616e672f537472696e673b0100066d61726b65720100032829490021000900020000000000030001000500060001000b0000001100010001000000052ab70001b1000000000009000c000d0001000b0000000f00010000000000031207b0000000000009000e000f0001000b0000000f00010000000000031009ac000000000000"
const jarReleaseDriverHex = "cafebabe00000034005e0a000200030700040c000500060100106a6176612f6c616e672f4f626a6563740100063c696e69743e0100032829560a0008000907000a0c000b000c01000e6d7270726f62652f43686f69636501000873656e74696e656c01001428294c6a6176612f6c616e672f537472696e673b0a0008000e0c000f00100100066d61726b657201000328294908001201000c43686f6963652e636c6173730a001400150700160c0017001801000f6a6176612f6c616e672f436c61737301000b6765745265736f75726365010022284c6a6176612f6c616e672f537472696e673b294c6a6176612f6e65742f55524c3b0a001a001b07001c0c001d001e0100106a6176612f6c616e672f537472696e6701000776616c75654f66010026284c6a6176612f6c616e672f4f626a6563743b294c6a6176612f6c616e672f537472696e673b0a001400200c002100220100126765744465636c617265644d6574686f647301001d28295b4c6a6176612f6c616e672f7265666c6563742f4d6574686f643b0700240100186a6176612f6c616e672f417373657274696f6e4572726f720800260100136d6574686f64206465636c61726174696f6e730a002300280c00050029010015284c6a6176612f6c616e672f4f626a6563743b295609002b002c07002d0c002e002f0100106a6176612f6c616e672f53797374656d0100036f75740100154c6a6176612f696f2f5072696e7453747265616d3b0700310100176a6176612f6c616e672f537472696e674275696c6465720a0030000308003401000973656e74696e656c3d0a003000360c00370038010006617070656e6401002d284c6a6176612f6c616e672f537472696e673b294c6a6176612f6c616e672f537472696e674275696c6465723b08003a010008206d61726b65723d0a0030003c0c0037003d01001c2849294c6a6176612f6c616e672f537472696e674275696c6465723b08003f01000a207265736f757263653d0a003000410c0042000c010008746f537472696e670a004400450700460c004700480100136a6176612f696f2f5072696e7453747265616d0100077072696e746c6e010015284c6a6176612f6c616e672f537472696e673b29560a001a004a0c004b004c010006657175616c73010015284c6a6176612f6c616e672f4f626a6563743b295a0a004e004f0700500c005100520100116a6176612f6c616e672f496e74656765720100087061727365496e74010015284c6a6176612f6c616e672f537472696e673b294908005401001577726f6e672072756e74696d652076617269616e740700560100116d726472697665722f4d52447269766572010004436f64650100046d61696e010016285b4c6a6176612f6c616e672f537472696e673b295601000d537461636b4d61705461626c6501000a457863657074696f6e7307005d0100136a6176612f6c616e672f457863657074696f6e002100550002000000000002000100050006000100570000001100010001000000052ab70001b10000000000090058005900020057000000910003000400000071b800074cb8000d3d12081211b60013b800194e1208b6001fbe059f000dbb0023591225b70027bfb2002abb003059b700321233b600352bb600351239b600351cb6003b123eb600352db60035b60040b600432b2a0332b6004999000d1c2a0432b8004d9f000dbb0023591253b70027bfb100000001005a0000000e0003fe002707001a0107001a3e09005b000000040001005c0000"

func jarReleaseChoiceBytes(t *testing.T) ([]byte, []byte) {
	t.Helper()
	base, variant := jarReleaseTestHex(t, jarReleaseChoiceBaseHex), jarReleaseTestHex(t, jarReleaseChoice9Hex)
	if fmt.Sprintf("%x", sha256.Sum256(base)) != "e81679ab32eb43e7ca8c09fd556a29e47e17e330cb1b6eb1f6d76a60563cbf3e" || fmt.Sprintf("%x", sha256.Sum256(variant)) != "9e81b83f6cd748ac232910c38ebf9e976257aa3912357dff256b0bfd1e5ad083" {
		t.Fatal("original authored class changed")
	}
	return base, variant
}
func jarReleaseWantBytes(t *testing.T, got []byte, err error, want []byte, route string) {
	t.Helper()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s SHA=%x err=%v want=%x", route, sha256.Sum256(got), err, sha256.Sum256(want))
	}
	obj, parseErr := Parse(got)
	if parseErr != nil || obj.GetClassName() != "mrprobe/Choice" {
		t.Fatalf("%s selected class identity %v", route, parseErr)
	}
}
func jarReleaseWantSource(t *testing.T, got []byte, err error, word, route string) {
	t.Helper()
	if err != nil || strings.Contains(string(got), DecompileStubMarker) || !strings.Contains(string(got), "\""+word+"\"") {
		t.Fatalf("%s source for %s: %v\n%s", route, word, err, got)
	}
}
func jarReleaseWantOpen(t *testing.T, open func(string) (fs.File, error), name string, want []byte) {
	t.Helper()
	file, err := open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(raw, want) {
		t.Fatalf("Open(%s) SHA=%x err=%v want=%x", name, sha256.Sum256(raw), err, sha256.Sum256(want))
	}
}

type jarReleaseRouteWorld struct {
	name, manifest string
	variant, multi bool
}

var jarReleaseOriginalRouteWorlds = []jarReleaseRouteWorld{
	{name: "original-mr-true", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a", variant: true, multi: true},
	{name: "current-rebuilt-mr-class-only", manifest: "", variant: true, multi: false},
	{name: "proposed-rebuilt-authorized-mr", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a", variant: true, multi: true},
	{name: "original-non-mr", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a0d0a", variant: true, multi: false},
	{name: "current-rebuilt-non-mr-class-only", manifest: "", variant: true, multi: false},
	{name: "proposed-rebuilt-non-mr", manifest: "", variant: true, multi: false},
	{name: "incorrect-directory-promoted-non-mr", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a", variant: true, multi: true},
	{name: "original-explicit-false", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a2066616c73650d0a0d0a", variant: true, multi: false},
	{name: "proposed-rebuilt-explicit-false", manifest: "", variant: true, multi: false},
	{name: "original-section-only-true", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a0d0a4e616d653a206172626974726172790d0a4d756c74692d52656c656173653a20747275650d0a0d0a", variant: true, multi: false},
	{name: "proposed-rebuilt-section-only-true", manifest: "", variant: true, multi: false},
	{name: "proposed-rebuilt-mr-variant-missing", manifest: "4d616e69666573742d56657273696f6e3a20312e300d0a4d756c74692d52656c656173653a20747275650d0a0d0a", variant: false, multi: true},
}

func TestAdversarialJarReleaseSelectionOriginalCallerRoutes(t *testing.T) {
	base, variant := jarReleaseChoiceBytes(t)
	for _, world := range jarReleaseOriginalRouteWorlds {
		t.Run(world.name, func(t *testing.T) {
			entries := []jarReleaseTestEntry{{"mrprobe/Choice.class", base}}
			if world.variant {
				entries = append(entries, jarReleaseTestEntry{"META-INF/versions/9/mrprobe/Choice.class", variant})
			}
			if world.manifest != "" {
				entries = append(entries, jarReleaseTestEntry{"META-INF/MANIFEST.MF", jarReleaseTestHex(t, world.manifest)})
			}
			raw := jarReleaseTestZIP(t, entries)
			disk := filepath.Join(t.TempDir(), "original.jar")
			if err := os.WriteFile(disk, raw, 0600); err != nil {
				t.Fatal(err)
			}
			for _, target := range []int{0, 8, 9, 17, 21} {
				t.Run(strconv.Itoa(target), func(t *testing.T) {
					opts := ArchiveOptionsFrom(DecompileOptions{TargetRelease: target})
					zipFS, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(raw), int64(len(raw)), filesys.WithArchiveOptions(opts))
					if err != nil {
						t.Fatal(err)
					}
					z := NewJarFS(zipFS)
					flag, err := z.RuntimeMultiRelease()
					if err != nil || flag != world.multi {
						t.Fatalf("original metadata=%v %v want %v", flag, err, world.multi)
					}
					want, word := base, "BASE8"
					if world.multi && world.variant && target >= 9 {
						want, word = variant, "MR9"
					}
					got, ok := z.enumSiblingResolver()("mrprobe/Choice")
					if !ok {
						t.Fatal("missing logical class declaration")
					}
					jarReleaseWantBytes(t, got, nil, want, "original sibling resolver")
					got, err = z.ReadFile("mrprobe/Choice.class")
					jarReleaseWantSource(t, got, err, word, "JarFS.ReadFile")
					sourceFile, err := z.Open("mrprobe/Choice.class")
					if err != nil {
						t.Fatal(err)
					}
					got, err = io.ReadAll(sourceFile)
					sourceFile.Close()
					jarReleaseWantSource(t, got, err, word, "JarFS.Open")
					expanded := NewExpandedZipFS(zipFS, zipFS)
					got, err = expanded.ReadFile("mrprobe/Choice.class")
					jarReleaseWantBytes(t, got, err, want, "Expanded.ReadFile")
					jarReleaseWantOpen(t, expanded.Open, "mrprobe/Choice.class", want)
					jarReleaseWantOpen(t, func(n string) (fs.File, error) { return expanded.OpenFile(n, os.O_RDONLY, 0) }, "mrprobe/Choice.class", want)
					public, err := NewExpandedArchiveFileSystemFromLocalWithArchiveOptions(disk, true, opts)
					if err != nil {
						t.Fatal(err)
					}
					got, err = public.ReadFile("mrprobe/Choice.class")
					jarReleaseWantBytes(t, got, err, want, "public local archive read")
					// Preserve the older raw force-view contract even for non-MR archives.
					forceWant := base
					if world.variant && target >= 9 {
						forceWant = variant
					}
					got, err = zipFS.ReadFile("mrprobe/Choice.class")
					jarReleaseWantBytes(t, got, err, forceWant, "raw ZipFS force view")
					if world.variant {
						got, err = z.readJarEntry("META-INF/versions/9/mrprobe/Choice.class")
						jarReleaseWantBytes(t, got, err, variant, "explicit physical class")
						view := z.sourceReleaseView(9)
						if view == nil {
							t.Fatal("missing physical source view")
						}
						got, ok = view.enumSiblingResolver()("mrprobe/Choice")
						if !ok {
							t.Fatal("physical sibling missing")
						}
						jarReleaseWantBytes(t, got, nil, variant, "physical source declaration role")
						if !view.originalMemberIndex().valid {
							t.Fatal("physical source index rejected valid original namespace")
						}
					}
					if !z.originalMemberIndex().valid {
						t.Fatal("logical member index rejected selected class")
					}
				})
			}
		})
	}
}

func TestAdversarialJarReleaseSelectionNestedCallerRoles(t *testing.T) {
	base, variant := jarReleaseChoiceBytes(t)
	innerBase := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"mrprobe/Choice.class", base}, {"META-INF/versions/9/mrprobe/Choice.class", variant}})
	innerVariant := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"mrprobe/Choice.class", variant}})
	for _, multi := range []bool{false, true} {
		t.Run(strconv.FormatBool(multi), func(t *testing.T) {
			manifest := []byte("Manifest-Version: 1.0\r\nMulti-Release: " + strconv.FormatBool(multi) + "\r\n\r\n")
			outer := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", manifest}, {"inner.jar", innerBase}, {"META-INF/versions/9/inner.jar", innerVariant}})
			opts := ArchiveOptionsFrom(DecompileOptions{TargetRelease: 21})
			zipFS, err := filesys.NewZipFSRawWithOptions(bytes.NewReader(outer), int64(len(outer)), filesys.WithArchiveOptions(opts))
			if err != nil {
				t.Fatal(err)
			}
			expanded := NewExpandedZipFS(zipFS, zipFS)
			wantInner, want, word := innerBase, base, "BASE8"
			if multi {
				wantInner, want, word = innerVariant, variant, "MR9"
			}
			got, err := expanded.ReadFile("inner.jar")
			if err != nil || !bytes.Equal(got, wantInner) {
				t.Fatalf("outer selected wrong nested entry %v", err)
			}
			nested, err := expanded.GetJarFS("inner.jar")
			if err != nil {
				t.Fatal(err)
			}
			inner := nested.GetFileSystem().(*JarFS)
			flag, err := inner.RuntimeMultiRelease()
			if err != nil || flag {
				t.Fatalf("outer permission leaked into non-MR inner %v %v", flag, err)
			}
			got, ok := inner.enumSiblingResolver()("mrprobe/Choice")
			if !ok {
				t.Fatal("nested class missing")
			}
			jarReleaseWantBytes(t, got, nil, want, "nested actual class")
			got, err = expanded.ReadFile("inner.jar/mrprobe/Choice.java")
			jarReleaseWantSource(t, got, err, word, "Expanded nested source")
			parent := NewJarFS(zipFS)
			got, err = parent.ReadFile("inner.jar/mrprobe/Choice.java")
			jarReleaseWantSource(t, got, err, word, "JarFS nested source")
			// Opening the exact physical outer variant retains its independent source role.
			physical, err := expanded.GetJarFS("META-INF/versions/9/inner.jar")
			if err != nil {
				t.Fatal(err)
			}
			got, ok = physical.GetFileSystem().(*JarFS).enumSiblingResolver()("mrprobe/Choice")
			if !ok {
				t.Fatal("physical nested missing")
			}
			jarReleaseWantBytes(t, got, nil, variant, "physical nested role")
		})
	}
}

func TestAdversarialJarReleaseSelectionMemberIndexNamespaces(t *testing.T) {
	base, variant := jarReleaseChoiceBytes(t)
	for _, manifest := range []string{"Manifest-Version: 1.0\nMulti-Release: false\n\n", "Manifest-Version: 1.0\nBroken\n"} {
		t.Run(fmt.Sprintf("%x", sha256.Sum256([]byte(manifest))), func(t *testing.T) {
			raw := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"META-INF/MANIFEST.MF", []byte(manifest)}, {"mrprobe/Choice.class", base}, {"META-INF/versions/9/mrprobe/Choice.class", variant}})
			z := jarReleaseTestLegacy(t, raw, 21)
			_, metadataErr := z.RuntimeMultiRelease()
			view := z.sourceReleaseView(9)
			if view == nil {
				t.Fatal("metadata refusal denied physical namespace")
			}
			got, ok := view.enumSiblingResolver()("mrprobe/Choice")
			if !ok {
				t.Fatal("physical sibling missing")
			}
			jarReleaseWantBytes(t, got, nil, variant, "independent physical source")
			if !view.originalMemberIndex().valid {
				t.Fatal("metadata permission contaminated physical index")
			}
			if metadataErr != nil && z.originalMemberIndex().valid {
				t.Fatal("unknown logical metadata admitted index")
			}
		})
	}
	// A non-MR version-only class must be skipped before logical resolution.
	raw := jarReleaseTestZIP(t, []jarReleaseTestEntry{{"mrprobe/Choice.class", base}, {"META-INF/versions/9/VersionOnly.class", []byte("invalid ignored physical class")}})
	z := jarReleaseTestLegacy(t, raw, 21)
	if !z.originalMemberIndex().valid {
		t.Fatal("non-MR version-only node invalidated logical index")
	}
	if z.sourceReleaseView(9).originalMemberIndex().valid {
		t.Fatal("physical namespace skipped its own invalid version-only declaration")
	}
}

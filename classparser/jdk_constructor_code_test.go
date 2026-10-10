package javaclassparser

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestAdversarialPlatformInterfaceDeclarationsUsePinnedOriginalFlags(t *testing.T) {
	var catalog struct {
		Profiles []struct {
			Release       int
			ArchiveSHA256 string `json:"archive_sha256"`
			Archives      map[string]string
			Classes       map[string]struct {
				IsInterface bool
				Parents     []string
			}
			Provenance map[string]struct {
				Archive string
				SHA256  string
			}
		}
	}
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, profile := range catalog.Profiles {
		count := 0
		for name, decl := range profile.Classes {
			provenance := profile.Provenance[name]
			if !decl.IsInterface || profile.Archives[provenance.Archive] != profile.ArchiveSHA256 {
				continue
			}
			raw, known := jdkConstructorClassBytes(name, profile.Release)
			if !known {
				t.Fatalf("missing pinned original %d/%s", profile.Release, name)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != provenance.SHA256 {
				t.Fatalf("interface bytes differ from pinned catalog %d/%s: %s", profile.Release, name, got)
			}
			obj, err := Parse(raw)
			if err != nil || obj.GetClassName() != name || obj.AccessFlags&0x0200 == 0 || int(obj.MajorVersion) != profile.Release+44 {
				t.Fatalf("wrong original interface %d/%s: %v", profile.Release, name, err)
			}
			for _, parent := range obj.GetInterfacesName() {
				if _, known := jdkConstructorClassBytes(parent, profile.Release); !known {
					t.Fatalf("missing original interface parent %d/%s", profile.Release, parent)
				}
			}
			count++
		}
		if count == 0 {
			t.Fatal("empty interface profile", profile.Release)
		}
	}
}

func TestAdversarialPlatformConstructorCodeIdentityAndEffectProof(t *testing.T) {
	// Read every retained class, independently check its original identity and
	// version, and require closed original ancestry. Hash verification happens
	// inside the lookup, with archives bound to the declaration catalog.
	var declared struct{ Profiles []struct{ Release int } }
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &declared); err != nil {
		t.Fatal(err)
	}
	for _, profile := range declared.Profiles {
		raw, ok := jdkConstructorClassBytes("java/util/AbstractList", profile.Release)
		if !ok {
			t.Fatal("missing original platform constructor", profile.Release)
		}
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for name := range jdkConstructorCode.entries[profile.Release] {
			data, ok := jdkConstructorClassBytes(name, profile.Release)
			if !ok {
				t.Fatal("original hash mismatch", name)
			}
			c, err := Parse(data)
			if err != nil || c.GetClassName() != name || int(c.MajorVersion) != profile.Release+44 {
				t.Fatalf("wrong class identity/profile: %d/%s %v", profile.Release, name, err)
			}
			for _, parent := range append([]string{c.GetSupperClassName()}, c.GetInterfacesName()...) {
				if parent != "" {
					if _, ok := jdkConstructorClassBytes(parent, profile.Release); !ok {
						t.Fatalf("unclosed original ancestry: %s -> %s", name, parent)
					}
				}
			}
		}
		d := &ClassObjectDumper{obj: obj, FuncCtx: &class_context.ClassContext{}}
		d.options.TargetSourceVersion = profile.Release
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		for _, tc := range []struct {
			owner, desc string
			want        bool
		}{
			{"java/util/AbstractList", "()V", true}, {"java/io/InputStream", "()V", true},
			{"java/util/HashMap", "()V", true}, {"java/util/HashMap", "(I)V", false},
		} {
			if _, ok := jdkConstructorClassBytes(tc.owner, profile.Release); !ok {
				t.Fatal("negative must exercise real retained code", tc.owner)
			}
			remaining := 512
			if got := d.constructorChainDoesNotObserve(tc.owner, tc.desc, map[string]bool{}, map[string]bool{}, &remaining, 0); got != tc.want {
				t.Fatalf("%d/%s%s effect proof=%v want=%v", profile.Release, tc.owner, tc.desc, got, tc.want)
			}
		}
		// Explicit bytes, including invalid or wrong-identity bytes, remain
		// authoritative. A platform index must not conceal caller evidence errors.
		d.foldSiblingResolver = func(string) ([]byte, bool) { return []byte{0, 1}, true }
		if _, ok := d.constructorMotionClass("java/io/InputStream"); ok {
			t.Fatal("invalid original provider hidden by platform fallback")
		}
		d.foldSiblingResolver = func(string) ([]byte, bool) { return raw, true }
		if _, ok := d.constructorMotionClass("java/io/InputStream"); ok {
			t.Fatal("wrong original identity hidden by platform fallback")
		}
	}
	for _, release := range []int{0, 7, 10, 12, 18, 22} {
		if _, ok := jdkConstructorClassBytes("java/util/AbstractList", release); ok {
			t.Fatal("uncataloged profile was substituted", release)
		}
	}
	raw, ok := jdkConstructorClassBytes("java/util/AbstractList", 8)
	if !ok {
		t.Fatal("missing original")
	}
	raw[0] ^= 0xff
	again, ok := jdkConstructorClassBytes("java/util/AbstractList", 8)
	if !ok || again[0] != 0xca {
		t.Fatal("request mutated shared original evidence")
	}
}

func TestAdversarialPlatformConstructorMovementRequiresRuntimeAgreement(t *testing.T) {
	// Use actual original code to find a constructor proved in the source
	// profile whose later runtime evidence is missing or does not admit the
	// movement. A source-only proof must not authorize that transformation.
	if _, ok := jdkConstructorClassBytes("java/lang/Object", 8); !ok {
		t.Fatal("missing pinned runtime evidence")
	}
	names := []string{}
	for name := range jdkConstructorCode.entries[8] {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw, _ := jdkConstructorClassBytes(name, 8)
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range obj.Methods {
			n, _ := obj.getUtf8(method.NameIndex)
			desc, _ := obj.getUtf8(method.DescriptorIndex)
			if n != "<init>" {
				continue
			}
			// Keep this object separate from the platform owner: caller-supplied
			// root bytes would correctly remain authoritative in every attempt.
			callerRaw, _ := jdkConstructorClassBytes("java/util/AbstractList", 8)
			caller, err := Parse(callerRaw)
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: caller, FuncCtx: &class_context.ClassContext{}}
			d.options.TargetSourceVersion = 8
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			remaining := 512
			if !d.constructorChainDoesNotObserve(name, desc, map[string]bool{}, map[string]bool{}, &remaining, 0) {
				continue
			}
			later := *d
			later.options.TargetSourceVersion = 21
			later.FuncCtx = &class_context.ClassContext{}
			later.FuncCtx.InvocationMetadata = later.buildInvocationMetadata()
			remaining = 512
			if later.constructorChainDoesNotObserve(name, desc, map[string]bool{}, map[string]bool{}, &remaining, 0) {
				continue
			}
			if d.constructorCaptureChainDoesNotObserve(name, desc, map[string]bool{}) {
				t.Fatalf("source-only platform proof authorized incompatible runtime: %s%s", name, desc)
			}
			t.Logf("original cross-profile counterexample: %s%s", name, desc)
			return
		}
	}
	t.Fatal("catalog contains no cross-profile counterexample; add explicit original evidence")
}

func TestAdversarialInvocationMetadataRetainsOriginalClassSignature(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		for _, name := range []string{"java/util/List", "java/util/Collection", "java/lang/Iterable", "java/util/AbstractCollection"} {
			raw, ok := jdkConstructorClassBytes(name, release)
			if !ok {
				t.Fatal("missing original generic declaration", release, name)
			}
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj}
			d.options.TargetSourceVersion = release
			catalog, ok := jdkInvocationMetadata(name, release)
			if !ok || catalog.Signature == "" {
				t.Fatal("missing independent original catalog signature", name)
			}
			original, ok := d.buildInvocationMetadata()(name)
			if !ok || original.Signature != catalog.Signature {
				t.Fatalf("original class formal/inheritance Signature lost: %d/%s: %q want %q", release, name, original.Signature, catalog.Signature)
			}
			for _, a := range obj.Attributes {
				if signature, ok := a.(*SignatureAttribute); ok {
					// Explicit malformed original evidence cannot be concealed by
					// the otherwise valid platform declaration fallback.
					prior := signature.SignatureIndex
					signature.SignatureIndex = 0
					if _, ok := d.buildInvocationMetadata()(name); ok {
						t.Fatal("malformed original class Signature accepted")
					}
					signature.SignatureIndex = prior
					obj.Attributes = append(obj.Attributes, signature)
					if _, ok := d.buildInvocationMetadata()(name); ok {
						t.Fatal("duplicate original class Signature accepted")
					}
					break
				}
			}
		}
	}
}

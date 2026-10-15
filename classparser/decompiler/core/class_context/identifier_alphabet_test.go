package class_context

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"
)

// The frozen oracle bytes were emitted by the two trusted Character int APIs,
// not inferred from the implementation's tables. The renderer's live JDK21
// oracle also runs separately in the source regression bank.
func TestIdentifierAlphabetsMatchFrozenCharacterProfiles(t *testing.T) {
	for _, profile := range []struct {
		name, digest string
		bits         func(rune) uint8
	}{
		{"java8", "072e36cb88f4a61bd70bb39de0651238561bd230df12cc3eaf1a332067b722e7", java8IdentifierBits},
		{"java21", "211b9e2bd49c5e47aa425779c88c4237807f5253a128406b19a8fb743fbf1c21", java21IdentifierBits},
	} {
		t.Run(profile.name, func(t *testing.T) {
			encoded, err := os.ReadFile("testdata/" + profile.name + "_identifier_flags.bin.gz")
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(io.LimitReader(reader, 0x110001))
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			if len(raw) != 0x110000 || fmt.Sprintf("%x", sha256.Sum256(raw)) != profile.digest {
				t.Fatal("incomplete or changed independent Character reference")
			}
			for cp, flags := range raw {
				want := flags & 1
				if flags&2 != 0 && flags&4 == 0 {
					want |= 2
				}
				if got := profile.bits(rune(cp)); got != want {
					t.Fatalf("U+%04X stable-name bits=%d, original Character profile requires %d", cp, got, want)
				}
			}
		})
	}
}

func TestSafeIdentifierPreservesCanonicalUnicodeWithoutNormalization(t *testing.T) {
	for _, name := range []string{"plain", "变量", "α", "\U00010400", "e\u0301", "é", "€", "‿", "Ⅻ", "x١", "r\uFE0F", "$field"} {
		t.Run(name, func(t *testing.T) {
			if got := SafeIdentifier(name); got != name {
				t.Fatalf("legal original symbol changed from %q to %q", name, got)
			}
			if !PreservesJava8Identifier(name) {
				t.Fatal("shared Java8/Java21 identifier lost its source certificate")
			}
		})
	}
	if SafeIdentifier("é") == SafeIdentifier("e\u0301") || SafeIdentifier("r") == SafeIdentifier("r\uFE0F") {
		t.Fatal("distinct JVM symbols were normalized together")
	}
}

func TestSafeIdentifierRepairsHaveNoImplicitOriginalNamePermission(t *testing.T) {
	for _, name := range []string{"", "_", "class", "a-b", "9name", "\u0301", "١", "a\u200Db", "a\u0000b", "\u2028", "\U0001F600", string([]byte{0xff})} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			got := SafeIdentifier(name)
			if got == "" || got == "_" || got == name || SafeIdentifier(got) != got {
				t.Fatalf("unsafe or unstable repair %q -> %q", name, got)
			}
			if PreservesJava8Identifier(name) {
				t.Fatal("a renamed or ignorable symbol gained original-source permission")
			}
		})
	}
}

func TestNativeJava8IdentifierCannotBorrowNewerRendererAlphabet(t *testing.T) {
	for _, name := range []string{"₿", "\U0001E900", "\U00031350", "prefix\U00031350"} {
		t.Run(name, func(t *testing.T) {
			if SafeIdentifier(name) != name {
				t.Fatal("the pinned modern renderer must preserve its legal symbol")
			}
			if PreservesJava8Identifier(name) {
				t.Fatal("a post-Java8 Unicode addition fabricated an older source certificate")
			}
		})
	}
}

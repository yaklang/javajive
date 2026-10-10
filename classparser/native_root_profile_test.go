package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeRootConstructorBridgeRejectsUnregenerableClassFlags(t *testing.T) {
	for _, flags := range []uint16{0x1000, 0x0020} {
		t.Run(map[uint16]string{0x1000: "synthetic", 0x0020: "without-super"}[flags], func(t *testing.T) {
			fixture := nativeRootPrivateConstructorFixture
			if flags == 0x1000 {
				fixture = strings.Replace(fixture, "Object token=new Object();int rows=0;", "Object token=new Object();if(!RootBridgePacket.class.isSynthetic())throw new AssertionError(\"original synthetic root\");int rows=0;", 1)
			}
			files := nativeCompileClasses(t, fixture)
			root, err := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			if flags == 0x1000 {
				root.AccessFlags |= flags
			} else {
				root.AccessFlags &^= flags
			}
			files["RootBridgePacket.class"] = root.Bytes()
			_, java := t04Tools(t)
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "RootBridgeDriver"); got != "2:private-root:identity:order:owner\n" {
				t.Fatalf("valid original JVM=%q", got)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			entry := z.nativeMemberEntry(root)
			if entry != nil && entry.family != nil {
				t.Fatal("original root flags have no matching Java-source compiler profile")
			}
		})
	}
}

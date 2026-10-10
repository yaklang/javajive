package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialAnonymousConditionalInitializerNativeCompilerProtocol(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	compile := func(debug string) map[string][]byte {
		root := t.TempDir()
		path := filepath.Join(root, "ConditionalInitOwner.java")
		if err := os.WriteFile(path, []byte(anonymousConditionalInitializerFixture), 0600); err != nil {
			t.Fatal(err)
		}
		if data, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
			t.Fatal("authored original compile", err, string(data))
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".class") {
				raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = raw
			}
		}
		return files
	}
	testNativeIndependentCompilerFamilyFixture(t, compile, NativeJavac8, javac, []string{"ConditionalInitOwner"}, "ConditionalInitDriver", "7:conditional:initializer:branch:partial:identity\n", nil, nativeLexicalExactSignatures)
}

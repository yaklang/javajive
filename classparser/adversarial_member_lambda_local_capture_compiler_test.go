package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdversarialMemberLambdaLocalCaptureNativeCompilerProtocol(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	for _, tc := range []struct{ name, fixture, owner, driver, want string }{
		{"reference", memberLambdaLocalCaptureFixture, "LocalCaptureOwner", "LocalCaptureDriver", "51:local:lambda:capture:snapshot:mutation:lazy:failure\n"},
		{"primitive", memberLambdaPrimitiveCaptureFixture, "WordCaptureOwner", "WordCaptureDriver", "8004:word:local:capture:bits:order:identity\n"},
		{"selected word", memberLambdaConditionalCaptureFixture, "JoinedCaptureOwner", "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n"},
		{"branch wide stores", strings.Replace(memberLambdaConditionalWideFixture(), "final long value=choose?JoinedCaptureEffects.left(seed):JoinedCaptureEffects.right(seed);", "final long value;if(choose){value=JoinedCaptureEffects.left(seed);}else{value=JoinedCaptureEffects.right(seed);}", 1), "JoinedCaptureOwner", "JoinedCaptureDriver", "52:conditional:local:lambda:choice:lazy:identity\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compile := func(debug string) map[string][]byte {
				root := t.TempDir()
				path := filepath.Join(root, "CaptureOwner.java")
				if err := os.WriteFile(path, []byte(tc.fixture), 0600); err != nil {
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
			testNativeIndependentCompilerFamilyFixture(t, compile, NativeJavac8, javac, []string{tc.owner}, tc.driver, tc.want, nil, nativeLexicalExactSignatures)
		})
	}
}

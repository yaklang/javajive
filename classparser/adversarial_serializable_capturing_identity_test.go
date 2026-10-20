package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A valid JVM binary name need not be a Java source identifier. An otherwise
// verified deserializer cannot authorize renaming its capturing class, because
// both the old caller and the serialized form retain the original identity.
func TestAdversarialSerializableCapturingClassCannotChangeBinaryIdentity(t *testing.T) {
	original := nativeCompileDebugClasses(t, serializableStaticReferenceFixture, "none")
	javac, java := t04Tools(t)
	for _, owner := range []string{"RenamedCapture", "switch"} {
		t.Run(owner, func(t *testing.T) {
			files := map[string][]byte{}
			for name, raw := range original {
				object, err := Parse(bytes.Clone(raw))
				if err != nil {
					t.Fatal(err)
				}
				for _, constant := range object.ConstantPool {
					if text, known := constant.(*ConstantUtf8Info); known && text != nil {
						text.Value = strings.ReplaceAll(text.Value, "SerializableReferences", owner)
					}
				}
				files[strings.ReplaceAll(name, "SerializableReferences", owner)] = object.Bytes()
			}
			oracle := t04RunJava(t, java, adversarialAssertionClasses(t, files), "SerializationDriver")
			if !strings.HasSuffix(oracle, "44:serializable:static:identity:protocol\n") {
				t.Fatal("valid original binary-name/protocol control", oracle)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			source, err := z.ReadFile(owner + ".class")
			published := err == nil && len(source) > 0 && !strings.Contains(string(source), DecompileStubMarker) && !strings.Contains(string(source), "decompile dump failed")
			if owner == "switch" && !published {
				return
			}
			if !published {
				t.Fatal("ordinary renamed capturing class lost supported source", err, string(source))
			}
			out := t.TempDir()
			for name, raw := range files {
				if name != owner+".class" {
					if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(out, owner+".java")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			compiled, compileErr := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, path).CombinedOutput()
			if owner == "switch" {
				rebuilt, runErr := exec.Command(java, "-Xverify:all", "-cp", out, "SerializationDriver").CombinedOutput()
				t.Fatalf("source published a different capturing-class binary identity: javac=%v %s unchanged caller=%v %s\n%s", compileErr, compiled, runErr, rebuilt, source)
			}
			if compileErr != nil {
				t.Fatal("ordinary renamed source compilation", compileErr, string(compiled))
			}
			if got := t04RunJava(t, java, out, "SerializationDriver"); got != oracle {
				t.Fatal("ordinary renamed source changed serialized protocol", got, oracle)
			}
		})
	}
}

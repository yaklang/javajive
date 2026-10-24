package javaclassparser

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This checks admission only. Completed-source publication, fresh helper
// identity and unchanged Code remain separately checked by the artifact tests.
func TestCompilerReferenceParameterTableNeedsOriginalArtifactAndCompilerSchema(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Fatal("native compiler required")
	}
	fixture := strings.Replace(localVarargsReceiverFixture, "$BODY", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;", 1)
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, "LocalReceiverOwner", "none", true)
	for _, variant := range []string{"artifact", "plain source", "stale original", "changed port name", "changed port flags", "wrong language", "unknown compiler"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, files["LocalReceiverOwner.class"])
			var method *MemberInfo
			var table *UnparsedAttribute
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x100a {
					if method != nil {
						t.Fatal("unique adapter")
					}
					method = m
				}
			}
			if method == nil {
				t.Fatal("adapter missing")
			}
			for _, a := range method.Attributes {
				if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
					table = p
				}
			}
			if table == nil || len(table.Info) < 5 {
				t.Fatal("original named adapter table")
			}
			if variant == "changed port name" {
				binary.BigEndian.PutUint16(table.Info[1:3], uint16(obj.ConstantPoolManager.AddUtf8Info("unprovedReceiver")))
			}
			if variant == "changed port flags" {
				table.Info[4] |= 0x10
			}
			owned := map[string][]byte{}
			for n, b := range files {
				owned[n] = b
			}
			owned["LocalReceiverOwner.class"] = obj.Bytes()
			jar := filepath.Join(t.TempDir(), "input.jar")
			if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			d := NewClassObjectDumper(obj)
			d.options.TargetSourceVersion, d.options.SourceCompiler, d.options.sourceLambdaMetadata = 8, ModernJavac, a.jar.sourceLambdaMetadata
			d.foldSiblingResolver = func(n string) ([]byte, bool) { b, k := owned[n+".class"]; return b, k }
			shape, known := d.originalVarargsAdapterShape(method)
			if !known {
				t.Fatal("original typed packing shape")
			}
			switch variant {
			case "plain source":
				d.options.sourceLambdaMetadata = nil
			case "stale original":
				obj.MinorVersion++
			case "wrong language":
				d.options.TargetSourceVersion = 11
			case "unknown compiler":
				d.options.SourceCompiler = "unknown"
			}
			if got := d.compilerVarargsParameterSchema(shape); got != (variant == "artifact") {
				t.Fatalf("schema accepted=%v", got)
			}
		})
	}
}

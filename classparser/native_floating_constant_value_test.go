package javaclassparser

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeFloatingConstantValueStatusInShadowedScope(t *testing.T) {
	files := nativeCompileClasses(t, `class Float{}class Double{}class FloatingFields{static final float FN=0F/0F,FP=1F/0F,FM=-1F/0F,FZ=-0F;static final double DN=0D/0D,DP=1D/0D,DM=-1D/0D,DZ=-0D;}`)
	javac, _ := t04Tools(t)
	snapshot := func(t *testing.T, raw []byte) string {
		t.Helper()
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		type constant struct {
			Descriptor string
			Flags      uint16
			Bits       uint64
			Count      int
		}
		data := map[string]constant{}
		for _, field := range obj.Fields {
			name, ok := sourceBridgeUTF8(obj, field.NameIndex)
			if !ok {
				t.Fatal("name")
			}
			descriptor, ok := sourceBridgeUTF8(obj, field.DescriptorIndex)
			if !ok {
				t.Fatal("descriptor")
			}
			value := constant{Descriptor: descriptor, Flags: field.AccessFlags}
			for _, attr := range field.Attributes {
				if a, ok := attr.(*ConstantValueAttribute); ok {
					value.Count++
					cp, err := obj.getConstantInfo(a.ConstantValueIndex)
					if err != nil {
						t.Fatal(err)
					}
					switch c := cp.(type) {
					case *ConstantFloatInfo:
						value.Bits = uint64(math.Float32bits(c.Value))
					case *ConstantDoubleInfo:
						value.Bits = math.Float64bits(c.Value)
					default:
						t.Fatalf("unexpected constant %T", cp)
					}
				}
			}
			if value.Count != 1 {
				t.Fatalf("field%s has%d constants", name, value.Count)
			}
			data[name] = value
		}
		for _, method := range obj.Methods {
			name, _ := sourceBridgeUTF8(obj, method.NameIndex)
			if name == "<clinit>" {
				t.Fatal("primitive constant gained an initializer")
			}
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	original := files["FloatingFields.class"]
	for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
		t.Run(string(mode), func(t *testing.T) {
			output := t.TempDir()
			for _, name := range []string{"Float", "Double"} {
				if err := os.WriteFile(filepath.Join(output, name+".class"), files[name+".class"], 0600); err != nil {
					t.Fatal(err)
				}
			}
			resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
			var source string
			var err error
			if mode == "legacy" {
				source, err = DecompileWithResolver(original, resolve)
			} else {
				var result DecompileResult
				result, err = DecompileWithOptions(original, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
				source = result.Source
			}
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(output, "FloatingFields.java")
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
				t.Fatalf("%v %s\n%s", err, out, source)
			}
			rebuilt, err := os.ReadFile(filepath.Join(output, "FloatingFields.class"))
			if err != nil {
				t.Fatal(err)
			}
			if want, got := snapshot(t, original), snapshot(t, rebuilt); want != got {
				t.Fatalf("constant bits/status changed\n%s\n%s", want, got)
			}
		})
	}
}

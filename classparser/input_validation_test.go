package javaclassparser

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"testing"
)

const validationSeedClass = "../test/cross/testdata/irreducible/original/IrreducibleMultiExitEffects.class"

func readValidationSeed(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(validationSeedClass)
	if err != nil {
		t.Fatalf("read seed class: %v", err)
	}
	return raw
}

func TestDecompileRejectsUnsupportedClassfileMajor(t *testing.T) {
	seed := readValidationSeed(t)
	for _, major := range []uint16{0, 1, 44, 45, 49, 52, 65, 66, 99, 255} {
		major := major
		t.Run(strconv.Itoa(int(major)), func(t *testing.T) {
			raw := append([]byte(nil), seed...)
			binary.BigEndian.PutUint16(raw[6:8], major)

			// Parse stays permissive: unknown versions are a capability boundary,
			// not a structural parse error.
			if _, err := Parse(raw); err != nil {
				t.Fatalf("Parse rejected major %d: %v", major, err)
			}
			_, err := DecompileWithOptions(raw, DecompileOptions{
				Mode:        Precision,
				EnvSnapshot: map[string]string{},
			})
			unsupported := err != nil && strings.Contains(err.Error(), "unsupported_classfile_version")
			if major < MinSupportedClassMajor || major > MaxSupportedClassMajor {
				if !unsupported {
					t.Fatalf("major %d: got error %v, want unsupported_classfile_version", major, err)
				}
				return
			}
			if unsupported {
				t.Fatalf("major %d: supported version was rejected: %v", major, err)
			}
		})
	}
}

func diagnosticTextForTest(result DecompileResult) string {
	var parts []string
	for _, diagnostic := range result.Diagnostics {
		parts = append(parts, diagnostic.Code+": "+diagnostic.Message)
	}
	return strings.Join(parts, " | ")
}

func TestDecompileRejectsOutOfRangeConstantPoolOperand(t *testing.T) {
	seed := readValidationSeed(t)
	object, err := Parse(seed)
	if err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	var code *CodeAttribute
	for _, method := range object.Methods {
		if int(method.NameIndex) == 0 || int(method.NameIndex) > len(object.ConstantPool) {
			continue
		}
		nameInfo, ok := object.ConstantPool[method.NameIndex-1].(*ConstantUtf8Info)
		if !ok || nameInfo.Value != "run" {
			continue
		}
		for _, attr := range method.Attributes {
			if candidate, ok := attr.(*CodeAttribute); ok {
				code = candidate
				break
			}
		}
	}
	if code == nil {
		t.Fatal("seed run method has no Code attribute")
	}

	mutated := append([]byte(nil), code.Code...)
	found := false
	for i := 0; i+2 < len(mutated); i++ {
		if mutated[i] != 0xb2 { // getstatic
			continue
		}
		binary.BigEndian.PutUint16(mutated[i+1:i+3], 0xffff)
		found = true
		break
	}
	if !found {
		t.Fatal("seed run method has no getstatic operand to corrupt")
	}
	code.Code = mutated

	result, err := DecompileWithOptions(object.Bytes(), DecompileOptions{
		Mode:        Precision,
		EnvSnapshot: map[string]string{},
	})
	if err != nil {
		t.Fatalf("malformed operand must degrade per method, got request error: %v", err)
	}
	if result.Status == "complete" || len(result.StubMethods) == 0 {
		t.Fatalf("malformed operand was accepted: status=%s stubs=%v", result.Status, result.StubMethods)
	}
	diagnostics := diagnosticTextForTest(result)
	if !strings.Contains(diagnostics, "invalid_constant_pool_reference") {
		t.Fatalf("diagnostics = %q, want invalid_constant_pool_reference", diagnostics)
	}
}

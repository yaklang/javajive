package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialEnumRootAssertionOwnerRequiresSourceAndCacheProof(t *testing.T) {
	originals, fresh, artifact := enumFlatTestInputs(t, "source,lines,vars", enumAssertionPrefixCompositionFixture)
	const name = "PortOwner$Mode.class"
	for _, variant := range []string{"control", "foreign status", "wrong branch", "wrong cache flags", "unknown PC attribute", "missing source", "pending source", "stale original", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object := mustParseLowering(t, bytes.Clone(fresh[name]))
			code := constructorLoweringCode(enumAssertionInitializer(t, object))
			authority := artifact.source.jar.sourceConstructorLowering
			original := originals[name]
			var work *workbudget.Budget
			switch variant {
			case "foreign status":
				index := object.ConstantPoolManager.AddNewClassInfo("ForeignAssertionOwner")
				if code.Code[0] != core.OP_LDC || index > 255 {
					t.Fatal("small literal fixture")
				}
				code.Code[1] = byte(index)
			case "wrong branch":
				ops, ok := nativeEnumMethodOps(object, enumAssertionInitializer(t, object), nil)
				if !ok || ops[2].Instr.OpCode != core.OP_IFNE {
					t.Fatal("compiler cache branch")
				}
				code.Code[int(ops[2].CurrentOffset)] = core.OP_IFEQ
			case "wrong cache flags":
				found := false
				for _, field := range object.Fields {
					if n, _ := sourceBridgeUTF8(object, field.NameIndex); n == nativeAssertionField {
						field.AccessFlags &^= 0x10
						found = true
					}
				}
				if !found {
					t.Fatal("compiler cache field")
				}
			case "unknown PC attribute":
				code.Attributes = append(code.Attributes, &UnparsedAttribute{Name: "UnknownPCMap", Length: 2, Info: []byte{0, 0}})
				enumPortFixCodeLength(t, object, code)
			case "missing source":
				authority = &sourceConstructorLoweringAuthority{jar: artifact.source.jar}
			case "pending source":
				authority = &sourceConstructorLoweringAuthority{jar: artifact.source.jar, enumPending: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": {}}}
			case "stale original":
				o := mustParseLowering(t, bytes.Clone(original))
				o.MinorVersion++
				original = o.Bytes()
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := object.Bytes()
			before := bytes.Clone(input)
			output, err := authority.restoreEnumAssertionOwner(original, input, work)
			if !bytes.Equal(input, before) {
				t.Fatal("fresh input mutated")
			}
			if variant != "control" {
				if err == nil && !bytes.Equal(output, input) {
					t.Fatal("unproved cache was changed")
				}
				return
			}
			if err != nil || bytes.Equal(output, input) {
				t.Fatal("original lexical status was not restored", err)
			}
			again, err := authority.restoreEnumAssertionOwner(original, output, nil)
			if err != nil || !bytes.Equal(output, again) {
				t.Fatal("cache projection not idempotent", err)
			}
		})
	}
}

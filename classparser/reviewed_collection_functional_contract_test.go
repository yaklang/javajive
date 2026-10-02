package javaclassparser

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

type reviewedCollectionSAM struct {
	name, erased, instantiated, owner, implementation string
	kind                                              uint8
}

// The instantiated descriptor is independent of the generated lambda spelling.
// In particular a typed SAM carrier is valid even when passed to a raw receiver.
func reviewedCollectionNative(t *testing.T, jar, entry string, want []reviewedCollectionSAM, check func(string)) {
	t.Helper()
	raw := originalJarClassForReview(t, jar, entry)
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, contract := range want {
		found := false
		for _, attr := range object.Attributes {
			b, ok := attr.(*BootstrapMethodsAttribute)
			if !ok {
				continue
			}
			for _, method := range b.BootstrapMethods {
				if len(method.BootstrapArguments) < 3 {
					continue
				}
				bootstrap, ok := cp.IndexInfo(int(method.BootstrapMethodRef)).(*ConstantMethodHandleInfo)
				if !ok {
					continue
				}
				bootstrapRef, ok := cp.IndexInfo(int(bootstrap.ReferenceIndex)).(*ConstantMethodrefInfo)
				if !ok || cp.GetClassName(int(bootstrapRef.ClassIndex)) != "java/lang/invoke/LambdaMetafactory" {
					continue
				}
				handle, ok := cp.IndexInfo(int(method.BootstrapArguments[1])).(*ConstantMethodHandleInfo)
				if !ok {
					continue
				}
				var member ConstantMemberrefInfo
				switch reference := cp.IndexInfo(int(handle.ReferenceIndex)).(type) {
				case *ConstantMethodrefInfo:
					member = reference.ConstantMemberrefInfo
				case *ConstantInterfaceMethodrefInfo:
					member = reference.ConstantMemberrefInfo
				default:
					continue
				}
				pair, ok := cp.IndexInfo(int(member.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
				if !ok || cp.GetUtf8(int(pair.NameIndex)).Value != contract.name {
					continue
				}
				sam, ok := cp.IndexInfo(int(method.BootstrapArguments[0])).(*ConstantMethodTypeInfo)
				if !ok {
					continue
				}
				inst, ok := cp.IndexInfo(int(method.BootstrapArguments[2])).(*ConstantMethodTypeInfo)
				if !ok {
					continue
				}
				if cp.GetUtf8(int(sam.DescriptorIndex)).Value == contract.erased && cp.GetUtf8(int(inst.DescriptorIndex)).Value == contract.instantiated && cp.GetClassName(int(member.ClassIndex)) == contract.owner && cp.GetUtf8(int(pair.DescriptorIndex)).Value == contract.implementation && handle.ReferenceKind == contract.kind {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("original bootstrap contract absent: %+v", contract)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_HARDJAR_SHAPE_OFF", setting)
		fs, err := NewJarFSFromLocal(filepath.Join(home, ".m2/repository", jar))
		if err != nil {
			t.Fatal(err)
		}
		source, err := fs.ReadFile(entry)
		fs.Close()
		if err != nil {
			t.Fatal(err)
		}
		check(string(source))
	}
}

func reviewedCollectionCarrier(t *testing.T, source, typ string) string {
	t.Helper()
	match := requireReviewedPattern(t, source, regexp.QuoteMeta(typ)+`\s+(\w+)\s*=`)
	return regexp.QuoteMeta(match[1])
}
func reviewedCollectionCarrierUse(t *testing.T, source, typ, call string) {
	t.Helper()
	id := reviewedCollectionCarrier(t, source, typ)
	requireReviewedPattern(t, source, regexp.QuoteMeta(call)+`\([^;\n]*\b`+id+`\b`)
}

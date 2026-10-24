package javaclassparser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const sourceFieldMetadataFixture = `class MetadataStorage<T> {
 @Deprecated private transient int memo;
 private transient volatile Object cached;
 private transient boolean seen;
 private T value;
 static final long serialVersionUID=7L;
 int read(int x){memo=x*31+17;return memo;}
}`

func metadataStorageOriginal(t *testing.T, raw []byte, encoding string) []byte {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, field := range object.Fields {
		if field.AccessFlags&0x82 != 0x82 {
			continue
		}
		if encoding != "attribute" {
			field.AccessFlags |= syntheticFieldFlag
		}
		if encoding != "flag" {
			object.ConstantPoolManager.AddUtf8Info("Synthetic")
			field.Attributes = append(field.Attributes, &SyntheticAttribute{})
		}
		changed++
	}
	if changed != 3 {
		t.Fatal("fixture must contain three ordinary storage fields", changed)
	}
	return object.Bytes()
}

func TestNativeSourceFieldMetadataRestoresOnlyOriginalMarkerBytes(t *testing.T) {
	for _, root := range []string{"MetadataStorage", "UnrelatedStorage"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			compiled := nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": strings.ReplaceAll(sourceFieldMetadataFixture, "MetadataStorage", root)}, debug, "8")[root+".class"]
			for _, encoding := range []string{"flag", "attribute", "both"} {
				t.Run(root+"/"+debug+"/"+encoding, func(t *testing.T) {
					original := metadataStorageOriginal(t, compiled, encoding)
					originalBefore, compiledBefore := bytes.Clone(original), bytes.Clone(compiled)
					result, err := RestoreSyntheticFieldMetadata(original, compiled, nil)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(original, originalBefore) || !bytes.Equal(compiled, compiledBefore) || len(result) != len(compiled) {
						t.Fatal("input mutated or output resized")
					}
					changes := 0
					for i, b := range result {
						if b != compiled[i] {
							if b != compiled[i]|0x10 || compiled[i]&0x10 != 0 {
								t.Fatal("only one original Synthetic flag bit may change", i)
							}
							changes++
						}
					}
					if changes != 3 {
						t.Fatal("missing or excessive restored field markers", changes)
					}
					before, err := Parse(compiled)
					if err != nil {
						t.Fatal(err)
					}
					after, err := Parse(result)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before.ConstantPool, after.ConstantPool) || !reflect.DeepEqual(before.Methods, after.Methods) || !reflect.DeepEqual(before.Attributes, after.Attributes) {
						t.Fatal("compiled method, constant pool or class metadata was changed")
					}
					for i, field := range before.Fields {
						other := after.Fields[i]
						if field.NameIndex != other.NameIndex || field.DescriptorIndex != other.DescriptorIndex || !reflect.DeepEqual(field.Attributes, other.Attributes) || field.AccessFlags&^syntheticFieldFlag != other.AccessFlags&^syntheticFieldFlag {
							t.Fatal("compiled field binding, attributes or ordinary modifiers changed")
						}
					}
					second, err := RestoreSyntheticFieldMetadata(original, result, nil)
					if err != nil || !bytes.Equal(second, result) {
						t.Fatal("restoration is not idempotent", err)
					}
				})
			}
		}
	}
}

func TestNativeSourceFieldMetadataRejectsChangedDeclarationUniverse(t *testing.T) {
	compiled := nativeCompileClasses(t, sourceFieldMetadataFixture)["MetadataStorage.class"]
	original := metadataStorageOriginal(t, compiled, "flag")
	for _, mutation := range []string{"class identity", "missing", "extra", "descriptor", "name", "private", "transient", "volatile", "unexpected synthetic", "duplicate original", "duplicate compiled", "malformed original", "truncated compiled"} {
		t.Run(mutation, func(t *testing.T) {
			authority, _ := Parse(original)
			candidate, _ := Parse(compiled)
			switch mutation {
			case "class identity":
				candidate.ThisClass = candidate.SuperClass
			case "missing":
				candidate.Fields = candidate.Fields[:len(candidate.Fields)-1]
			case "extra":
				candidate.Fields = append(candidate.Fields, &MemberInfo{AccessFlags: 2, NameIndex: candidate.Fields[4].NameIndex, DescriptorIndex: candidate.Fields[0].DescriptorIndex})
			case "descriptor":
				candidate.Fields[0].DescriptorIndex = candidate.Fields[1].DescriptorIndex
			case "name":
				candidate.Fields[0].NameIndex = candidate.Fields[1].NameIndex
			case "private":
				candidate.Fields[0].AccessFlags &^= 2
			case "transient":
				candidate.Fields[0].AccessFlags &^= 0x80
			case "volatile":
				candidate.Fields[1].AccessFlags &^= 0x40
			case "unexpected synthetic":
				candidate.Fields[3].AccessFlags |= syntheticFieldFlag
			case "duplicate original":
				authority.Fields = append(authority.Fields, authority.Fields[0])
			case "duplicate compiled":
				candidate.Fields = append(candidate.Fields, candidate.Fields[0])
			}
			a, b := authority.Bytes(), candidate.Bytes()
			if mutation == "malformed original" {
				a[0] = 0
			}
			if mutation == "truncated compiled" {
				b = b[:len(b)-1]
			}
			ac, bc := bytes.Clone(a), bytes.Clone(b)
			got, err := RestoreSyntheticFieldMetadata(a, b, nil)
			if err == nil || got != nil || !bytes.Equal(a, ac) || !bytes.Equal(b, bc) {
				t.Fatal("changed declaration admitted or partial output exposed", err)
			}
		})
	}
}

func TestNativeSourceFieldMetadataRetainsSharedResourceAndCancellationLimits(t *testing.T) {
	compiled := nativeCompileClasses(t, sourceFieldMetadataFixture)["MetadataStorage.class"]
	original := metadataStorageOriginal(t, compiled, "flag")
	for _, limit := range []string{"input", "read", "parse", "scan", "output", "allocation", "canceled"} {
		t.Run(limit, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			limits := Limits{}
			switch limit {
			case "input":
				limits.MaxInputBytes = int64(len(original) + len(compiled) - 1)
			case "read":
				limits.MaxReadBytes = 1
			case "parse":
				limits.MaxParseItems = 1
			case "scan":
				limits.MaxGraphScans = 1
			case "output":
				limits.MaxOutputBytes = int64(len(compiled) - 1)
			case "allocation":
				limits.MaxOutputBytes = 1
			case "canceled":
				cancel()
			}
			budget := workbudget.New(ctx, limits)
			got, err := RestoreSyntheticFieldMetadata(original, compiled, budget)
			var denied *workbudget.Error
			if err == nil || got != nil || !errors.As(err, &denied) {
				t.Fatal("shared denial was lost or class output escaped", err)
			}
		})
	}
}

func TestNativeSourceFieldMetadataUsesFieldIdentityAndKeepsCompiledMethods(t *testing.T) {
	base := nativeCompileClasses(t, sourceFieldMetadataFixture)["MetadataStorage.class"]
	differentCode := strings.Replace(sourceFieldMetadataFixture, "x*31+17", "x*17+31", 1)
	compiled := nativeCompileClasses(t, differentCode)["MetadataStorage.class"]
	candidate, err := Parse(compiled)
	if err != nil {
		t.Fatal(err)
	}
	// Class-file declaration order and constant-pool order are not field
	// identities. Reordering must not patch the old field's byte position.
	for i, j := 0, len(candidate.Fields)-1; i < j; i, j = i+1, j-1 {
		candidate.Fields[i], candidate.Fields[j] = candidate.Fields[j], candidate.Fields[i]
	}
	compiled = candidate.Bytes()
	original := metadataStorageOriginal(t, base, "flag")
	result, err := RestoreSyntheticFieldMetadata(original, compiled, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := Parse(original)
	after, _ := Parse(result)
	if reflect.DeepEqual(authority.Methods, candidate.Methods) || !reflect.DeepEqual(after.Methods, candidate.Methods) || !reflect.DeepEqual(after.ConstantPool, candidate.ConstantPool) {
		t.Fatal("restoration substituted original executable code or constant-pool bindings")
	}
	fields, err := sourceFieldIdentities(after, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := sourceFieldIdentities(authority, nil)
	if err != nil || !reflect.DeepEqual(fields, expected) {
		t.Fatal("declaration-order change misbound restored metadata", err)
	}
}

func TestNativeSourceFieldMetadataAcceptsDenseMinimalFieldTables(t *testing.T) {
	var source strings.Builder
	source.WriteString("class DenseMetadata {")
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&source, "private transient int field%d;", i)
	}
	source.WriteString("}")
	compiled := nativeCompileClasses(t, source.String())["DenseMetadata.class"]
	original, err := Parse(compiled)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range original.Fields {
		if len(field.Attributes) != 0 {
			t.Fatal("fixture needs minimal eight-byte field_info entries")
		}
		field.AccessFlags |= syntheticFieldFlag
	}
	result, err := RestoreSyntheticFieldMetadata(original.Bytes(), compiled, nil)
	if err != nil {
		t.Fatal("metadata map allocation must not enlarge the encoded minimum", err)
	}
	after, err := Parse(result)
	if err != nil || len(after.Fields) != 64 {
		t.Fatal("field universe changed", err)
	}
	for _, field := range after.Fields {
		if field.AccessFlags != 0x1082 {
			t.Fatal("dense field metadata missed", field.AccessFlags)
		}
	}
}

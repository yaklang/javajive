package dexparser

import (
	"encoding/binary"
	"hash/adler32"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func TestParseWithOptionsToleratesSignatureMismatchWhenChecksumValid(t *testing.T) {
	staleSHA1 := testDEXWithMapItem(0xf000)
	// Mutate SHA-1 digest while keeping Adler-32 over data[12:] valid, matching
	// tampered/repacked crackme DEX headers.
	staleSHA1[12] ^= 0xff
	putTestU32(staleSHA1, 8, adler32.Checksum(staleSHA1[12:]))

	if _, err := Parse(staleSHA1, "classes.dex"); err == nil || !strings.Contains(err.Error(), "SHA-1 signature mismatch") {
		t.Fatalf("strict Parse(stale SHA-1) error = %v, want SHA-1 signature mismatch", err)
	}

	recovered, err := ParseWithOptions(staleSHA1, "classes.dex", ParseOptions{TolerateSignatureMismatch: true})
	if err != nil {
		t.Fatalf("ParseWithOptions(TolerateSignatureMismatch=true) failed: %v", err)
	}
	if len(recovered.ParseWarnings) != 1 || recovered.ParseWarnings[0].Code != "header.signature_mismatch" {
		t.Fatalf("ParseWarnings = %#v, want single header.signature_mismatch warning", recovered.ParseWarnings)
	}

	badAdler := append([]byte(nil), staleSHA1...)
	putTestU32(badAdler, 8, binary.LittleEndian.Uint32(badAdler[8:12])^1)
	if _, err := ParseWithOptions(badAdler, "classes.dex", ParseOptions{TolerateSignatureMismatch: true}); err == nil || !strings.Contains(err.Error(), "Adler-32 checksum=") {
		t.Fatalf("ParseWithOptions(bad Adler-32) error = %v, want Adler-32 checksum failure", err)
	}
}

func TestTranslateSystemAnnotationsToleratesOrphanAnnotationDefault(t *testing.T) {
	const owner = "LT0/f;"
	makeClass := func() model.Class {
		return model.Class{
			Descriptor: owner,
			VirtualMethods: []model.EncodedMethod{{
				MethodIndex: 7,
				Method:      model.MethodID{Class: owner, Name: "value"},
			}},
			Annotations: []model.Annotation{{
				Visibility: model.AnnotationSystem,
				Type:       "Ldalvik/annotation/AnnotationDefault;",
				Elements: []model.AnnotationElement{{
					Name: "value",
					Value: model.EncodedValue{Type: model.ValueAnnotation, Annotation: &model.EncodedAnnotation{
						Type: owner,
						Elements: []model.AnnotationElement{
							{Name: "value", Value: model.EncodedValue{Type: model.ValueInt, Signed: 7}},
							{Name: "intEncoding", Value: model.EncodedValue{Type: model.ValueInt, Signed: 1}},
						},
					}},
				}},
			}},
		}
	}

	strictFile := &model.File{Entry: "classes.dex"}
	strictClass := makeClass()
	strictParser := &parser{file: strictFile}
	if err := strictParser.translateSystemAnnotations(&strictClass); err == nil || !strings.Contains(err.Error(), `AnnotationDefault element "intEncoding" has no annotation method`) {
		t.Fatalf("strict translateSystemAnnotations error = %v, want orphan annotation method failure", err)
	}

	recoverFile := &model.File{Entry: "classes.dex"}
	recoverClass := makeClass()
	recoverParser := &parser{file: recoverFile, options: ParseOptions{TolerateOrphanAnnotationDefault: true}}
	if err := recoverParser.translateSystemAnnotations(&recoverClass); err != nil {
		t.Fatalf("recover translateSystemAnnotations failed: %v", err)
	}
	if len(recoverFile.ParseWarnings) != 1 || recoverFile.ParseWarnings[0].Code != "annotation.orphan_default" {
		t.Fatalf("ParseWarnings = %#v, want single annotation.orphan_default warning", recoverFile.ParseWarnings)
	}
	if len(recoverClass.MethodAnnotations) != 1 || recoverClass.MethodAnnotations[0].MethodIndex != 7 {
		t.Fatalf("MethodAnnotations = %#v, want valid 'value' default preserved", recoverClass.MethodAnnotations)
	}
	gotDefault := recoverClass.MethodAnnotations[0].Annotations[0].Elements[0].Value
	if gotDefault.Type != model.ValueInt || gotDefault.Signed != 7 {
		t.Fatalf("preserved default = %#v, want int 7", gotDefault)
	}
}

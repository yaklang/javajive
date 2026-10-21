package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialSourceCompilerMetadataAdmissionRequiresOriginalFactoryRole(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("original native8 compiler")
	}
	files := compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, sourceLayoutSubsetFixture, "LayoutGood", "none", true)
	for _, variant := range []string{"modern", "unspecified profile", "native8", "native8 wrong target", "artifact", "artifact foreign bytes", "artifact resolver only", "no implementation table", "not synthetic", "unused factory metadata", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["LayoutOpaque.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var implementation *MemberInfo
			for _, method := range object.Methods {
				if method.AccessFlags == 0x100a {
					if implementation != nil {
						t.Fatal("fixture implementation not unique")
					}
					implementation = method
				}
			}
			metadata, known := readLambdaParameterMetadata(object, implementation, nil)
			if !known || metadata == nil || !nativeLambdaBodyOriginalUseClosed(object, implementation, nil, metadata) || metadata.factory == nil {
				t.Fatal("original physical role/factory missing")
			}
			profile, target := ModernJavac, 8
			var work *workbudget.Budget
			var authorities []*sourceLambdaMetadataAuthority
			eligible := false
			switch variant {
			case "unspecified profile":
				profile = ""
			case "native8":
				profile, eligible = NativeJavac8, true
			case "native8 wrong target":
				profile, target = NativeJavac8, 7
			case "artifact", "artifact foreign bytes", "artifact resolver only":
				originals := map[string][]byte{}
				for name, raw := range files {
					originals[name] = bytes.Clone(raw)
				}
				if variant == "artifact foreign bytes" {
					other, err := Parse(originals["LayoutOpaque.class"])
					if err != nil {
						t.Fatal(err)
					}
					other.AccessFlags ^= 0x10
					originals["LayoutOpaque.class"] = other.Bytes()
				}
				if variant == "artifact resolver only" {
					delete(originals, "LayoutOpaque.class")
				}
				archive := nativeArchive(t, originals)
				defer archive.Close()
				archive.declarationResolver = func(name string) ([]byte, bool) { raw, found := files[name+".class"]; return raw, found }
				authorities = []*sourceLambdaMetadataAuthority{{jar: archive}}
				eligible = variant == "artifact"
			case "no implementation table":
				for i, attribute := range implementation.Attributes {
					if attribute == metadata.table {
						implementation.Attributes = append(implementation.Attributes[:i], implementation.Attributes[i+1:]...)
						break
					}
				}
				eligible = true
			case "not synthetic":
				implementation.AccessFlags &^= 0x1000
				eligible = true
			case "unused factory metadata":
				// A valid, unused bootstrap packet and matching synthetic name do
				// not prove a source lambda. Remove the only actual factory method;
				// keep the implementation, original schema, handle and bootstrap.
				methods := object.Methods[:0]
				for _, method := range object.Methods {
					if method != metadata.factory.method {
						methods = append(methods, method)
					}
				}
				object.Methods = methods
				if fresh, valid := readLambdaParameterMetadata(object, implementation, nil); !valid || fresh == nil || nativeLambdaBodyOriginalUseClosed(object, implementation, nil, fresh) {
					t.Fatal("false friend did not remove the actual factory role")
				}
				eligible = true
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := object.Bytes()
			family := &nativeMemberFamily{owner: object.GetClassName(), lexicalObjects: map[string]*ClassObject{object.GetClassName(): object}}
			if got := nativeMemberCompilerMetadataEligible(object, family, profile, target, work, authorities...); got != eligible || !bytes.Equal(before, object.Bytes()) {
				t.Fatalf("local eligibility=%v wanted=%v, original input must stay unchanged", got, eligible)
			}
		})
	}
}

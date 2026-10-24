package javaclassparser

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestConstructorBindingPublicationRejectsStaleWriterAndOriginal(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, constructorBindingPublicationFixture, "SuffixOwner", "none", false)
	const owner = "SuffixOwner$Member"
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return bytes.Clone(b), k })
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	for _, variant := range []string{"control", "missing span", "shifted span", "changed cached signature", "changed cached code", "changed published code", "changed descriptor", "foreign member", "foreign publisher", "missing authority", "changed original capture"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files[owner+".class"]))
			d := artifact.source.jar.nativeMemberReader(obj)
			d.options.sourceConstructorLowering = artifact.source.jar.sourceConstructorLowering
			source, err := d.DumpClass()
			if err != nil || legacySourceHasFailureMarker(source) {
				t.Fatal("actual declaration publisher", err, source)
			}
			var spans []ordinaryConstructorMethodSourceSpan
			var bound *dumpedMethods
			for _, m := range d.dumpedMethodsSet {
				if m == nil || m.code == "" || m.sourceRequest.role != methodSourceDeclaration || strings.Count(source, m.code) != 1 {
					continue
				}
				spans = append(spans, ordinaryConstructorMethodSourceSpan{m, strings.Index(source, m.code)})
				if m.constructorBindingSignature != "" {
					if bound != nil || m.constructorLowering != nil || m.ordinaryConstructorSource != nil || m.genericFunctionalSource != nil {
						t.Fatal("Signature-only publication must not borrow another receipt")
					}
					bound = m
				}
			}
			if bound == nil {
				t.Fatal("actual binding publication absent")
			}
			sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
			methods := make([]*dumpedMethods, len(spans))
			for i, p := range spans {
				methods[i] = p.method
			}
			switch variant {
			case "missing span":
				spans = spans[1:]
			case "shifted span":
				spans[0].start++
			case "changed cached signature", "changed cached code":
				copy := *bound
				if variant == "changed cached signature" {
					copy.constructorBindingSignature += "X"
				} else {
					copy.code += "/* stale */"
				}
				d.dumpedMethodsSet[bound.sourceCacheKey] = &copy
			case "changed published code":
				bound.constructorBindingCode += "/* stale */"
			case "changed descriptor":
				bound.descriptor = "()V"
			case "foreign member":
				copy := *bound.member
				bound.member = &copy
			case "foreign publisher":
				bound.sourceDumper = NewClassObjectDumper(obj)
			case "missing authority":
				d.options.sourceConstructorLowering = nil
			case "changed original capture":
				code := constructorLoweringCode(bound.member)
				if code == nil || len(code.Code) < 2 || code.Code[1] != 0x2b {
					t.Fatal("original enclosing load")
				}
				code.Code[1] = 0x2c
			}
			receipt, err := d.newOrdinaryConstructorClassSource(source, methods, spans)
			if variant != "control" {
				if err == nil || receipt != nil {
					t.Fatal("stale constructor binding gained class-rewrite authority")
				}
				return
			}
			if err != nil || receipt == nil || !receipt.finish(source) {
				t.Fatal("missing binding source frame", err)
			}
			d.ordinaryConstructorClassSource = receipt
			result := d.sourceRewrite("binding-frame-test", "class_source", source, func(body string) string { return "/* independent body */" + body })
			if result == source || strings.Count(result, bound.code) != 1 || !receipt.finish(result) {
				t.Fatal("constructor changed or independent methods became uneditable")
			}
			if receipt.finish(result + "\nclass Shadow {}") {
				t.Fatal("untracked declaration environment accepted")
			}
		})
	}
}

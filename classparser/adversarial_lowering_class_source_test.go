package javaclassparser

import (
	"bytes"
	"sort"
	"strings"
	"testing"
)

// Use real publisher receipts, with no library names or source-shape repair.
// A class-level rule must not change a sealed SUPER binding/capture schedule,
// nor its type/field environment, even when an unsealed body can be edited.
func TestAdversarialConstructorLoweringClassRecoveryKeepsProofBoundaries(t *testing.T) {
	original, _, artifact := loweringAuthorityTestInputs(t)
	setup := func(t *testing.T) (*ClassObjectDumper, string, []*dumpedMethods, []ordinaryConstructorMethodSourceSpan) {
		t.Helper()
		obj := mustParseLowering(t, bytes.Clone(original))
		d := artifact.source.jar.nativeMemberReader(obj)
		d.options.sourceConstructorLowering = artifact.source.jar.sourceConstructorLowering
		text, err := d.DumpClass()
		if err != nil || legacySourceHasFailureMarker(text) {
			t.Fatal("actual lowering publisher", err, text)
		}
		var spans []ordinaryConstructorMethodSourceSpan
		bound := 0
		for _, m := range d.dumpedMethodsSet {
			if m == nil || m.code == "" || m.sourceRequest.role != methodSourceDeclaration || strings.Count(text, m.code) != 1 {
				continue
			}
			spans = append(spans, ordinaryConstructorMethodSourceSpan{m, strings.Index(text, m.code)})
			if m.constructorLowering != nil {
				bound++
			}
		}
		if bound != 1 {
			t.Fatal("expected original lowering certificate", bound)
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
		methods := make([]*dumpedMethods, len(spans))
		for i, p := range spans {
			methods[i] = p.method
		}
		return d, text, methods, spans
	}
	t.Run("sealed constructor and binding environment", func(t *testing.T) {
		d, source, methods, spans := setup(t)
		receipt, err := d.newOrdinaryConstructorClassSource(source, methods, spans)
		if err != nil || receipt == nil || !receipt.finish(source) {
			t.Fatal("missing constructor lowering frame", err)
		}
		d.ordinaryConstructorClassSource = receipt
		var ctor *dumpedMethods
		for _, m := range methods {
			if m.constructorLowering != nil {
				ctor = m
			}
		}
		rule := func(s string) string {
			s = strings.ReplaceAll(s, "super(", "this(")
			s = strings.ReplaceAll(s, "final Object capturedToken", "final String capturedToken")
			s = strings.ReplaceAll(s, "extends LoweringParent", "extends ChangedParent")
			return strings.ReplaceAll(s, "31L", "29L")
		}
		if rule(source) == source || rule(ctor.code) == ctor.code {
			t.Fatal("inactive independent mutation")
		}
		result := d.sourceRewrite("lowering-proof-test", "class_source", source, rule)
		if strings.Count(result, ctor.code) != 1 || result != strings.ReplaceAll(source, "31L", "29L") || !receipt.finish(result) {
			t.Fatal("frame changed constructor or froze independent body", result)
		}
		if receipt.finish(result + "\nclass ChangedParent {}") {
			t.Fatal("accepted untracked binding environment")
		}
	})
	for _, variant := range []string{"missing span", "changed packet", "changed cached receipt", "changed cached code", "changed descriptor", "lost plan", "foreign publisher"} {
		t.Run(variant, func(t *testing.T) {
			d, source, methods, spans := setup(t)
			var bound *dumpedMethods
			for _, m := range methods {
				if m.constructorLowering != nil {
					bound = m
				}
			}
			switch variant {
			case "missing span":
				spans = spans[1:]
			case "changed packet":
				bound.constructorLoweringCode += "/* stale */"
			case "changed cached receipt":
				copy := *d.dumpedMethodsSet[bound.sourceCacheKey]
				plan := *bound.constructorLowering
				copy.constructorLowering = &plan
				d.dumpedMethodsSet[bound.sourceCacheKey] = &copy
			case "changed cached code":
				copy := *d.dumpedMethodsSet[bound.sourceCacheKey]
				copy.code += "/* stale */"
				d.dumpedMethodsSet[bound.sourceCacheKey] = &copy
			case "changed descriptor":
				bound.descriptor = "()V"
			case "lost plan":
				delete(d.constructorLowerings, bound.descriptor)
			case "foreign publisher":
				bound.sourceDumper = NewClassObjectDumper(d.obj)
			}
			if receipt, err := d.newOrdinaryConstructorClassSource(source, methods, spans); err == nil || receipt != nil {
				t.Fatal("stale lowering publication gained rewrite authority")
			}
		})
	}
}

package javaclassparser

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// This unit harness retains packets from the real method publisher. Production
// coordinates come from assemble's actual writes; the whole-unit JVM regression
// exercises that wiring independently. Here locating those known declarations
// gives the lower-level frame checker deliberate malformed span inputs as well.
func ordinaryConstructorClassFrameSetup(t *testing.T, files map[string][]byte) (*ClassObjectDumper, string, []*dumpedMethods, []ordinaryConstructorMethodSourceSpan) {
	t.Helper()
	obj, err := Parse(bytes.Clone(files["ObserveFactory.class"]))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	d.options = DecompileOptions{Mode: Compatibility, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
	d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }
	source, err := d.DumpClass()
	if err != nil || strings.Contains(source, DecompileStubMarker) {
		t.Fatalf("actual publisher: %v\n%s", err, source)
	}
	var spans []ordinaryConstructorMethodSourceSpan
	bound := 0
	for _, method := range d.dumpedMethodsSet {
		if method == nil || method.code == "" || method.sourceRequest.role != methodSourceDeclaration || strings.Count(source, method.code) != 1 {
			continue
		}
		spans = append(spans, ordinaryConstructorMethodSourceSpan{method: method, start: strings.Index(source, method.code)})
		if method.ordinaryConstructorSource != nil {
			bound++
		}
	}
	if bound != 1 {
		t.Fatalf("expected one actual bound declaration, got %d", bound)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	methods := make([]*dumpedMethods, len(spans))
	for i, span := range spans {
		methods[i] = span.method
	}
	return d, source, methods, spans
}

func TestAdversarialOrdinaryConstructorClassRecoveryKeepsDeclarationBindings(t *testing.T) {
	files := nativeCompileDebugClasses(t, ordinaryConstructorLifecycleFixture, "none")
	setup := func(t *testing.T) (*ClassObjectDumper, string, *ordinaryConstructorClassSourceReceipt, []*dumpedMethods, []ordinaryConstructorMethodSourceSpan) {
		t.Helper()
		d, source, methods, spans := ordinaryConstructorClassFrameSetup(t, files)
		s, err := d.newOrdinaryConstructorClassSource(source, methods, spans)
		if err != nil || s == nil || !s.finish(source) {
			t.Fatal("actual class frame", err)
		}
		d.ordinaryConstructorClassSource = s
		return d, source, s, methods, spans
	}
	t.Run("unrelated body remains editable without borrowing a sibling context", func(t *testing.T) {
		d, source, s, _, _ := setup(t)
		calls := 0
		rule := func(text string) string {
			calls++
			text = strings.ReplaceAll(text, "ObserveBase(", "Object(")
			text = strings.ReplaceAll(text, "static int field", "static int capturedName")
			return strings.ReplaceAll(text, "visible", "updated")
		}
		if rule(source) == source {
			t.Fatal("mutant control did not change the whole source")
		}
		calls = 0
		d.FuncCtx.FunctionName, d.FuncCtx.CurrentMethodDesc = "otherMethod", "()V"
		out := d.sourceRewrite("ordinary-frame-test", "class_source", source, rule)
		if calls != 1 || out != strings.Replace(source, "visible", "updated", 1) || !s.finish(out) {
			t.Fatalf("frame leaked a declaration or froze the unrelated body: calls=%d\n%s", calls, out)
		}
		if s.finish(out + "\nclass Shadow {}") {
			t.Fatal("untracked class-level binding change was published")
		}
	})
	t.Run("cycle rejection does not commit a draft", func(t *testing.T) {
		d, source, s, _, _ := setup(t)
		d.report = &DecompileResult{}
		next := d.sourceRewrite("first", "class_source", source, func(x string) string { return strings.ReplaceAll(x, "visible", "updated") })
		if next == source || !s.finish(next) {
			t.Fatal("forward draft was not committed")
		}
		if out := d.sourceRewrite("reverse", "class_source", next, func(x string) string { return strings.ReplaceAll(x, "updated", "visible") }); out != next || !s.finish(next) {
			t.Fatal("rejected cycle consumed or changed the current frame")
		}
	})
	t.Run("trusted retry cache copy keeps the same consumed publication", func(t *testing.T) {
		d, source, s, _, _ := setup(t)
		restore := d.snapshotRetryState()
		d.FuncCtx.FunctionName = "discarded-retry"
		restore()
		if !s.finish(source) {
			t.Fatal("copying a cache container revoked an unchanged original publication")
		}
	})
	for _, replacement := range []string{"} class Shadow {} {", `return "unterminated;`, "// no closing newline", `\u007d class Shadow {`, "{", ")"} {
		t.Run("reject body escape "+replacement, func(t *testing.T) {
			d, source, s, _, _ := setup(t)
			if out := d.sourceRewrite("escape", "class_source", source, func(string) string { return replacement }); out != source || !s.finish(source) {
				t.Fatal("an unsealed body escaped its actual writer port")
			}
		})
	}
	for _, variant := range []string{"shifted span", "missing span", "reordered spans", "stale body range", "changed packet", "stale original Code", "wrong saved context"} {
		t.Run(variant, func(t *testing.T) {
			d, source, s, methods, spans := setup(t)
			bound := s.bound[0]
			switch variant {
			case "shifted span":
				spans[0].start++
			case "missing span":
				spans = spans[1:]
			case "reordered spans":
				spans[0], spans[1] = spans[1], spans[0]
			case "stale body range":
				for _, m := range methods {
					if m.methodName == "unbound" {
						m.sourceBodyStart++
						break
					}
				}
			case "changed packet":
				copy := *bound
				copy.code += "/*stale cached source*/"
				d.dumpedMethodsSet[bound.sourceCacheKey] = &copy
			case "stale original Code":
				bound.ordinaryConstructorSource.code.MaxStack++
			case "wrong saved context":
				bound.ordinaryConstructorSource.sourceContext.FunctionName = "borrowed"
			}
			if receipt, err := d.newOrdinaryConstructorClassSource(source, methods, spans); err == nil || receipt != nil {
				t.Fatal("stale producer/class facts manufactured a new frame")
			}
		})
	}
	t.Run("shared budget is not reset for body recovery", func(t *testing.T) {
		d, source, s, _, _ := setup(t)
		d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
		calls := 0
		if out := d.sourceRewrite("bounded", "class_source", source, func(string) string { calls++; return "return null;" }); out != source || calls != 0 || d.Work.Err() == nil || s.finish(source) {
			t.Fatal("body recovery escaped cancellation/work admission")
		}
	})
}

func TestAdversarialOrdinaryConstructorSourceBodyPortClosure(t *testing.T) {
	for _, source := range []string{"", `return "}[](";`, "// }\nreturn value;", `Runnable r=()->{return;};`, "{ int x=1; }"} {
		if !ordinaryConstructorBodyScopeClosed(source) {
			t.Errorf("valid lexical body rejected: %q", source)
		}
	}
	for _, source := range []string{"}", "{", ")", "]", `\u007d`, `"""text"""`, "// no newline", "/* unclosed", `return "unclosed;`, "}; class Captured {} {"} {
		if ordinaryConstructorBodyScopeClosed(source) {
			t.Errorf("body escaped its lexical port: %q", source)
		}
	}
}

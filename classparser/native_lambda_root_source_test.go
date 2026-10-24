package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Planning the root's lambda does not consume any source. Its final recorder
// must bind the exact root object, original method, Code and factory operands.
func TestNativeLambdaRootLocalCaptureRequiresOwnSourceReceipt(t *testing.T) {
	files := nativeCompileClasses(t, nativeLambdaSingleLifetimeFixture())
	for _, variant := range []string{"original", "foreign root object", "wrong root owner", "missing root view", "missing family", "member precedence", "anonymous precedence", "foreign method", "foreign code", "wrong arity", "duplicate source", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(files["BridgeHandleRoot.class"])
			if err != nil {
				t.Fatal(err)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			prepared := z.prepareNativeMemberFamilyUnpublished(root, snapshotJDECEnv())
			if prepared == nil || prepared.family.rootHandleSource == nil {
				t.Fatal("original root lambda plan")
			}
			view := prepared.family.rootHandleSource
			if len(view.lambdaContext.localCaptures) != 1 {
				t.Fatal("original local factory required")
			}
			var key string
			var site *nativeLambdaLocalCaptureSite
			for name, candidate := range view.lambdaContext.localCaptures {
				key, site = name, candidate
			}
			var name, desc string
			for method := range view.lambdaImplementations {
				name, _ = sourceBridgeUTF8(root, method.NameIndex)
				desc, _ = sourceBridgeUTF8(root, method.DescriptorIndex)
			}
			if name+desc != key {
				t.Fatal("original implementation identity")
			}
			d := NewClassObjectDumper(root)
			d.nativeMemberRoot, d.CurrentMethod, d.FuncCtx = prepared.family, site.method, &class_context.ClassContext{}
			code := site.code
			captured := make([]values.JavaValue, len(site.operands))
			for i := range captured {
				captured[i] = values.JavaNull
			}
			switch variant {
			case "foreign root object":
				copy := *root
				d.obj = &copy
			case "wrong root owner":
				prepared.family.owner = "Foreign"
			case "missing root view":
				prepared.family.rootHandleSource = nil
			case "missing family":
				d.nativeMemberRoot = nil
			case "member precedence":
				d.nativeMemberCurrent = &nativeMemberClass{}
			case "anonymous precedence":
				d.nativeAnonymousLambdaCurrent = &nativeMemberClass{}
			case "foreign method":
				copy := *site.method
				d.CurrentMethod = &copy
			case "foreign code":
				copy := *code
				code = &copy
			case "wrong arity":
				captured = nil
			case "duplicate source":
				d.nativeLambdaLocalSources = map[string]*nativeLambdaLocalCaptureSource{key: {}}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				// The one operand costs one scan; consume the shared allowance
				// before this recorder, as an earlier planning phase can do.
				if !nativeProofWork(d.Work, 1) {
					t.Fatal("initial shared work allowance")
				}
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			d.recordNativeLambdaLocalCaptureSource(name, desc, code, captured)
			receipt := d.nativeLambdaLocalSources[key]
			bound := receipt != nil && receipt.method == site.method && receipt.code == site.code && receipt.factoryPC == site.pc && receipt.hasFactoryPC
			if bound != (variant == "original") {
				t.Fatalf("exact original factory receipt=%v", bound)
			}
			if nativeLambdaLocalCaptureSourceClosed(site, receipt, nil) {
				t.Fatal("unrendered/fake captured values cannot close the source proof")
			}
			if d.Work != nil && d.Work.Err() == nil {
				t.Fatal("bounded refusal must be sticky")
			}
		})
	}
}

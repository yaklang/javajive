package javaclassparser

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialParameterNamespaceFailureDoesNotCertifySiblingFailure(t *testing.T) {
	raw := methodParameterNameGuardClass(t)
	for _, variant := range []string{"valid sibling", "opaque sibling", "changed header", "work", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, _, params := methodParameterNameGuardInput(t, raw)
			names, known := d.originalOrdinaryMethodParameterNames(method, "(JI)J")
			if !known {
				t.Fatal("original names")
			}
			binding, known := d.bindOrdinaryMethodParameterNames(method, "(JI)J", names, params)
			if !known {
				t.Fatal("original entry bindings")
			}
			d.nativeMemberRoot = &nativeMemberFamily{}
			opaque := values.NewCustomValue(func(*class_context.ClassContext) string { return "leading" }, func() types.JavaType { return types.NewJavaPrimer(types.JavaLong) })
			bad := []statements.Statement{statements.NewReturnStatement(opaque)}
			if d.prepareNativeSourceNames(bad, params, map[string]bool{"leading": true}, nil) || !d.nativeCaptureFailed || !d.nativeMemberRoot.failed {
				t.Fatal("opaque predecessor did not fail and retain publication barriers")
			}
			local := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaLong))
			local.Id.SetName("leading")
			body := []statements.Statement{statements.NewAssignStatement(local, values.NewJavaLiteral(int64(7), local.Type()), true), statements.NewReturnStatement(local)}
			switch variant {
			case "opaque sibling":
				body = append(body, statements.NewReturnStatement(opaque))
			case "changed header":
				params[0].(*values.JavaRef).Id.SetName("different")
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := make(map[*coreutils.VariableId]string, len(d.FuncCtx.LocalNames))
			for id, name := range d.FuncCtx.LocalNames {
				before[id] = name
			}
			got := d.prepareOrdinaryMethodParameterNamespace(binding, body, params)
			if got != (variant == "valid sibling") {
				t.Fatalf("method-local proof=%v", got)
			}
			if !d.nativeCaptureFailed || !d.nativeMemberRoot.failed {
				t.Fatal("a method-local success cleared a class/family failure")
			}
			if got {
				if local.String(d.FuncCtx) != "jdec$local$leading" || params[0].String(d.FuncCtx) != "leading" || params[1].String(d.FuncCtx) != "trailing" {
					t.Fatal("sibling allocator did not protect original names and rename the colliding local")
				}
			} else if !reflect.DeepEqual(before, d.FuncCtx.LocalNames) {
				t.Fatal("a failed local proof committed a partial namespace")
			}
		})
	}
}

func TestAdversarialLambdaEnclosingNamespaceKeepsLocalResultAndPublicationLatch(t *testing.T) {
	for _, opaque := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "opaque"}[opaque], func(t *testing.T) {
			ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
			ref.Id.SetName("enclosing")
			d := &ClassObjectDumper{FuncCtx: &class_context.ClassContext{Arguments: []string{"enclosing"}, LocalNames: map[*coreutils.VariableId]string{}}, nativeCaptureFailed: true}
			body := []statements.Statement{statements.NewReturnStatement(ref)}
			if opaque {
				body = append(body, statements.NewReturnStatement(values.NewCustomValue(func(*class_context.ClassContext) string { return "enclosing" }, func() types.JavaType { return ref.Type() })))
			}
			if got := d.prepareLambdaEnclosingParameterNamespace(body, nil); got == opaque {
				t.Fatal("local lambda walk result borrowed an unrelated failure or accepted opaque dependencies")
			}
			if !d.nativeCaptureFailed {
				t.Fatal("local lambda names cleared publication failure")
			}
			if !opaque && ref.String(d.FuncCtx) != "jdec$local$enclosing" || opaque && len(d.FuncCtx.LocalNames) != 0 {
				t.Fatal("local lambda namespace was not committed atomically")
			}
		})
	}
}

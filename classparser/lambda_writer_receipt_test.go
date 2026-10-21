package javaclassparser

import (
	"context"
	"testing"

	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialLambdaWriterReceiptSelectsOnlyItsLiveOriginalRequest(t *testing.T) {
	files := methodParameterBoundaryCompile(t, `class GuardOwner {
 static int read(int originalNamed){return originalNamed;}
 static java.util.function.IntUnaryOperator first(int captured){return value->captured*31+value;}
}`, "GuardOwner")
	for _, variant := range []string{"original", "preview", "other owner", "ended frame", "invalid frame", "stale frame request", "wrong body", "wrong request", "foreign source object", "detached cache", "foreign body dumper", "unsealed factory", "changed factory PC", "different original body", "shared-domain refusal", "duplicate implementation", "canceled", "rollback"} {
		t.Run(variant, func(t *testing.T) {
			d := methodParameterBoundaryDumper(t, files["GuardOwner.class"])
			name := methodSourceRequestSyntheticName(t, d, "(II)I")
			var parent *MemberInfo
			var code *CodeAttribute
			for _, member := range d.obj.Methods {
				n, _ := sourceBridgeUTF8(d.obj, member.NameIndex)
				if n == "first" {
					parent = member
					for _, attr := range member.Attributes {
						if c, known := attr.(*CodeAttribute); known {
							code = c
						}
					}
				}
			}
			if parent == nil || code == nil || len(code.Code) != 7 || code.Code[1] != 0xba {
				t.Fatal("authored one-load/factory/return instruction shape")
			}
			makeReceipt := func() (*dumpedMethods, *lambdaMethodSourceReceipt) {
				r, known := d.newLambdaMethodSourceRequest(1)
				if !known {
					t.Fatal("source request")
				}
				r.factory = &lambdaSourceFactory{object: d.obj, method: parent, code: code, pc: 1}
				if !r.factory.seal(nil) {
					t.Fatal("original factory")
				}
				body, err := d.dumpMethodWithSourceRequest(name, "(II)I", coreutils.NewRootVariableId(), nil, r)
				if err != nil || body == nil || !d.recordLambdaMethodSourceBody(name, "(II)I", r, body) {
					t.Fatal("successful original body receipt", err)
				}
				return body, d.lambdaMethodSourceBodies[body.member]
			}
			wanted, receipt := makeReceipt()
			later, current := makeReceipt()
			if wanted == later || receipt.identity == nil || receipt.identity != current.identity || d.lambdaReceiptForBody(wanted) != nil {
				t.Fatal("distinct callback namespaces must retain one original declaration identity")
			}
			owner := &dumpedMethods{member: parent, sourceRequest: methodSourceRequest{role: methodSourceDeclaration}}
			frame := &lambdaMethodEmission{body: owner, request: owner.sourceRequest}
			d.lambdaMethodRendering = frame
			var rollback func()
			switch variant {
			case "preview":
				d.lambdaMethodRendering = nil
			case "other owner":
				frame.body = &dumpedMethods{member: wanted.member}
			case "ended frame":
				frame.ended = true
			case "invalid frame":
				frame.invalid = true
			case "stale frame request":
				frame.request.serial++
			case "wrong body":
				receipt.body = later
			case "wrong request":
				receipt.request.serial++
			case "foreign source object":
				copy := *d.obj
				receipt.object = &copy
			case "detached cache":
				delete(d.dumpedMethodsSet, receipt.key)
			case "foreign body dumper":
				copy := *d
				wanted.sourceDumper = &copy
			case "unsealed factory":
				receipt.request.factory.sealed = false
			case "changed factory PC":
				receipt.request.factory.pc++
			case "different original body":
				receipt.identity = values.NewLambdaSourceIdentity()
			case "shared-domain refusal":
				current.shared = []*lambdaMethodSourceReceipt{receipt}
			case "duplicate implementation":
				frame.children = []lambdaMethodOccurrence{{body: later, request: later.sourceRequest, packet: "already observed"}}
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			case "rollback":
				rollback = d.snapshotRetryState()
			}
			got := d.selectLambdaWriterReceipt(wanted, receipt)
			selects := variant == "original" || variant == "rollback"
			want := selects || variant == "preview" || variant == "other owner"
			if got != want || selects && d.lambdaMethodSourceBodies[wanted.member] != receipt || !selects && d.lambdaMethodSourceBodies[wanted.member] != current {
				t.Fatalf("writer selection=%v; want %v, selects=%v", got, want, selects)
			}
			if !want && !frame.invalid {
				t.Fatal("refused writer must invalidate its active emission frame")
			}
			if rollback != nil {
				rollback()
				restored := d.lambdaMethodSourceBodies[later.member]
				if restored == nil || restored.identity != current.identity || restored.request != later.sourceRequest || restored.key != later.sourceCacheKey || restored.body == wanted {
					t.Fatal("declined retry retained the selected writer instead of its prior receipt")
				}
			}
		})
	}
}

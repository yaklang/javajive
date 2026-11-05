package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestNativeSharedReturnOperandRequiresOriginalOrderedHandlers(t *testing.T) {
	for _, variant := range []string{"split intervals", "copied rows", "nested call", "unknown table", "nil row", "invalid range", "oversized table", "unprotected owner", "unprotected call", "wrong catch type", "wrong handler", "reversed order", "unknown invocation", "unprotected nested call", "missing return witness", "wrong return witness", "earlier effect", "later effect", "cleanup successor", "expression cycle", "handler work budget"} {
		t.Run(variant, func(t *testing.T) {
			owner := core.NewNode(&statements.MiddleStatement{Flag: "switch"})
			owner.OriginPC, owner.HasOriginPC = 10, true
			call := &values.FunctionCallExpression{IsStatic: true, Kind: values.InvokeStatic, ClassName: "Example", FunctionName: "produce", Descriptor: "()Ljava/lang/Object;", OriginPC: 27, HasOriginPC: true}
			ret := &statements.ReturnStatement{JavaValue: call, OriginPC: 30, HasOriginPC: true}
			target := core.NewNode(ret)
			target.OriginPC, target.HasOriginPC = 30, true
			target.AddNext(core.NewNode(&statements.MiddleStatement{Flag: "end"}))
			rows := []*core.ExceptionTableEntry{
				{StartPc: 5, EndPc: 20, HandlerPc: 90, CatchType: 1},
				{StartPc: 24, EndPc: 30, HandlerPc: 90, CatchType: 1},
				{StartPc: 5, EndPc: 20, HandlerPc: 95, CatchType: 0},
				{StartPc: 24, EndPc: 30, HandlerPc: 95, CatchType: 0},
			}
			want := false
			switch variant {
			case "split intervals", "copied rows":
				want = true
			case "nested call", "unprotected nested call":
				child := *call
				child.OriginPC = 25
				if variant == "unprotected nested call" {
					child.OriginPC = 22
				}
				call.Arguments, call.Descriptor = []values.JavaValue{&child}, "(Ljava/lang/Object;)Ljava/lang/Object;"
				want = variant == "nested call"
			case "nil row":
				rows = append(rows, nil)
			case "invalid range":
				rows[0].EndPc = rows[0].StartPc
			case "oversized table":
				for len(rows) < 4097 {
					rows = append(rows, rows[0])
				}
			case "unprotected owner":
				owner.OriginPC = 1
			case "unprotected call":
				call.OriginPC = 22
			case "wrong catch type":
				rows[1].CatchType = 2
			case "wrong handler":
				rows[1].HandlerPc = 91
			case "reversed order":
				rows[1], rows[3] = rows[3], rows[1]
			case "unknown invocation":
				call.HasOriginPC = false
			case "missing return witness":
				ret.HasOriginPC = false
			case "wrong return witness":
				ret.OriginPC = 29
			case "earlier effect":
				call.OriginPC = 9
			case "later effect":
				call.OriginPC = 30
			case "cleanup successor":
				target.AddNext(jumpTestNode("cleanup()"))
			case "expression cycle":
				call.Arguments = []values.JavaValue{call}
			case "handler work budget":
				rows = nil
				for i := 0; i < 4096; i++ {
					rows = append(rows, &core.ExceptionTableEntry{StartPc: 0, EndPc: 100, HandlerPc: 101, CatchType: 1})
				}
				ret.OriginPC, target.OriginPC = 100, 100
				cursor := call
				for pc := 28; pc < 50; pc++ {
					child := *call
					child.OriginPC, child.Arguments = pc, nil
					cursor.Arguments = []values.JavaValue{&child}
					cursor = &child
				}
			}
			manager := NewRootStatementManager(owner)
			if variant != "unknown table" {
				manager.SetOriginalExceptionTable(rows)
			}
			if variant == "copied rows" {
				rows[1].CatchType = 9
			}
			if got := sharedReturnOperandProtection(manager, owner, target); got != want {
				t.Fatalf("operand handler proof=%v, want %v", got, want)
			}
		})
	}
}

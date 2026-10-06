package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestConstructorPreludeHandlerDomainRequiresExactSeparation(t *testing.T) {
	for _, variant := range []string{"none", "later", "at initialization end", "prefix covered", "invoke operand covered", "backward handler", "empty range", "reversed range", "range past code", "handler past code", "nil handler", "mixed domains", "wrong instruction", "truncated instruction", "negative PC", "absent code", "budget", "canceled", "start inside operand", "end inside operand", "handler inside operand", "range ends at code end", "absent decoder", "unparsed decoder", "different invoke operands"} {
		t.Run(variant, func(t *testing.T) {
			code := &CodeAttribute{Code: []byte{core.OP_ALOAD_0, core.OP_INVOKESPECIAL, 0, 1, core.OP_NOP, core.OP_NOP, core.OP_RETURN, core.OP_NOP, core.OP_ASTORE_1, core.OP_RETURN}}
			pc := 1
			handler := &ExceptionTableEntry{StartPc: 4, EndPc: 6, HandlerPc: 8}
			code.ExceptionTable = []*ExceptionTableEntry{handler}
			var work *workbudget.Budget
			want := variant == "none" || variant == "later" || variant == "at initialization end" || variant == "range ends at code end"
			switch variant {
			case "none":
				code.ExceptionTable = nil
			case "later":
				handler.StartPc = 5
			case "range ends at code end":
				handler.EndPc = uint16(len(code.Code))
			case "prefix covered":
				handler.StartPc = 0
			case "invoke operand covered":
				handler.StartPc = 2
			case "backward handler":
				handler.HandlerPc = 0
			case "empty range":
				handler.EndPc = handler.StartPc
			case "reversed range":
				handler.EndPc = 3
			case "range past code":
				handler.EndPc = 11
			case "handler past code":
				handler.HandlerPc = 10
			case "nil handler":
				code.ExceptionTable[0] = nil
			case "mixed domains":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 4, HandlerPc: 8})
			case "wrong instruction":
				code.Code[pc] = core.OP_INVOKESTATIC
			case "truncated instruction":
				code.Code = code.Code[:3]
			case "negative PC":
				pc = -1
			case "absent code":
				code = nil
			case "start inside operand", "end inside operand", "handler inside operand":
				code.Code[5] = core.OP_SIPUSH
				switch variant {
				case "start inside operand":
					handler.StartPc, handler.EndPc = 6, 8
				case "end inside operand":
					handler.EndPc = 7
				case "handler inside operand":
					handler.EndPc, handler.HandlerPc = 8, 6
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			var decoder *core.Decompiler
			if code != nil {
				decoder = core.NewDecompiler(code.Code, nil)
				if err := decoder.ParseOpcode(); err != nil {
					decoder = nil
				}
			}
			switch variant {
			case "absent decoder":
				decoder = nil
			case "unparsed decoder":
				decoder = core.NewDecompiler(code.Code, nil)
			case "different invoke operands":
				foreign := append([]byte(nil), code.Code...)
				foreign[pc+2] ^= 1
				decoder = core.NewDecompiler(foreign, nil)
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
			}
			if got := constructorPreludeOutsideHandlers(code, pc, decoder, work); got != want {
				t.Fatalf("prelude outside handlers=%v want=%v", got, want)
			}
		})
	}
}

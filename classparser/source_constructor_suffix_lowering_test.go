package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorSuffixRejectsEdgesAndHandlersAcrossInitialization(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original compiler required")
	}
	fixture := strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", `try{long r=seed;for(int i=0;i<5;i++)r=r*31L+mode;result=r;}catch(RuntimeException e){throw e;}SuffixWitness.visits++;`, 1), "ROUNDS", "5")
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "SuffixOwner", "none", false)
	for _, kind := range []string{"control", "backedge prefix", "branch operand", "handler start prefix", "handler target prefix", "handler end operand", "reversed handler", "missing catch type", "canceled", "work", "allocation"} {
		t.Run(kind, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(files["SuffixOwner$1.class"]))
			var ctor *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			p := constructorLoweringPacketFor(o, ctor, true, true, nil)
			if p == nil || !p.plan.body || len(p.code.ExceptionTable) == 0 {
				t.Fatal("actual original isolated suffix packet")
			}
			code, ops := p.code, p.ops
			changed := false
			switch kind {
			case "backedge prefix", "branch operand":
				for _, op := range ops {
					if constructorMotionPacketBranch(op.Instr.OpCode) && len(op.Data) == 2 {
						pc := int(op.CurrentOffset)
						target := 0
						if kind == "branch operand" {
							target = pc + 1
						}
						binary.BigEndian.PutUint16(op.Data, uint16(int16(target-pc)))
						changed = true
						break
					}
				}
			case "handler start prefix":
				code.ExceptionTable[0].StartPc = 0
				changed = true
			case "handler target prefix":
				code.ExceptionTable[0].HandlerPc = 0
				changed = true
			case "handler end operand":
				for _, op := range ops {
					if int(op.CurrentOffset) >= p.secondEnd && len(op.Data) > 0 {
						code.ExceptionTable[0].EndPc = uint16(op.CurrentOffset + 1)
						changed = true
						break
					}
				}
			case "reversed handler":
				code.ExceptionTable[0].EndPc = code.ExceptionTable[0].StartPc
				changed = true
			case "missing catch type":
				code.ExceptionTable[0].CatchType = uint16(o.ConstantPoolManager.AddUtf8Info("not a class"))
				changed = true
			}
			var work *workbudget.Budget
			switch kind {
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				changed = true
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
				changed = true
			case "allocation":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				changed = true
			}
			if kind != "control" && !changed {
				t.Fatal("mutation absent")
			}
			if constructorLoweringSuffixClosed(o, code, ops, p.secondEnd, work) != (kind == "control") {
				t.Fatal("unproved control edge or exception domain", kind)
			}
		})
	}
}

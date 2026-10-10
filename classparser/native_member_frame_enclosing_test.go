package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMemberFrameEnclosingRequiresExactOriginalCapturePath(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"FrameEnclosingOwner.java": `
class FrameEnclosingOwner {class Parent {Parent(Class c){}}
 class Layer {class Child extends Parent {Child(java.lang.reflect.Field f){super(f.getType());}}}
}`}, "none", "8")
	for _, variant := range []string{"original", "direct parameter", "wrong parameter", "wrong field owner", "wrong field name", "wrong field descriptor", "wrong base parameter", "wrong parameter owner", "wrong result slot", "wrong result auxiliary", "missing IR", "missing frames", "missing use", "duplicate record", "foreign use", "cyclic path", "backward path", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["FrameEnclosingOwner$Layer$Child.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<init>" {
					method = m
					for _, a := range m.Attributes {
						if c, known := a.(*CodeAttribute); known {
							code = c
						}
					}
				}
			}
			ir, frames, known := NewClassObjectDumper(obj).nativeOriginalMethodSnapshot(method, code)
			if !known {
				t.Fatal("original enclosing method snapshot")
			}
			var path *nativeMemberLexicalRead
			baseIndex := -1
			for i, ins := range ir.Instrs {
				if ins.Opcode == core.OP_ALOAD_1 {
					baseIndex = i
				}
				if ins.Opcode == core.OP_GETFIELD && ins.Class == "FrameEnclosingOwner$Layer" && ins.Desc == "LFrameEnclosingOwner;" {
					if baseIndex < 0 {
						t.Fatal("missing original enclosing parameter load")
					}
					path = &nativeMemberLexicalRead{pc: int(ins.PC), owner: ins.Class, field: ins.Member, descriptor: ins.Desc, basePC: int(ir.Instrs[baseIndex].PC), parameterOwner: "FrameEnclosingOwner$Layer"}
					break
				}
			}
			if path == nil {
				t.Fatal("missing original capture read")
			}
			origin := ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(path.pc)}
			var work *workbudget.Budget
			switch variant {
			case "direct parameter", "wrong parameter":
				path = nil
				origin = ssabuild.Origin{Kind: ssabuild.OriginParam, Slot: 1}
				if variant == "wrong parameter" {
					origin.Slot = 2
				}
			case "wrong field owner":
				path.owner += "Other"
			case "wrong field name":
				path.field += "Other"
			case "wrong field descriptor":
				path.descriptor = "Ljava/lang/Object;"
			case "wrong base parameter":
				ir.Instrs[baseIndex].Local = 2
			case "wrong parameter owner":
				path.parameterOwner = "FrameEnclosingOwner"
			case "wrong result slot":
				origin.Slot = 1
			case "wrong result auxiliary":
				origin.Aux = 1
			case "missing IR":
				ir = nil
			case "missing frames":
				frames = nil
			case "missing use", "duplicate record", "foreign use":
				for i, record := range frames.Instructions {
					if record.PC != origin.PC {
						continue
					}
					if variant == "missing use" {
						frames.Instructions[i].Uses = nil
					} else if variant == "foreign use" {
						frames.Instructions[i].Uses = []ssabuild.Origin{{Kind: ssabuild.OriginParam, Slot: 2}}
					} else {
						frames.Instructions = append(frames.Instructions, record)
					}
					break
				}
			case "cyclic path":
				path.prior = path
			case "backward path":
				copy := *path
				copy.pc++
				path.prior = &copy
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberFrameEnclosingOrigin(ir, frames, origin, path, work); got != (variant == "original" || variant == "direct parameter") {
				t.Fatalf("original enclosing origin=%v", got)
			}
		})
	}
}

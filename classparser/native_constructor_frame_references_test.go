package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorFrameReferencesRequireEveryOriginalReachingProducer(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"FrameRefOwner.java": `
interface FrameRefContract {}
class FrameRefBase implements FrameRefContract {}
class FrameRefLeaf extends FrameRefBase {}
class FrameRefParent {FrameRefParent(Class t,FrameRefContract[] a){}}
class FrameRefOwner {class Child extends FrameRefParent {
 Child(java.lang.reflect.Field f,FrameRefLeaf leaf,FrameRefBase base,FrameRefLeaf[] table,boolean flag){super(f.getType(),new FrameRefContract[]{flag?leaf:base,null,table[0]});}
}}`}, "none", "8")
	provider := func(name string) (callbinding.Class, bool) {
		parents, known := map[string][]string{"FrameRefLeaf": {"FrameRefBase"}, "FrameRefBase": {"FrameRefContract"}, "FrameRefContract": {}, "Rival": {}}[name]
		return callbinding.Class{Name: name, Parents: parents, ParentsComplete: known}, known
	}
	for _, variant := range []string{"original phi", "typed array read", "all reference to Object", "missing phi", "one foreign arm", "one unknown arm", "null arm", "cycle", "unknown hierarchy", "wrong phi identity", "wrong instruction identity", "missing array operands", "duplicate array record", "budget", "canceled", "operand limit"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["FrameRefOwner$Child.class"]...))
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
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			ir, frames, known := NewClassObjectDumper(obj).nativeOriginalMethodSnapshot(method, code)
			if !known {
				t.Fatal("original method snapshot")
			}
			var stored []ssabuild.Origin
			for _, record := range frames.Instructions {
				ins, ok := ir.InstrByID(methodir.InstrID(record.PC))
				if ok && ins.Opcode == core.OP_AASTORE {
					stored = append(stored, record.BeforeOrigins[len(record.BeforeOrigins)-1])
				}
			}
			if len(stored) != 3 || stored[0].Kind != ssabuild.OriginPhi {
				t.Fatalf("original phi/null/array read=%v", stored)
			}
			p := newNativeConstructorFrameWords(obj, method, ir, frames, nil)
			if p == nil {
				t.Fatal("original producer graph")
			}
			origin, formal := stored[0], "LFrameRefContract;"
			q := newConstructorWideningQuery(provider)
			want := variant == "original phi" || variant == "typed array read" || variant == "all reference to Object" || variant == "null arm"
			switch variant {
			case "typed array read":
				origin = stored[2]
			case "all reference to Object":
				formal = "Ljava/lang/Object;"
			case "missing phi":
				delete(p.phis, origin)
			case "one foreign arm", "one unknown arm":
				p.params[4] = "LRival;"
				if variant == "one unknown arm" {
					p.params[4] = "LUnknown;"
				}
			case "null arm":
				p.phis[origin] = []ssabuild.Origin{{Kind: ssabuild.OriginParam, Slot: 3}, stored[1]}
			case "cycle":
				p.phis[origin] = []ssabuild.Origin{origin}
			case "unknown hierarchy":
				q = newConstructorWideningQuery(nil)
			case "wrong phi identity":
				origin.Aux++
			case "wrong instruction identity":
				origin = stored[2]
				origin.Slot++
			case "missing array operands", "duplicate array record":
				origin = stored[2]
				for i, record := range frames.Instructions {
					if record.PC == origin.PC {
						if variant == "missing array operands" {
							frames.Instructions[i].Uses = nil
						} else {
							frames.Instructions = append(frames.Instructions, record)
						}
						break
					}
				}
			case "budget":
				p.work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				p.work = workbudget.New(ctx, workbudget.Limits{})
			case "operand limit":
				p.remaining = 1
			}
			if got := p.reference(origin, formal, q, frames); got != want {
				t.Fatalf("all-reaching assignment=%v want=%v", got, want)
			}
		})
	}
}

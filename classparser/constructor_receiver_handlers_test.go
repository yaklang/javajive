package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"reflect"
	"strconv"
	"testing"
)

func TestAdversarialConstructorCatchAllTargetsKeepTableOrderAndThrowingBoundaries(t *testing.T) {
	// The self-protected ASTORE at7 cannot itself throw. IDIV at4 is covered
	// by two catch-all rows; only the first matching row can handle it.
	for _, variant := range []string{"original", "empty", "overlapping first match", "astore self range", "end at code length", "typed catch", "nil handler", "empty range", "start not boundary", "end not boundary", "handler not boundary", "invalid original opcode", "nil op", "budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			code := &CodeAttribute{Code: []byte{core.OP_SIPUSH, 0, 1, core.OP_ICONST_0, core.OP_IDIV, core.OP_POP, core.OP_RETURN, core.OP_ASTORE_0, core.OP_ALOAD_0, core.OP_ATHROW}, ExceptionTable: []*ExceptionTableEntry{{StartPc: 0, EndPc: 5, HandlerPc: 7}}}
			decoder := core.NewDecompiler(code.Code, func(int) values.JavaValue { return nil })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			remaining := 512
			var work *workbudget.Budget
			want := map[int]int{2: 5}
			accept := true
			switch variant {
			case "empty":
				code.ExceptionTable = nil
				want = map[int]int{}
			case "overlapping first match":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 4, EndPc: 6, HandlerPc: 6})
			case "astore self range":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 7, EndPc: 8, HandlerPc: 7})
			case "end at code length":
				code.ExceptionTable[0].EndPc = uint16(len(code.Code))
				want[7] = 5
			case "typed catch":
				code.ExceptionTable[0].CatchType = 1
				accept = false
			case "nil handler":
				code.ExceptionTable[0] = nil
				accept = false
			case "empty range":
				code.ExceptionTable[0].StartPc = 5
				accept = false
			case "start not boundary":
				code.ExceptionTable[0].StartPc = 1
				accept = false
			case "end not boundary":
				code.ExceptionTable[0].EndPc = 2
				accept = false
			case "handler not boundary":
				code.ExceptionTable[0].HandlerPc = 2
				accept = false
			case "invalid original opcode":
				code.Code[4] = core.OP_IADD
				accept = false
			case "nil op":
				ops[2] = nil
				accept = false
			case "budget":
				remaining = 0
				accept = false
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				accept = false
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				accept = false
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
				accept = false
			}
			targets, ok := constructorReceiverCatchAllHandlerTargets(code, ops, &remaining, work)
			if ok != accept || ok && !reflect.DeepEqual(targets, want) {
				t.Fatalf("targets=%v remaining=%d accepted=%v want=%v accepted=%v", targets, remaining, ok, want, accept)
			}
		})
	}
}

func TestAdversarialConstructorCatchAllExceptionFrameInvalidatesOnlyThrowingNewAliases(t *testing.T) {
	original := []constructorEffectValue{{kind: 'L', receiver: true}, {kind: 'L', allocation: 7}, {kind: 'L', allocation: 7}, {kind: 'L', allocation: 9}, {kind: 'I', knownInt: true, intWord: -3}, {kind: 'J'}, {}}
	for _, failed := range []int{0, 7, 9} {
		t.Run(strconv.Itoa(failed), func(t *testing.T) {
			got := constructorEffectExceptionLocals(original, failed)
			want := append([]constructorEffectValue(nil), original...)
			if failed == 7 {
				want[1] = constructorEffectValue{}
				want[2] = constructorEffectValue{}
			} else if failed == 9 {
				want[3] = constructorEffectValue{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("handler INPUT locals=%+v want %+v", got, want)
			}
			got[0] = constructorEffectValue{}
			if original[0].kind != 'L' || !original[0].receiver || original[1].allocation != 7 || original[2].allocation != 7 || original[3].allocation != 9 {
				t.Fatalf("exception transfer mutated original locals: %+v", original)
			}
		})
	}
}

const closedHandlerEvidenceFixture = `class ClosedHandlerEffects{static Object saved;static void opaque(){}}
class ClosedMethodEvidence{
 int word,capture;Object self;
 private void cleanup(int n){try{word=n;ClosedHandlerEffects.opaque();}finally{word=n+1;}}
 private int returning(int n){try{ClosedHandlerEffects.opaque();return n;}finally{word=n;}}
 private Object reference(Object input){try{ClosedHandlerEffects.opaque();return input;}finally{word=1;}}
 private void nested(int n){try{try{word=n;ClosedHandlerEffects.opaque();}finally{word=n+1;}}finally{word=n+2;}}
 private void publish(){try{ClosedHandlerEffects.opaque();}finally{ClosedHandlerEffects.saved=this;}}
 private void readCapture(){try{ClosedHandlerEffects.opaque();}finally{word=capture;}}
 private void catchRead(){try{ClosedHandlerEffects.opaque();}catch(Throwable ex){word=capture;}}
 private void catchPublish(){try{ClosedHandlerEffects.opaque();}catch(Throwable ex){ClosedHandlerEffects.saved=this;}}
 private void catchSelf(){try{ClosedHandlerEffects.opaque();}catch(Throwable ex){self=this;}}
 private void crossSelf(){try{self=this;ClosedHandlerEffects.opaque();}catch(Throwable ex){ClosedHandlerEffects.saved=self;}}
}`

func TestAdversarialConstructorClosedFinallyUsesOriginalHandlerEffectsAndStorage(t *testing.T) {
	files := nativeCompileClasses(t, closedHandlerEvidenceFixture)
	for _, row := range []struct {
		name, desc string
		args       []constructorEffectValue
		want       bool
	}{
		{"cleanup", "(I)V", []constructorEffectValue{{kind: 'I'}}, true},
		{"returning", "(I)I", []constructorEffectValue{{kind: 'I'}}, true},
		{"reference", "(Ljava/lang/Object;)Ljava/lang/Object;", []constructorEffectValue{{kind: 'L'}}, true},
		{"nested", "(I)V", []constructorEffectValue{{kind: 'I'}}, true},
		{"publish", "()V", nil, false}, {"readCapture", "()V", nil, false},
		{"catchRead", "()V", nil, false}, {"catchPublish", "()V", nil, false},
		{"catchSelf", "()V", nil, true}, {"crossSelf", "()V", nil, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			obj, _, code := closedMethodEvidence(t, files, row.name)
			if len(code.ExceptionTable) == 0 {
				t.Fatal("original handler fixture missing exception table")
			}
			// A catch(Throwable) row has the same selection domain as catch_type0.
			// These typed classfile model edits are unit refusal/admission evidence, not
			// compiled JVM semantic mutants and are never counted as such.
			for _, h := range code.ExceptionTable {
				h.CatchType = 0
			}
			remaining := 512
			aliases := &constructorSelfStorageProof{}
			d := &ClassObjectDumper{obj: obj, constructorReceiverFinalizerSilent: true}
			writes := map[string]bool{obj.GetClassName() + "\x00capture\x00I": true}
			value, accepted := d.constructorReceiverClosedMethod(obj, &values.JavaClassMember{Name: obj.GetClassName(), Member: row.name, Description: row.desc}, core.OP_INVOKESPECIAL, writes, map[string]bool{}, &remaining, 0, aliases, row.args...)
			if accepted != row.want || accepted && (value.receiver || value.allocation != 0 || value.knownInt) {
				t.Fatalf("accepted=%v want=%v returned=%+v aliases=%+v remaining=%d", accepted, row.want, value, aliases, remaining)
			}
		})
	}
}

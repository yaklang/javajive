package javaclassparser

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorLinearHandlerLivenessRetainsIncomingReads(t *testing.T) {
	for _, row := range []struct {
		name     string
		code     []byte
		slots    int
		handlers map[int]int
		want     []bool
	}{
		{"killed exception local", []byte{core.OP_ASTORE_3, core.OP_ALOAD_0, core.OP_POP, core.OP_ALOAD_3, core.OP_ATHROW}, 4, nil, []bool{true, false, false, false}},
		{"read before copy", []byte{core.OP_ASTORE_3, core.OP_ILOAD_1, core.OP_ISTORE_2, core.OP_ILOAD_2, core.OP_POP, core.OP_ALOAD_3, core.OP_ATHROW}, 4, nil, []bool{false, true, false, false}},
		{"increment reads input", []byte{core.OP_ASTORE_3, core.OP_IINC, 1, 1, core.OP_ALOAD_3, core.OP_ATHROW}, 4, nil, []bool{false, true, false, false}},
		{"wide input has two slots", []byte{core.OP_ASTORE_3, core.OP_LLOAD_1, core.OP_POP2, core.OP_ALOAD_3, core.OP_ATHROW}, 4, nil, []bool{false, true, true, false}},
		{"new wide value kills both", []byte{core.OP_ASTORE, 4, core.OP_LCONST_0, core.OP_LSTORE_1, core.OP_LLOAD_1, core.OP_POP2, core.OP_ALOAD, 4, core.OP_ATHROW}, 5, nil, []bool{false, false, false, false, false}},
		{"branch", []byte{core.OP_ASTORE_3, core.OP_ILOAD_1, core.OP_IFEQ, 0, 3, core.OP_ALOAD_3, core.OP_ATHROW}, 4, nil, nil},
		{"covered throw", []byte{core.OP_ASTORE_3, core.OP_ALOAD_3, core.OP_ATHROW}, 4, map[int]int{2: 0}, nil},
		{"missing exit", []byte{core.OP_ASTORE_3, core.OP_NOP}, 4, nil, nil},
		{"slot outside frame", []byte{core.OP_ASTORE_3, core.OP_ALOAD, 5, core.OP_ATHROW}, 4, nil, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			d := core.NewDecompiler(row.code, func(int) values.JavaValue { return nil })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			remaining := 512
			got := constructorLinearHandlerLiveness(constructorMotionOps(d), 0, row.slots, row.handlers, &remaining, nil)
			if !reflect.DeepEqual(got, row.want) {
				t.Fatalf("incoming reads=%v want=%v", got, row.want)
			}
		})
	}
}

func TestAdversarialConstructorHandlerProofEpochIncludesMetadataAndRootQueries(t *testing.T) {
	e := &constructorProfileEvidence{eligible: true, inBody: true}
	proofEpoch := e.observationEpoch
	called := false
	provider := e.metadata(func(name string) (callbinding.Class, bool) {
		called = true
		if e.observationEpoch == proofEpoch {
			t.Fatal("provider callback ran while the old path certificate remained current")
		}
		return callbinding.Class{}, false
	})
	if _, known := provider("java/lang/Object"); known || !called || !e.eligible || e.observationEpoch == proofEpoch {
		t.Fatal("missing bootstrap metadata query must invalidate the epoch even though eligibility is unchanged")
	}
	proofEpoch = e.observationEpoch
	if !e.record(&ClassObjectDumper{}, constructorProfileObservation{absentRootMethod: "read", methodDescriptor: "()I"}) || e.observationEpoch == proofEpoch {
		t.Fatal("root method-table query left an old certificate current")
	}
	proofEpoch = e.observationEpoch
	e.observed() // The resolver wrapper records even a provider attempt returning no bytes.
	if e.observationEpoch == proofEpoch {
		t.Fatal("provider attempt left an old certificate current")
	}
	e.observationEpoch = ^uint64(0)
	e.observed()
	if e.eligible || !e.inconsistent {
		t.Fatal("epoch wrap must never revive an old certificate")
	}
}

func TestAdversarialConstructorHandlerFrameProjectionKeepsLiveIdentityAndWideShape(t *testing.T) {
	locals := []constructorEffectValue{{kind: 'L', receiver: true}, {kind: 'I', knownInt: true, intWord: 3}, {kind: 'L', allocation: 7}, {kind: 'L'}}
	stack := []constructorEffectValue{{kind: 'L'}}
	live := []bool{true, false, true, false}
	base := constructorEffectFrameKey(9, locals, stack, true, live, 0)
	for _, variant := range []string{"dead scalar", "dead receiver", "dead allocation", "dead wide", "live receiver", "live allocation", "stack receiver", "stack allocation", "uninitialized", "different pc"} {
		t.Run(variant, func(t *testing.T) {
			ls, ss := append([]constructorEffectValue(nil), locals...), append([]constructorEffectValue(nil), stack...)
			pc, initialized, equal := 9, true, false
			switch variant {
			case "dead scalar":
				ls[1].intWord = 37
				equal = true
			case "dead receiver":
				ls[1] = constructorEffectValue{kind: 'L', receiver: true}
				equal = true
			case "dead allocation":
				ls[1] = constructorEffectValue{kind: 'L', allocation: 19}
				equal = true
			case "dead wide":
				ls[1] = constructorEffectValue{kind: 'J'}
			case "live receiver":
				ls[0].receiver = false
			case "live allocation":
				ls[2].allocation = 19
			case "stack receiver":
				ss[0].receiver = true
			case "stack allocation":
				ss[0].allocation = 19
			case "uninitialized":
				initialized = false
			case "different pc":
				pc++
			}
			if got := constructorEffectFrameKey(pc, ls, ss, initialized, live, 0); (got == base) != equal {
				t.Fatalf("equal=%v want=%v for %s", got == base, equal, variant)
			}
		})
	}
	cleared := append([]constructorEffectValue(nil), locals...)
	cleared[2] = constructorEffectValue{}
	if constructorEffectFrameKey(9, locals, stack, true, live, 7) != constructorEffectFrameKey(9, cleared, stack, true, live, 0) {
		t.Fatal("throwing initializer aliases not invalidated in the key")
	}
	if !locals[0].receiver || locals[2].allocation != 7 || stack[0].receiver {
		t.Fatal("projection changed the interpreter's input")
	}
	wide := append([]constructorEffectValue(nil), locals...)
	wide[1] = constructorEffectValue{kind: 'J'}
	double := append([]constructorEffectValue(nil), wide...)
	double[1].kind = 'D'
	if constructorEffectFrameKey(9, wide, stack, true, live, 0) != constructorEffectFrameKey(9, double, stack, true, live, 0) {
		t.Fatal("dead category-2 values should retain their common overwrite shape")
	}
}

func TestAdversarialConstructorHandlerLivenessRespectsProofResources(t *testing.T) {
	d := core.NewDecompiler([]byte{core.OP_ASTORE_1, core.OP_ALOAD_1, core.OP_ATHROW}, func(int) values.JavaValue { return nil })
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"shared units", "graph scans", "memory", "cancel"} {
		t.Run(variant, func(t *testing.T) {
			remaining := 512
			slots := 2
			var work *workbudget.Budget
			switch variant {
			case "shared units":
				remaining = 1
			case "graph scans":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				// CheckAlloc derives an eight-byte cap from this output limit.
				// The mask must exceed that actual cap to exercise rejection.
				slots = 9
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := constructorLinearHandlerLiveness(constructorMotionOps(d), 0, slots, nil, &remaining, work); got != nil {
				t.Fatalf("resource failure retained a liveness proof: %v", got)
			}
		})
	}
}

// Independent forward provenance oracle: remember whether each scalar still
// depends on its incoming parameter. Observe that dependency on every read;
// a fresh store removes it, while IINC reads and preserves the dependency.
// Enumerate the complete finite grammar (six commands, lengths zero to four).
func TestAdversarialConstructorHandlerLivenessFiniteReadProvenanceModel(t *testing.T) {
	commands := [][]byte{{core.OP_ILOAD_1, core.OP_POP}, {core.OP_ICONST_0, core.OP_ISTORE_1}, {core.OP_ILOAD_2, core.OP_POP}, {core.OP_ICONST_0, core.OP_ISTORE_2}, {core.OP_IINC, 1, 1}, {core.OP_IINC, 2, 1}}
	checked := 0
	for length := 0; length <= 4; length++ {
		combinations := 1
		for i := 0; i < length; i++ {
			combinations *= len(commands)
		}
		for encoded := 0; encoded < combinations; encoded++ {
			code := []byte{core.OP_ASTORE_3}
			dependent, observed := [2]bool{true, true}, [2]bool{}
			n := encoded
			for i := 0; i < length; i++ {
				command := n % len(commands)
				n /= len(commands)
				code = append(code, commands[command]...)
				switch command {
				case 0:
					observed[0] = observed[0] || dependent[0]
				case 1:
					dependent[0] = false
				case 2:
					observed[1] = observed[1] || dependent[1]
				case 3:
					dependent[1] = false
				case 4:
					observed[0] = observed[0] || dependent[0]
				case 5:
					observed[1] = observed[1] || dependent[1]
				}
			}
			code = append(code, core.OP_ALOAD_3, core.OP_ATHROW)
			d := core.NewDecompiler(code, func(int) values.JavaValue { return nil })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			remaining := 512
			got := constructorLinearHandlerLiveness(constructorMotionOps(d), 0, 4, nil, &remaining, nil)
			want := []bool{false, observed[0], observed[1], false}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("length=%d word=%d got=%v want=%v", length, encoded, got, want)
			}
			checked++
		}
	}
	if checked != 1555 {
		t.Fatalf("finite grammar omitted cases: %d", checked)
	}
}

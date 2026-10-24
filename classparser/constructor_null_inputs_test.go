package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

func TestAdversarialConstructorNullFactsKeepEveryUnknownObservationPath(t *testing.T) {
	const source = `
class NullFactParent {static Object escaped;NullFactParent(Object gate){if(gate!=null)escaped=this;}}
class NullFactFixed extends NullFactParent {NullFactFixed(){super(null);}}
class NullFactForward extends NullFactParent {NullFactForward(Object gate){super(gate);}}
class NullFactSource {static Object gate;static Object value(){return null;}}
class NullFactStatic extends NullFactParent {NullFactStatic(){super(NullFactSource.gate);}}
class NullFactCall extends NullFactParent {NullFactCall(){super(NullFactSource.value());}}
class NullFactCycle {static Object escaped;NullFactCycle(int n){Object gate=null;for(int i=0;i<n;i++){if(gate!=null)escaped=this;gate=new Object();}}}
class NullFactAcyclic {static Object escaped;NullFactAcyclic(int n){Object gate=null;if(n!=0)gate=new Object();if(gate!=null)escaped=this;}}
`
	files := nativeCompileDebugClasses(t, source, "none")
	root, err := Parse(files["NullFactFixed.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, descriptor string
		want             bool
	}{
		{"NullFactFixed", "()V", true},
		{"NullFactForward", "(Ljava/lang/Object;)V", false},
		{"NullFactStatic", "()V", false},
		{"NullFactCall", "()V", false},
		{"NullFactCycle", "(I)V", false},
		{"NullFactAcyclic", "(I)V", false},
	} {
		t.Run(row.name, func(t *testing.T) {
			d := &ClassObjectDumper{obj: root, foldSiblingResolver: func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }, FuncCtx: &class_context.ClassContext{}}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			if got := d.constructorCaptureChainDoesNotObserve(row.name, row.descriptor, map[string]bool{}); got != row.want {
				t.Fatalf("original null/unknown path proof=%v, want=%v", got, row.want)
			}
		})
	}
}

func TestAdversarialConstructorNullFactsJoinAndMemoRetainTheirDomains(t *testing.T) {
	null := constructorEffectValue{kind: 'L', knownNull: true}
	unknown := constructorEffectValue{kind: 'L'}
	for _, left := range []constructorEffectValue{null, unknown} {
		for _, right := range []constructorEffectValue{null, unknown} {
			prior := &constructorEffectLoopFrame{initialized: true, locals: []constructorEffectValue{left}, stack: []constructorEffectValue{{kind: 'J'}}}
			joined, changed, ok := constructorEffectLoopJoin(prior, []constructorEffectValue{right}, prior.stack, true)
			if !ok || joined.locals[0].knownNull != (left.knownNull && right.knownNull) || changed != (left.knownNull && !right.knownNull) || prior.locals[0] != left {
				t.Fatal("join must contain both incoming null/reference domains without mutating evidence")
			}
			reverse, _, ok := constructorEffectLoopJoin(&constructorEffectLoopFrame{initialized: true, locals: []constructorEffectValue{right}, stack: prior.stack}, []constructorEffectValue{left}, prior.stack, true)
			if !ok || !reflect.DeepEqual(joined, reverse) {
				t.Fatal("null-domain join depends on predecessor order")
			}
			if _, change, ok := constructorEffectLoopJoin(joined, []constructorEffectValue{right}, prior.stack, true); !ok || change {
				t.Fatal("widened loop invariant is not stable")
			}
		}
	}
	for _, part := range []string{"local", "stack", "live projection"} {
		t.Run(part, func(t *testing.T) {
			key := func(value constructorEffectValue) string {
				locals, stack := []constructorEffectValue{value}, []constructorEffectValue(nil)
				var live []bool
				if part == "stack" {
					locals, stack = nil, locals
				}
				if part == "live projection" {
					live = []bool{true}
				}
				return constructorEffectFrameKey(3, locals, stack, true, live, 0)
			}
			if key(null) == key(unknown) {
				t.Fatal("cached null-only path can hide the non-null observation path")
			}
		})
	}
	for _, v := range []constructorEffectValue{unknown, {kind: 'I', knownNull: true}, {kind: 'L', knownNull: true, knownInt: true}, {kind: 'L', knownNull: true, receiver: true}, {kind: 'L', knownNull: true, retainsReceiver: true}, {kind: 'L', knownNull: true, allocation: 9}} {
		if _, known := constructorKnownNullBranch(core.OP_IFNULL, []constructorEffectValue{v}); known {
			t.Fatal("unknown, primitive or receiver-bearing value borrowed an exact null branch")
		}
		if v.knownNull {
			p := &constructorEffectLoopFrame{locals: []constructorEffectValue{v}, initialized: true}
			if _, _, ok := constructorEffectLoopJoin(p, p.locals, nil, true); ok {
				t.Fatal("invalid null role acquired an inductive loop certificate")
			}
		}
	}
	for _, opcode := range []int{core.OP_IFNULL, core.OP_IFNONNULL} {
		taken, known := constructorKnownNullBranch(opcode, []constructorEffectValue{null})
		if !known || taken != (opcode == core.OP_IFNULL) {
			t.Fatal("exact null has the wrong original branch outcome")
		}
	}
	for _, operands := range [][]constructorEffectValue{nil, {null, null}} {
		if _, known := constructorKnownNullBranch(core.OP_IFNULL, operands); known {
			t.Fatal("wrong operand arity")
		}
	}
	if _, known := constructorKnownNullBranch(core.OP_IFEQ, []constructorEffectValue{null}); known {
		t.Fatal("reference fact licensed a primitive branch")
	}
}

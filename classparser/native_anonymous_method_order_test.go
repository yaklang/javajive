package javaclassparser

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func nativeAnonymousMethodOrderFixture(t *testing.T) *ClassObjectDumper {
	t.Helper()
	files := nativeCompileClasses(t, `class OrderModelOwner{static Runnable one;static{one=new Runnable(){public void run(){}};}static Runnable two(){return new Runnable(){public void run(){}};}{Runnable three=new Runnable(){public void run(){}};}}`)
	root, err := Parse(files["OrderModelOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(root)
	d.nativeAnonymousRoot = &nativeAnonymousFamily{owner: root.GetClassName(), children: map[string]*nativeAnonymousClass{}}
	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := fmt.Sprintf("OrderModelOwner$%d", ordinal)
		child, err := Parse(files[name+".class"])
		if err != nil {
			t.Fatal(err)
		}
		d.nativeAnonymousRoot.children[name] = &nativeAnonymousClass{object: child, ordinal: ordinal}
	}
	return d
}

func nativeAnonymousOrderAtom(name string, ordinal int) *dumpedMethods {
	code := name
	if ordinal > 0 {
		code += fmt.Sprintf(" /*jdec-owned-anonymous-ordinal:%d:OrderModelOwner*/", ordinal)
	}
	return &dumpedMethods{methodName: name, code: code}
}

// Exhaustively enumerate all input and output permutations. The independent
// model simply filters whole permutations by increasing registration labels
// and unchanged initialization subsequence; it does not construct graph edges.
func TestNativeAnonymousMethodOrderMatchesExhaustiveAtomicPermutationModel(t *testing.T) {
	d := nativeAnonymousMethodOrderFixture(t)
	atoms := []*dumpedMethods{nativeAnonymousOrderAtom("<clinit>", 1), nativeAnonymousOrderAtom("two", 2), nativeAnonymousOrderAtom("<initializer>", 3), nativeAnonymousOrderAtom("ordinary", 0)}
	var permutations [][]int
	var enumerate func([]int, []int)
	enumerate = func(prefix, remaining []int) {
		if len(remaining) == 0 {
			permutations = append(permutations, append([]int(nil), prefix...))
			return
		}
		for i, value := range remaining {
			tail := append(append([]int(nil), remaining[:i]...), remaining[i+1:]...)
			enumerate(append(append([]int(nil), prefix...), value), tail)
		}
	}
	enumerate(nil, []int{0, 1, 2, 3})
	if len(permutations) != 24 {
		t.Fatal("incomplete independent domain")
	}
	for _, input := range permutations {
		methods := make([]*dumpedMethods, len(input))
		var initializers []int
		for i, atom := range input {
			methods[i] = atoms[atom]
			if atom == 0 || atom == 2 {
				initializers = append(initializers, atom)
			}
		}
		var expected []*dumpedMethods
		for _, output := range permutations { // ascending original position preference
			var candidate []*dumpedMethods
			var effects, labels []int
			for _, index := range output {
				atom := input[index]
				candidate = append(candidate, atoms[atom])
				if atom < 3 {
					labels = append(labels, atom+1)
				}
				if atom == 0 || atom == 2 {
					effects = append(effects, atom)
				}
			}
			if slices.Equal(labels, []int{1, 2, 3}) && slices.Equal(effects, initializers) {
				expected = candidate
				break
			}
		}
		original := append([]*dumpedMethods(nil), methods...)
		got, known := d.nativeAnonymousMethodOrder(methods)
		if known != (expected != nil) || known && !slices.Equal(got, expected) || !slices.Equal(methods, original) {
			t.Fatalf("input=%v known=%v expected=%v: wrong atomic schedule or input mutation", input, known, expected != nil)
		}
	}
}

func TestNativeAnonymousMethodOrderRefusesMissingOrContradictoryWitnesses(t *testing.T) {
	base := nativeAnonymousMethodOrderFixture(t)
	for _, variant := range []string{"original", "failed group", "wrong owner", "missing child", "foreign child", "unknown ordinal", "duplicate ordinal", "reverse body", "interleaved body", "nil atom", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d := NewClassObjectDumper(base.obj)
			group := *base.nativeAnonymousRoot
			group.children = map[string]*nativeAnonymousClass{}
			for name, child := range base.nativeAnonymousRoot.children {
				group.children[name] = child
			}
			d.nativeAnonymousRoot = &group
			methods := []*dumpedMethods{nativeAnonymousOrderAtom("two", 2), nativeAnonymousOrderAtom("<initializer>", 3), nativeAnonymousOrderAtom("<clinit>", 1)}
			// The original has one static block; use a method atom for the third
			// registration so this positive control has no initialization cycle.
			methods[1].methodName = "third"
			switch variant {
			case "failed group":
				group.failed = true
			case "wrong owner":
				group.owner = "ForeignOwner"
			case "missing child":
				group.children["OrderModelOwner$2"] = nil
			case "foreign child":
				child := *group.children["OrderModelOwner$2"]
				child.object = base.obj
				group.children["OrderModelOwner$2"] = &child
			case "unknown ordinal":
				methods[0] = nativeAnonymousOrderAtom("two", 4)
			case "duplicate ordinal":
				methods[0] = nativeAnonymousOrderAtom("two", 1)
			case "reverse body":
				methods[0].code = nativeAnonymousOrderAtom("two", 2).code + nativeAnonymousOrderAtom("two", 1).code
			case "interleaved body":
				methods[0].code = nativeAnonymousOrderAtom("two", 1).code + nativeAnonymousOrderAtom("two", 3).code
			case "nil atom":
				methods[0] = nil
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if _, known := d.nativeAnonymousMethodOrder(methods); known != (variant == "original") {
				t.Fatalf("unsupported original schedule %q admitted=%v", variant, known)
			}
		})
	}
}

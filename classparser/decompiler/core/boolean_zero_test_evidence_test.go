package core

import "testing"

// A JVM zero branch also accepts an int. Increment evidence must dominate
// this heuristic's decision, independent of successor enumeration or width.
func TestBooleanZeroTestRejectsReachableIncrement(t *testing.T) {
	for _, branch := range []int{OP_IFEQ, OP_IFNE} {
		for _, wide := range []bool{false, true} {
			for _, order := range []bool{false, true} {
				store, load, test := op(OP_ISTORE_0, 0), op(OP_ILOAD_0, 1), op(branch, 2)
				inc, stop := op(OP_IINC, 3), op(OP_RETURN, 4)
				inc.IsWide = wide
				inc.Data = []byte{0, 1}
				if wide {
					inc.Data = []byte{0, 0, 0, 1}
				}
				store.Target = []*OpCode{load}
				load.Target = []*OpCode{test}
				test.Target = []*OpCode{stop, inc}
				if order {
					test.Target = []*OpCode{inc, stop}
				}
				inc.Target = []*OpCode{load}
				if (&Decompiler{}).slotStoreFollowedByBooleanZStore(store, 0) {
					t.Fatalf("integer loop classified boolean: branch=%x wide=%v order=%v", branch, wide, order)
				}
			}
		}
	}
}

func TestBooleanZeroTestDefinitionBoundaries(t *testing.T) {
	for _, scenario := range []string{"zero-test-only", "another-slot-increment", "redefined-before-increment", "unreachable-increment", "cycle-without-increment", "alternate-path-increment", "no-zero-test"} {
		t.Run(scenario, func(t *testing.T) {
			store, load, test := op(OP_ISTORE_0, 0), op(OP_ILOAD_0, 1), op(OP_IFNE, 2)
			inc, stop := op(OP_IINC, 3), op(OP_RETURN, 4)
			inc.Data = []byte{0, 1}
			store.Target, load.Target, test.Target = []*OpCode{load}, []*OpCode{test}, []*OpCode{stop}
			want := true
			switch scenario {
			case "another-slot-increment":
				inc.Data[0] = 1
				test.Target = []*OpCode{inc}
				inc.Target = []*OpCode{stop}
			case "redefined-before-increment":
				kill := op(OP_ISTORE_0, 5)
				test.Target = []*OpCode{kill}
				kill.Target = []*OpCode{inc}
			case "cycle-without-increment":
				test.Target = []*OpCode{load, stop}
			case "alternate-path-increment":
				store.Target = []*OpCode{load, inc}
				want = false
			case "no-zero-test":
				load.Target = []*OpCode{stop}
				want = false
			}
			if got := (&Decompiler{}).slotStoreFollowedByBooleanZStore(store, 0); got != want {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

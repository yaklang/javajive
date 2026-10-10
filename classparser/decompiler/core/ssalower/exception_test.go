package ssalower

import (
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/frametransfer"
	"github.com/yaklang/javajive/classparser/decompiler/core/methodir"
	"github.com/yaklang/javajive/classparser/decompiler/core/ssabuild"
)

func exceptionFixture(t *testing.T) *ssabuild.Function {
	t.Helper()
	// f(a,b): local=7; 1/a; local=11; 1/b; return local;
	// catch ArithmeticException: return local.
	code := []byte{core.OP_BIPUSH, 7, core.OP_ISTORE_2, core.OP_ICONST_1, core.OP_ILOAD_0, core.OP_IDIV, core.OP_POP, core.OP_BIPUSH, 11, core.OP_ISTORE_2, core.OP_ICONST_1, core.OP_ILOAD_1, core.OP_IDIV, core.OP_POP, core.OP_ILOAD_2, core.OP_IRETURN, core.OP_ASTORE_3, core.OP_ILOAD_2, core.OP_IRETURN}
	ir, err := methodir.BuildFromBytes(code, []*core.ExceptionTableEntry{{StartPc: 3, EndPc: 14, HandlerPc: 16, CatchType: 0}}, methodir.MethodMeta{ClassName: "ExceptionProof", Name: "f", Descriptor: "(II)I", IsStatic: true, Bytecode: code}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return fn
}

func TestExceptionSnapshotsExecute7And11(t *testing.T) {
	fn := exceptionFixture(t)
	low, err := Destroy(fn)
	if err != nil {
		t.Fatal(err)
	}
	if len(low.Exception.Sites) != 2 || low.Exception.Sites[0].PC != 5 || low.Exception.Sites[1].PC != 12 {
		t.Fatalf("wrong spill sites: %+v", low.Exception)
	}
	if len(low.Exception.Catch) != 1 {
		t.Fatalf("missing distinct catch binding: %+v", low.Exception)
	}
	for _, site := range low.Exception.Sites {
		if len(site.Spills) != 1 || site.Spills[0].LocalSlot != 2 {
			t.Fatalf("wrong local snapshot: %+v", site)
		}
		if low.Exception.Catch[0].Destination == site.Spills[0].Destination || low.Exception.Catch[0].Destination == site.Spills[0].Source {
			t.Fatal("catch object used before throwing")
		}
		if len(site.Moves) != 1 {
			t.Fatalf("missing executable spill: %+v", site)
		}
	}
	for _, tc := range []struct {
		a, b      int64
		want      int64
		divisions int
	}{{0, 1, 7, 1}, {1, 0, 11, 2}, {1, 1, 11, 2}} {
		original, n1 := executeExceptionFixture(t, fn, nil, tc.a, tc.b)
		lowered, n2 := executeExceptionFixture(t, fn, low, tc.a, tc.b)
		if original != tc.want || lowered != original || n1 != tc.divisions || n2 != n1 {
			t.Fatalf("f(%d,%d): original=%d/%d lowered=%d/%d want=%d/%d", tc.a, tc.b, original, n1, lowered, n2, tc.want, tc.divisions)
		}
	}
	for _, e := range fn.IR.Edges {
		if e.Kind == core.EdgeException && len(low.MovesOn(e.ID)) != 0 {
			t.Fatal("copy executes on exception edge")
		}
	}
	if !low.ThrowsUnmoved() {
		t.Fatal("throw coverage changed")
	}
}

// This small independent bytecode evaluator executes the same input with and
// without the emission plan. Spills read runtime locals; exception dispatch
// reads the handler phi, and the caught object is bound only after division.
func executeExceptionFixture(t *testing.T, fn *ssabuild.Function, low *Lowered, a, b int64) (int64, int) {
	t.Helper()
	locals := []int64{a, b, 0, 0}
	stack := []int64{}
	vars := map[VarID]int64{}
	pc := 0
	divisions := 0
	pop := func() int64 { v := stack[len(stack)-1]; stack = stack[:len(stack)-1]; return v }
	for steps := 0; steps < 100; steps++ {
		if low != nil {
			for _, s := range low.Exception.Sites {
				if int(s.PC) == pc {
					for _, sp := range s.Spills {
						vars[sp.Source] = locals[sp.LocalSlot]
					}
					for _, m := range s.Moves {
						v, ok := vars[m.Src]
						if !ok {
							t.Fatalf("undefined spill source %d", m.Src)
						}
						vars[m.Dst] = v
					}
				}
			}
		}
		op := int(fn.IR.Bytecode[pc])
		pc++
		switch op {
		case core.OP_BIPUSH:
			stack = append(stack, int64(int8(fn.IR.Bytecode[pc])))
			pc++
		case core.OP_ICONST_1:
			stack = append(stack, 1)
		case core.OP_ILOAD_0, core.OP_ILOAD_1, core.OP_ILOAD_2:
			stack = append(stack, locals[op-core.OP_ILOAD_0])
		case core.OP_ISTORE_2:
			locals[2] = pop()
		case core.OP_ASTORE_3:
			locals[3] = pop()
		case core.OP_POP:
			pop()
		case core.OP_IDIV:
			divisions++
			divisor, dividend := pop(), pop()
			if divisor != 0 {
				stack = append(stack, dividend/divisor)
				continue
			}
			if low != nil {
				for _, binding := range low.Exception.Catch {
					vars[binding.Destination] = -12345
				}
				handler, _ := fn.IR.BlockOf(16)
				for _, p := range fn.PhisOf(handler.ID) {
					if p.Slot.Local {
						id, err := low.Values.Phi(p)
						if err != nil {
							t.Fatal(err)
						}
						v, ok := vars[id]
						if !ok {
							t.Fatal("handler reads undefined phi")
						}
						locals[p.Slot.Index] = v
					}
				}
			}
			stack = []int64{-12345}
			pc = 16
		case core.OP_IRETURN:
			return pop(), divisions
		default:
			t.Fatalf("unexpected opcode %d", op)
		}
	}
	t.Fatal("interpreter budget exhausted")
	return 0, 0
}

func TestExceptionPlanRejectsUnsafeAndIncompleteSources(t *testing.T) {
	for _, mode := range []string{"missing operand", "duplicate operand", "effectful origin", "uninitialized", "conversion", "missing before"} {
		t.Run(mode, func(t *testing.T) {
			fn := exceptionFixture(t)
			var p *ssabuild.Phi
			for i := range fn.Phis {
				if fn.Phis[i].Slot.Local {
					p = &fn.Phis[i]
					break
				}
			}
			if p == nil {
				t.Fatal("fixture missing local phi")
			}
			switch mode {
			case "missing operand":
				p.Operands = p.Operands[:1]
			case "duplicate operand":
				p.Operands[1] = p.Operands[0]
			case "effectful origin":
				p.Operands[0].Origin = ssabuild.Origin{Kind: ssabuild.OriginInstr, PC: uint16(p.Operands[0].Edge.From), Slot: 99}
			case "uninitialized":
				p.Type = frametransfer.UninitAt(0)
			case "conversion":
				p.Type = frametransfer.T(frametransfer.Float)
			case "missing before":
				fn.Instructions = nil
			}
			if low, err := Destroy(fn); err == nil || low != nil {
				t.Fatalf("unsafe plan returned: %+v err=%v", low, err)
			}
		})
	}
}
func TestExceptionPlanBudgetIsTransactional(t *testing.T) {
	fn := exceptionFixture(t)
	before := fn.Normalize()
	frames := map[uint16]frametransfer.Frame{}
	for _, v := range fn.Instructions {
		frames[v.PC] = v.Before.Clone()
	}
	low, err := DestroyWithOptions(fn, Options{MaxSpills: 1})
	if err == nil || low != nil {
		t.Fatal("partial plan on exhausted budget")
	}
	if fn.Normalize() != before {
		t.Fatal("SSA mutated on failure")
	}
	for _, v := range fn.Instructions {
		if !reflect.DeepEqual(v.Before, frames[v.PC]) {
			t.Fatal("before frame mutated")
		}
	}
	low, err = Destroy(fn)
	if err != nil || len(low.Exception.Sites) != 2 {
		t.Fatalf("failed request contaminated next plan: %v", err)
	}
}
func TestCriticalEdgeHasSingleMoveOwner(t *testing.T) {
	_, low := ssaDestroy(t, loopExitBytecode(), "()I", nil)
	if len(low.Splits) == 0 {
		t.Fatal("fixture lacks critical edge")
	}
	for _, s := range low.Splits {
		n := 0
		for _, a := range low.Assigns {
			if a.Edge == s.Edge {
				n++
				if len(a.Moves) != 0 {
					t.Fatal("moves duplicated on edge and split")
				}
			}
		}
		if n != 1 {
			t.Fatalf("split edge has %d assignment records", n)
		}
	}
}

func TestSharedHandlerDeduplicatesSameSiteSpills(t *testing.T) {
	base := exceptionFixture(t)
	ex := []*core.ExceptionTableEntry{{StartPc: 3, EndPc: 14, HandlerPc: 16, CatchType: 1}, {StartPc: 3, EndPc: 14, HandlerPc: 16, CatchType: 0}}
	ir, err := methodir.BuildFromBytes(base.IR.Bytecode, ex, methodir.MethodMeta{ClassName: "ExceptionProof", Name: "f", Descriptor: "(II)I", IsStatic: true, Bytecode: base.IR.Bytecode}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fn, err := ssabuild.Build(ir, ssabuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	low, err := DestroyWithOptions(fn, Options{MaxSpills: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(low.Exception.Sites) != 2 || len(low.Exception.Catch) != 1 {
		t.Fatalf("unexpected plan: %+v", low.Exception)
	}
	for _, s := range low.Exception.Sites {
		if len(s.Spills) != 1 || len(s.Coverage) != 2 || s.Coverage[0].Order != 0 || s.Coverage[1].Order != 1 {
			t.Fatalf("duplicate spill or reordered coverage: %+v", s)
		}
	}
}

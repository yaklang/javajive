package javaclassparser

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Enumerate every encoded branch/join destination in this decoded instruction
// inventory. For every admitted layout, compare its certificate with an
// independent instruction-pointer interpreter on both outcomes. This is a CFG
// model, not a claim that arbitrary mutated layouts are verifier-valid JVMs.
func TestNativeAnonymousInitializerControlMatchesIndependentPathModel(t *testing.T) {
	obj, err := Parse(nativeCompileClasses(t, anonymousConditionalInitializerFixture)["ConditionalInitOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	child := nativeAnonymousConstructor(obj, "ConditionalInitOwner", "make", nil)
	if child == nil || child.expressionInitializer == nil {
		t.Fatal("original constructor")
	}
	p := child.expressionInitializer
	var branch, jump *core.OpCode
	index := map[int]int{}
	for i, op := range p.ops {
		index[int(op.CurrentOffset)] = i
		if op.Instr.OpCode == core.OP_IFNULL || op.Instr.OpCode == core.OP_IFNONNULL {
			branch = op
		}
		if op.Instr.OpCode == core.OP_GOTO {
			jump = op
		}
	}
	if branch == nil || jump == nil {
		t.Fatal("original diamond")
	}
	setTarget := func(op *core.OpCode, pc int) {
		n := uint16(int16(pc - int(op.CurrentOffset)))
		op.Data = []byte{byte(n >> 8), byte(n)}
	}
	graphs, admitted := 0, 0
	for _, branchTarget := range p.ops {
		for _, joinTarget := range p.ops {
			graphs++
			setTarget(branch, int(branchTarget.CurrentOffset))
			setTarget(jump, int(joinTarget.CurrentOffset))
			certificate, _, _, closed := nativeAnonymousInitializerControlEvents(obj, p.ops, p.start, nil)
			if !closed {
				continue
			}
			admitted++
			for _, taken := range []bool{false, true} {
				want := []int{}
				seen := map[int]bool{}
				for cursor := p.start; cursor < len(p.ops); {
					if seen[cursor] {
						t.Fatal("accepted cyclic path")
					}
					seen[cursor] = true
					op := p.ops[cursor]
					kind, pc := op.Instr.OpCode, int(op.CurrentOffset)
					if kind == core.OP_RETURN {
						break
					}
					// Independent effect classification and branch decoder.
					switch kind {
					case core.OP_PUTFIELD, core.OP_GETFIELD, core.OP_GETSTATIC, core.OP_NEW, core.OP_NEWARRAY, core.OP_ANEWARRAY, core.OP_MULTIANEWARRAY, core.OP_ARRAYLENGTH, core.OP_CHECKCAST, core.OP_IALOAD, core.OP_LALOAD, core.OP_FALOAD, core.OP_DALOAD, core.OP_AALOAD, core.OP_BALOAD, core.OP_CALOAD, core.OP_SALOAD, core.OP_INVOKEVIRTUAL, core.OP_INVOKESPECIAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE:
						want = append(want, pc)
					}
					if kind == core.OP_GOTO || op == branch && taken {
						delta := int(int16(uint16(op.Data[0])<<8 | uint16(op.Data[1])))
						next, valid := index[pc+delta]
						if !valid || next < p.start {
							t.Fatal("accepted escaping path")
						}
						cursor = next
					} else {
						cursor++
					}
				}
				got := []int{}
				selected := true
				for _, token := range certificate {
					if token >= 0 {
						if selected {
							got = append(got, token)
						}
						continue
					}
					part := (-token - 1) % 3
					switch part {
					case 0:
						selected = !taken
					case 1:
						selected = taken
					case 2:
						selected = true
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("control certificate changed path: taken=%v got=%v want=%v", taken, got, want)
				}
			}
		}
	}
	if graphs != len(p.ops)*len(p.ops) || admitted == 0 {
		t.Fatalf("graphs=%d admitted=%d", graphs, admitted)
	}
	t.Logf("independent CFG destination model: graphs=%d admitted=%d outcomes=%d", graphs, admitted, admitted*2)
}

func TestNativeAnonymousInitializerControlRequiresClosedOriginalDiamonds(t *testing.T) {
	files := nativeCompileClasses(t, anonymousConditionalInitializerFixture)
	for _, variant := range []string{"original", "short jump", "backedge", "outside suffix", "interior target", "missing join", "wrong join", "non-null opcode", "parameter producer", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["ConditionalInitOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			child := nativeAnonymousConstructor(obj, "ConditionalInitOwner", "make", nil)
			if child == nil || child.expressionInitializer == nil {
				t.Fatal("missing original constructor proof")
			}
			p := child.expressionInitializer
			branchIndex := -1
			for i, op := range p.ops {
				if op.Instr.OpCode == core.OP_IFNONNULL || op.Instr.OpCode == core.OP_IFNULL {
					branchIndex = i
				}
			}
			if branchIndex < 0 {
				t.Fatal("missing original branch")
			}
			branch := p.ops[branchIndex]
			branchPC := int(branch.CurrentOffset)
			var jump *core.OpCode
			for _, op := range p.ops[branchIndex:] {
				if op.Instr.OpCode == core.OP_GOTO {
					jump = op
				}
			}
			if jump == nil {
				t.Fatal("missing original join")
			}
			setTarget := func(op *core.OpCode, pc int) {
				n := uint16(int16(pc - int(op.CurrentOffset)))
				op.Data = []byte{byte(n >> 8), byte(n)}
			}
			changeKind := func(op *core.OpCode, kind int) {
				copy := *op.Instr
				copy.OpCode = kind
				op.Instr = &copy
			}
			switch variant {
			case "short jump":
				branch.Data = []byte{0}
			case "backedge":
				setTarget(branch, branchPC)
			case "outside suffix":
				setTarget(branch, int(p.ops[0].CurrentOffset))
			case "interior target":
				setTarget(branch, branchPC+1)
			case "missing join":
				changeKind(jump, core.OP_NOP)
			case "wrong join":
				setTarget(jump, branchPC)
			case "non-null opcode":
				changeKind(branch, core.OP_IFEQ)
			case "parameter producer":
				changeKind(p.ops[branchIndex-1], core.OP_ALOAD_1)
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			events, branches, arms, closed := nativeAnonymousInitializerControlEvents(obj, p.ops, p.start, work)
			if closed != (variant == "original") {
				t.Fatalf("closed=%v", closed)
			}
			if closed && (len(events) == 0 || len(branches) != 1 || len(arms) != 1) {
				t.Fatal("empty control certificate")
			}
		})
	}
}

func TestNativeAnonymousInitializerControlRefusesUnanchoredArmResults(t *testing.T) {
	for _, rhs := range []string{"input==null?1:2", "input==null?ConditionalInitEffects.arm(left,1).hashCode()+1:ConditionalInitEffects.arm(right,2).hashCode()+2"} {
		fixture := strings.Replace(anonymousConditionalInitializerFixture, "final Object chosen=input==null?ConditionalInitEffects.arm(left,1):ConditionalInitEffects.arm(right,2);", "final Object chosen=Integer.valueOf("+rhs+");", 1)
		obj, err := Parse(nativeCompileClasses(t, fixture)["ConditionalInitOwner$1.class"])
		if err != nil {
			t.Fatal(err)
		}
		if child := nativeAnonymousConstructor(obj, "ConditionalInitOwner", "make", nil); child != nil {
			t.Fatal("arm result without exact producer identity accepted")
		}
	}
}

func TestNativeAnonymousInitializerSourceBranchKeepsPredicateAndResultIdentity(t *testing.T) {
	obj, err := Parse(nativeCompileClasses(t, anonymousConditionalInitializerFixture)["ConditionalInitOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	child := nativeAnonymousConstructor(obj, "ConditionalInitOwner", "make", nil)
	if child == nil || child.expressionInitializer == nil {
		t.Fatal("original constructor")
	}
	p := child.expressionInitializer
	for producerPC, original := range p.branches {
		field := &values.RefMember{OriginPC: producerPC, HasOriginPC: true}
		for _, variant := range []string{"original", "reversed predicate", "wrong producer", "no origin", "wrong operator", "different null operand"} {
			t.Run(variant, func(t *testing.T) {
				copy := *field
				predicate := values.EQ
				if original.Instr.OpCode == core.OP_IFNULL {
					predicate = values.NEQ
				}
				var null values.JavaValue = values.JavaNull
				switch variant {
				case "reversed predicate":
					if predicate == values.EQ {
						predicate = values.NEQ
					} else {
						predicate = values.EQ
					}
				case "wrong producer":
					copy.OriginPC++
				case "no origin":
					copy.HasOriginPC = false
				case "wrong operator":
					predicate = values.GT
				case "different null operand":
					null = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.String"))
				}
				condition := values.NewBinaryExpression(&copy, null, predicate, types.NewJavaPrimer(types.JavaBoolean))
				branch, isFallthrough := nativeAnonymousInitializerSourceBranch(p, condition)
				valid := variant == "original" || variant == "reversed predicate"
				if (branch != nil) != valid || valid && isFallthrough != (variant == "original") {
					t.Fatalf("branch=%v fallthrough=%v", branch != nil, isFallthrough)
				}
			})
		}
		for _, pc := range p.armValues[int(original.CurrentOffset)] {
			call := &values.FunctionCallExpression{OriginPC: pc, HasOriginPC: true}
			if !nativeAnonymousInitializerArmValue(call, pc) {
				t.Fatal("original producer lost")
			}
			for _, value := range []values.JavaValue{&values.FunctionCallExpression{OriginPC: pc + 1, HasOriginPC: true}, &values.FunctionCallExpression{OriginPC: pc}, values.JavaNull, &values.JavaExpression{Values: []values.JavaValue{call, values.JavaNull}, Op: values.EQ, Typ: types.NewJavaPrimer(types.JavaBoolean)}} {
				if nativeAnonymousInitializerArmValue(value, pc) {
					t.Fatal("same text or unanchored result substituted")
				}
			}
		}
	}
}

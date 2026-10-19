package core

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func captureMotionProgram(words []int, producer, crossed, prefix []int) (*Decompiler, values.JavaValue, *Node, *Node, *values.JavaRef) {
	object := types.NewJavaClass("java.lang.Object")
	refs := make([]*values.JavaRef, 4)
	for i := range refs {
		typ := types.JavaType(object)
		if len(words) == 3 && words[1] == 2 {
			if i == 0 {
				typ = types.NewJavaPrimer(types.JavaLong)
			} else if i == 1 {
				typ = types.NewJavaPrimer(types.JavaDouble)
			}
		}
		refs[i] = values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		refs[i].IsParam = true
		word := 6 // a SAM formal, outside the physical capture prefix
		if i < 3 {
			word = words[i]
		}
		refs[i].MarkOriginalParameter(word)
	}
	call := func(name string, reads []int) *values.FunctionCallExpression {
		result := &values.FunctionCallExpression{ClassName: "authored.Operations", FunctionName: name, IsStatic: true, FuncType: &types.JavaFuncType{ReturnType: object}}
		for _, read := range reads {
			result.Arguments = append(result.Arguments, refs[read])
			result.FuncType.ParamTypes = append(result.FuncType.ParamTypes, refs[read].Type())
		}
		return result
	}
	value := call("produce", producer)
	local := values.NewJavaRef(utils.NewRootVariableId(), value, object)
	source := NewNode(statements.NewAssignStatement(local, value, true))
	consumer := call("consume", prefix)
	consumer.Arguments = append(consumer.Arguments, local)
	consumer.FuncType.ParamTypes = append(consumer.FuncType.ParamTypes, object)
	target := NewNode(statements.NewExpressionStatement(consumer))
	if len(crossed) == 0 {
		source.AddNext(target)
	} else {
		middle := NewNode(statements.NewExpressionStatement(call("middle", crossed)))
		source.AddNext(middle)
		middle.AddNext(target)
	}
	return &Decompiler{LambdaCaptureOrderSlots: append([]int(nil), words...)}, value, source, target, local
}

func TestLambdaCaptureMotionPreservesIndependentLexicalReadTrace(t *testing.T) {
	// Exhaust the bounded read grammar: zero, one or two reads from three
	// captured formals and one SAM formal, in each of the producer, intervening
	// statement and consumer prefix. The oracle compares actual first-use
	// traces before and after substitution, independently of the motion rule.
	vectors := [][]int{nil}
	for a := 0; a < 4; a++ {
		vectors = append(vectors, []int{a})
		for b := 0; b < 4; b++ {
			vectors = append(vectors, []int{a, b})
		}
	}
	firstUses := func(groups ...[]int) []int {
		seen := map[int]bool{}
		var result []int
		for _, group := range groups {
			for _, item := range group {
				if item != 3 && !seen[item] {
					seen[item] = true
					result = append(result, item)
				}
			}
		}
		return result
	}
	accepted, inverted, considered := 0, 0, 0
	for _, words := range [][]int{{0, 1, 2}, {0, 2, 4}} {
		for _, producer := range vectors {
			for _, middle := range vectors {
				for _, prefix := range vectors {
					before := firstUses(producer, middle, prefix)
					if !reflect.DeepEqual(before, []int{0, 1, 2}) {
						continue
					}
					considered++
					after := firstUses(middle, prefix, producer)
					d, value, source, target, local := captureMotionProgram(words, producer, middle, prefix)
					allowed := d.lambdaCaptureInliningOrderClosed(value, source, target, local)
					if !reflect.DeepEqual(before, after) {
						inverted++
						if allowed {
							t.Fatalf("motion changed physical capture discovery: producer=%v middle=%v prefix=%v words=%v before=%v after=%v", producer, middle, prefix, words, before, after)
						}
					} else if allowed {
						accepted++
					}
				}
			}
		}
	}
	if considered == 0 || inverted == 0 || accepted == 0 {
		t.Fatalf("missing independent counterexamples or accepted safe movement: %d/%d/%d", considered, inverted, accepted)
	}
	t.Logf("checked %d original capture traces, including %d inverted substitutions and %d accepted safe movements", considered, inverted, accepted)
}

func TestLambdaCaptureMotionConstrainsExistingInlineProofAndFailsClosed(t *testing.T) {
	for _, variant := range []string{"ordinary", "captured", "duplicate words", "missing entry", "negative word", "wide tail outside frame", "cycle", "join", "opaque", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, value, source, target, local := captureMotionProgram([]int{0, 1, 2}, []int{0}, nil, []int{1})
			source.Id, target.Id = 1, 2
			origins := map[int]*OpCode{1: {Instr: &Instruction{OpCode: OP_ASTORE}, CurrentOffset: 10}, 2: {Instr: &Instruction{OpCode: OP_INVOKESTATIC}, CurrentOffset: 11}}
			switch variant {
			case "ordinary":
				d.LambdaCaptureOrderSlots = nil
			case "duplicate words":
				d.LambdaCaptureOrderSlots = []int{0, 0}
			case "missing entry":
				d.LambdaCaptureOrderSlots = []int{1}
			case "negative word":
				d.LambdaCaptureOrderSlots = []int{-1}
			case "wide tail outside frame":
				d.LambdaCaptureOrderSlots = []int{0, 255}
			case "cycle":
				target.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Arguments[0] = value.(*values.FunctionCallExpression).Arguments[0]
				source.Next = []*Node{source}
			case "join":
				target.Statement.(*statements.ExpressionStatement).Expression.(*values.FunctionCallExpression).Arguments[0] = value.(*values.FunctionCallExpression).Arguments[0]
				NewNode(statements.NewExpressionStatement(values.JavaNull)).AddNext(target)
			case "opaque":
				value = &values.CustomValue{}
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if allowed := d.canInlineValue(value, source, target, origins, local, nil); allowed != (variant == "ordinary") {
				t.Fatalf("existing motion proof=%v for %s", allowed, variant)
			}
		})
	}
}

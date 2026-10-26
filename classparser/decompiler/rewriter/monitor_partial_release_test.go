package rewriter

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"reflect"
	"testing"
)

// Evaluate the small typed source tree independently of rendering. The exact
// effect PCs distinguish a released path from a still-held shared suffix.
func TestMonitorMixedOpenBranchesRetainAbruptRelease(t *testing.T) {
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			tr, branch := monitorContinuationFixture()
			outer := branch.Condition.(*values.JavaRef)
			inner := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaBoolean))
			inner.IsParam = true
			effect := func(pc int) statements.Statement {
				return statements.NewExpressionStatement(&values.FunctionCallExpression{ClassName: "p.Actions", FunctionName: "effect", Descriptor: "()V", IsStatic: true, Kind: values.InvokeStatic, HasOriginPC: true, OriginPC: pc})
			}
			originalA, originalB := branch.IfBody, branch.ElseBody
			branch.IfBody = []statements.Statement{statements.NewIfStatement(inner, originalA, nil)}
			branch.ElseBody = []statements.Statement{effect(34)}
			tr.TryBody = append([]statements.Statement{branch, effect(35)}, originalB...)
			prefix, held, tail, known := originalMonitorContinuations(tr, 3)
			if !known {
				t.Fatal("complete original monitor was refused")
			}
			state := map[*utils.VariableId]any{outer.Id: a, inner.Id: b}
			var eval func(values.JavaValue) any
			eval = func(v values.JavaValue) any {
				switch x := v.(type) {
				case *values.JavaLiteral:
					return x.Data
				case *values.JavaRef:
					return state[x.Id]
				case *values.JavaExpression:
					return reflect.DeepEqual(eval(x.Values[0]), eval(x.Values[1]))
				default:
					t.Fatalf("unsupported expression %T", v)
					return nil
				}
			}
			var effects []int
			var run func([]statements.Statement) string
			run = func(body []statements.Statement) string {
				for _, st := range body {
					switch x := st.(type) {
					case *statements.AssignStatement:
						if x.JavaValue != nil {
							state[x.LeftValue.(*values.JavaRef).Id] = eval(x.JavaValue)
						}
					case *statements.ExpressionStatement:
						effects = append(effects, x.Expression.(*values.FunctionCallExpression).OriginPC)
					case *statements.IfStatement:
						arm := x.ElseBody
						if eval(x.Condition) == true {
							arm = x.IfBody
						}
						if exit := run(arm); exit != "" {
							return exit
						}
					case *statements.DoWhileStatement:
						if eval(x.ConditionValue) != false {
							t.Fatal("region repeats")
						}
						if exit := run(x.Body); exit != "" && exit != "break "+x.Label {
							return exit
						}
					case *statements.CustomStatement:
						kind, label, ok := x.SourceTransferProtocol()
						if !ok || kind != "break" || label == "" {
							t.Fatal("unknown release barrier")
						}
						return kind + " " + label
					case *statements.ReturnStatement:
						return "return"
					default:
						t.Fatalf("unsupported statement %T", st)
					}
				}
				return ""
			}
			if run(prefix) != "" || run(held) != "" || run(tail) != "return" {
				t.Fatal("wrong continuation completion")
			}
			want := []int{35, 32, 41}
			if !a {
				want = []int{34, 35, 32, 41}
			} else if b {
				want = []int{12, 21}
			}
			if !reflect.DeepEqual(effects, want) {
				t.Fatalf("outer=%v inner=%v effects=%v want=%v", a, b, effects, want)
			}
		}
	}
}

func TestMonitorReleaseBarrierPreservesExistingTransferTargets(t *testing.T) {
	for _, variant := range []string{"inner label collision", "external break", "external continue", "foreign label", "label inventory bound"} {
		t.Run(variant, func(t *testing.T) {
			tr, branch := monitorContinuationFixture()
			base := "jdec$monitor$body$3"
			loop := statements.NewDoWhileStatement(values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{statements.NewSourceTransferStatement("break", base)})
			loop.Label = base
			switch variant {
			case "inner label collision":
				branch.IfBody = append([]statements.Statement{loop}, branch.IfBody...)
			case "external break":
				branch.IfBody = append([]statements.Statement{statements.NewSourceTransferStatement("break", "")}, branch.IfBody...)
			case "external continue":
				branch.IfBody = append([]statements.Statement{statements.NewSourceTransferStatement("continue", "")}, branch.IfBody...)
			case "foreign label":
				loop.Body = []statements.Statement{statements.NewSourceTransferStatement("break", "foreign")}
				branch.IfBody = append([]statements.Statement{loop}, branch.IfBody...)
			case "label inventory bound":
				for i := 0; i < 2050; i++ {
					branch.IfBody = append(branch.IfBody, loop)
				}
			}
			_, held, _, known := originalMonitorContinuations(tr, 3)
			if known != (variant == "inner label collision") {
				t.Fatalf("proved=%v", known)
			}
			if known {
				region := held[0].(*statements.DoWhileStatement)
				if region.Label == base || loop.Label != base {
					t.Fatal("release barrier captured an existing target")
				}
			}
		})
	}
}

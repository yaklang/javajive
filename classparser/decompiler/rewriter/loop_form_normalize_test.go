package rewriter

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func loopTestBreak() *statements.CustomStatement {
	return statements.NewSourceTransferStatement("break", "")
}

func TestNormalizeLoopFormsDoWhileGuardToWhile(t *testing.T) {
	condition := values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
	body := statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	guard := statements.NewIfStatement(condition,
		[]statements.Statement{body},
		[]statements.Statement{loopTestBreak()})
	loop := statements.NewDoWhileStatement(
		values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		[]statements.Statement{guard})
	out := NormalizeLoopForms([]statements.Statement{loop})
	if len(out) != 1 {
		t.Fatalf("statements = %d", len(out))
	}
	while, ok := out[0].(*statements.WhileStatement)
	if !ok {
		t.Fatalf("loop = %T, want WhileStatement", out[0])
	}
	if len(while.Body) != 1 {
		t.Fatalf("while body = %d", len(while.Body))
	}
}

func TestNormalizeLoopFormsDoesNotTouchLabeledDoWhile(t *testing.T) {
	loop := statements.NewDoWhileStatement(
		values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)),
		[]statements.Statement{statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))})
	loop.Label = "OUTER"
	out := NormalizeLoopForms([]statements.Statement{loop})
	if _, ok := out[0].(*statements.DoWhileStatement); !ok {
		t.Fatalf("labeled loop changed to %T", out[0])
	}
}

func TestNormalizeLoopFormsRecoversForLoop(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	ref := useTestRef("var30", "int")
	ref.ResetVarType(intType)
	init := statements.NewAssignStatement(ref, values.NewJavaLiteral(0, intType), true)
	condition := values.NewBinaryExpression(ref, values.NewJavaLiteral(10, intType), values.LT,
		types.NewJavaPrimer(types.JavaBoolean))
	update := statements.NewExpressionStatement(values.NewUnaryExpression(ref, values.INC, intType))
	body := statements.NewExpressionStatement(values.NewJavaLiteral(1, intType))
	while := statements.NewWhileStatement(condition, []statements.Statement{body, update})
	out := NormalizeLoopForms([]statements.Statement{init, while})
	if len(out) != 1 {
		t.Fatalf("statements = %d, want one for statement", len(out))
	}
	forSt, ok := out[0].(*statements.ForStatement)
	if !ok {
		t.Fatalf("loop = %T, want ForStatement", out[0])
	}
	if len(forSt.SubStatements) != 1 || forSt.InitVar == nil || forSt.EndExp == nil {
		t.Fatalf("malformed for: %#v", forSt)
	}
	text := forSt.String(&class_context.ClassContext{})
	if !strings.HasPrefix(text, "for(") || strings.Contains(text, "if (") {
		t.Fatalf("for rendering = %s", text)
	}
}

func TestNormalizeLoopFormsKeepsContinueOrEscapingDeclaration(t *testing.T) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	ref := useTestRef("var31", "int")
	ref.ResetVarType(intType)
	init := statements.NewAssignStatement(ref, values.NewJavaLiteral(0, intType), true)
	condition := values.NewBinaryExpression(ref, values.NewJavaLiteral(10, intType), values.LT,
		types.NewJavaPrimer(types.JavaBoolean))
	update := statements.NewExpressionStatement(values.NewUnaryExpression(ref, values.INC, intType))
	continueStmt := statements.NewSourceTransferStatement("continue", "")
	while := statements.NewWhileStatement(condition, []statements.Statement{continueStmt, update})
	out := NormalizeLoopForms([]statements.Statement{init, while})
	if len(out) != 2 {
		t.Fatalf("continue loop was converted to for")
	}

	use := statements.NewExpressionStatement(ref)
	out = NormalizeLoopForms([]statements.Statement{init, while, use})
	if len(out) != 3 {
		t.Fatalf("escaping declaration was converted to for")
	}
}

func evalLoopIntStatements(t *testing.T, sts []statements.Statement, maxSteps int) map[string]int {
	t.Helper()
	env := map[string]int{}
	steps := 0
	var evalVal func(values.JavaValue) int
	var evalBool func(values.JavaValue) bool

	tick := func() {
		steps++
		if steps > maxSteps {
			t.Fatalf("loop evaluation exceeded %d steps (env=%v)", maxSteps, env)
		}
	}

	assignRef := func(target values.JavaValue, val int) int {
		ref := plainLoopRef(target)
		if ref == nil || ref.VarUid == "" {
			t.Fatalf("expected plain loop ref target, got %T", target)
		}
		env[ref.VarUid] = val
		return val
	}

	evalBool = func(v values.JavaValue) bool {
		return evalVal(v) != 0
	}

	evalVal = func(v values.JavaValue) int {
		tick()
		if v == nil {
			t.Fatalf("nil value in evaluator")
		}
		if slot, ok := v.(*values.SlotValue); ok {
			return evalVal(slot.GetValue())
		}
		switch x := v.(type) {
		case *values.JavaRef:
			if x == nil || x.VarUid == "" {
				t.Fatalf("nil/unnamed JavaRef in evaluator")
			}
			return env[x.VarUid]
		case *values.JavaLiteral:
			switch d := x.Data.(type) {
			case int:
				return d
			case int64:
				return int(d)
			case bool:
				if d {
					return 1
				}
				return 0
			default:
				t.Fatalf("unsupported literal %T (%v)", x.Data, x.Data)
			}
		case *values.AssignmentExpression:
			val := evalVal(x.Value)
			return assignRef(x.Target, val)
		case *values.JavaExpression:
			switch x.Op {
			case values.INC, "++":
				ref := plainLoopRef(x.Values[0])
				if ref == nil {
					t.Fatalf("INC target is not plain ref: %T", x.Values[0])
				}
				if len(x.Values) == 1 {
					env[ref.VarUid]++
					return env[ref.VarUid]
				}
				old := env[ref.VarUid]
				env[ref.VarUid] = old + 1
				return old
			case values.DEC, "--":
				ref := plainLoopRef(x.Values[0])
				if ref == nil {
					t.Fatalf("DEC target is not plain ref: %T", x.Values[0])
				}
				if len(x.Values) == 1 {
					env[ref.VarUid]--
					return env[ref.VarUid]
				}
				old := env[ref.VarUid]
				env[ref.VarUid] = old - 1
				return old
			case values.ADD:
				return evalVal(x.Values[0]) + evalVal(x.Values[1])
			case values.SUB:
				return evalVal(x.Values[0]) - evalVal(x.Values[1])
			case values.LT:
				if evalVal(x.Values[0]) < evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.LTE:
				if evalVal(x.Values[0]) <= evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.GT:
				if evalVal(x.Values[0]) > evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.GTE:
				if evalVal(x.Values[0]) >= evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.EQ:
				if evalVal(x.Values[0]) == evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.NEQ:
				if evalVal(x.Values[0]) != evalVal(x.Values[1]) {
					return 1
				}
				return 0
			case values.LOGICAL_AND:
				if !evalBool(x.Values[0]) {
					return 0
				}
				if evalBool(x.Values[1]) {
					return 1
				}
				return 0
			case values.LOGICAL_OR:
				if evalBool(x.Values[0]) {
					return 1
				}
				if evalBool(x.Values[1]) {
					return 1
				}
				return 0
			case values.Not:
				if evalBool(x.Values[0]) {
					return 0
				}
				return 1
			default:
				t.Fatalf("unsupported expression op %q", x.Op)
			}
		default:
			t.Fatalf("unsupported value %T", v)
		}
		return 0
	}

	var execStmts func([]statements.Statement) string
	var execOne func(statements.Statement) string

	execOne = func(st statements.Statement) string {
		tick()
		if st == nil {
			return ""
		}
		switch s := st.(type) {
		case *statements.AssignStatement:
			return func() string {
				assignRef(s.LeftValue, evalVal(s.JavaValue))
				return ""
			}()
		case *statements.ExpressionStatement:
			evalVal(s.Expression)
			return ""
		case *statements.IfStatement:
			if evalBool(s.Condition) {
				return execStmts(s.IfBody)
			}
			return execStmts(s.ElseBody)
		case *statements.WhileStatement:
			for evalBool(s.ConditionValue) {
				tick()
				sig := execStmts(s.Body)
				if sig == "break" {
					return ""
				}
			}
			return ""
		case *statements.DoWhileStatement:
			for {
				tick()
				sig := execStmts(s.Body)
				if sig == "break" {
					return ""
				}
				if !evalBool(s.ConditionValue) {
					return ""
				}
			}
		case *statements.ForStatement:
			if s.InitVar != nil {
				execOne(s.InitVar)
			}
			for evalBool(s.Condition.Condition) {
				tick()
				sig := execStmts(s.SubStatements)
				if sig == "break" {
					return ""
				}
				if s.EndExp != nil {
					execOne(s.EndExp)
				}
			}
			return ""
		case *statements.CustomStatement:
			kind, label, known := s.SourceTransferProtocol()
			if known && label == "" && (kind == "break" || kind == "continue") {
				return kind
			}
			t.Fatalf("unsupported CustomStatement in evaluator")
		default:
			t.Fatalf("unsupported statement %T in evaluator", st)
		}
		return ""
	}

	execStmts = func(list []statements.Statement) string {
		for _, st := range list {
			if sig := execOne(st); sig != "" {
				return sig
			}
		}
		return ""
	}

	if sig := execStmts(sts); sig != "" {
		t.Fatalf("uncaught loop transfer %q at top level", sig)
	}
	return env
}

func makeIntLoopVar(name string) (*values.JavaRef, types.JavaType) {
	intType := types.NewJavaPrimer(types.JavaInteger)
	ref := useTestRef(name, "int")
	ref.ResetVarType(intType)
	return ref, intType
}

func TestNormalizeLoopFormsConstPrefixBehavioralEquivalenceAndWriteGuards(t *testing.T) {
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	intType := types.NewJavaPrimer(types.JavaInteger)

	condCases := []struct {
		name           string
		buildCond      func(x, i *values.JavaRef) values.JavaValue
		wantRounds     int
		wantI          int
		naiveBugRounds int
		naiveBugI      int
	}{
		{
			name: "condition_post_inc_short_circuit",
			// x++ < 1 && i++ < 2  (returns rounds=2, i=3; naive hoisting yields rounds=1, i=1)
			buildCond: func(x, i *values.JavaRef) values.JavaValue {
				left := values.NewBinaryExpression(
					values.NewBinaryExpression(x, values.NewJavaLiteral(1, intType), values.INC, intType),
					values.NewJavaLiteral(1, intType), values.LT, boolType)
				right := values.NewBinaryExpression(
					values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType),
					values.NewJavaLiteral(2, intType), values.LT, boolType)
				return values.NewBinaryExpression(left, right, values.LOGICAL_AND, boolType)
			},
			wantRounds:     2,
			wantI:          3,
			naiveBugRounds: 1,
			naiveBugI:      1,
		},
		{
			name: "condition_pre_inc_short_circuit",
			// ++x <= 1 && i++ < 3  (returns rounds=3, i=4; naive hoisting yields rounds=1, i=1)
			buildCond: func(x, i *values.JavaRef) values.JavaValue {
				left := values.NewBinaryExpression(
					values.NewUnaryExpression(values.NewSlotValue(x, intType), values.INC, intType),
					values.NewJavaLiteral(1, intType), values.LTE, boolType)
				right := values.NewBinaryExpression(
					values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType),
					values.NewJavaLiteral(3, intType), values.LT, boolType)
				return values.NewBinaryExpression(left, right, values.LOGICAL_AND, boolType)
			},
			wantRounds:     3,
			wantI:          4,
			naiveBugRounds: 1,
			naiveBugI:      1,
		},
		{
			name: "condition_pre_dec_short_circuit",
			// --x == -1 && i++ < 2  (returns rounds=2, i=3; naive hoisting yields rounds=1, i=1)
			buildCond: func(x, i *values.JavaRef) values.JavaValue {
				left := values.NewBinaryExpression(
					values.NewUnaryExpression(values.NewSlotValue(x, intType), values.DEC, intType),
					values.NewJavaLiteral(-1, intType), values.EQ, boolType)
				right := values.NewBinaryExpression(
					values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType),
					values.NewJavaLiteral(2, intType), values.LT, boolType)
				return values.NewBinaryExpression(left, right, values.LOGICAL_AND, boolType)
			},
			wantRounds:     2,
			wantI:          3,
			naiveBugRounds: 1,
			naiveBugI:      1,
		},
		{
			name: "condition_embedded_assignment",
			// (x = x + 1) == 1 && i++ < 2  (returns rounds=2, i=3; naive hoisting yields rounds=1, i=1)
			buildCond: func(x, i *values.JavaRef) values.JavaValue {
				assignX := values.NewAssignmentExpression(x,
					values.NewBinaryExpression(x, values.NewJavaLiteral(1, intType), values.ADD, intType), 0, nil)
				left := values.NewBinaryExpression(assignX, values.NewJavaLiteral(1, intType), values.EQ, boolType)
				right := values.NewBinaryExpression(
					values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType),
					values.NewJavaLiteral(2, intType), values.LT, boolType)
				return values.NewBinaryExpression(left, right, values.LOGICAL_AND, boolType)
			},
			wantRounds:     2,
			wantI:          3,
			naiveBugRounds: 1,
			naiveBugI:      1,
		},
	}

	for _, tc := range condCases {
		t.Run(tc.name, func(t *testing.T) {
			makeAST := func() (x, i, rounds *values.JavaRef, initX statements.Statement, cond values.JavaValue, stmts []statements.Statement) {
				x, _ = makeIntLoopVar("x")
				i, _ = makeIntLoopVar("i")
				rounds, _ = makeIntLoopVar("rounds")
				initI := statements.NewAssignStatement(i, values.NewJavaLiteral(0, intType), true)
				initRounds := statements.NewAssignStatement(rounds, values.NewJavaLiteral(0, intType), true)
				initX = statements.NewAssignStatement(x, values.NewJavaLiteral(0, intType), true)
				cond = tc.buildCond(x, i)
				body := []statements.Statement{
					statements.NewExpressionStatement(values.NewBinaryExpression(rounds, values.NewJavaLiteral(1, intType), values.INC, intType)),
				}
				guard := statements.NewIfStatement(cond, body, []statements.Statement{loopTestBreak()})
				loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{initX, guard})
				stmts = []statements.Statement{initI, initRounds, loop}
				return x, i, rounds, initX, cond, stmts
			}

			_, i, rounds, _, _, orig := makeAST()
			origEnv := evalLoopIntStatements(t, orig, 500)
			if origEnv[rounds.VarUid] != tc.wantRounds || origEnv[i.VarUid] != tc.wantI {
				t.Fatalf("orig eval = (rounds=%d, i=%d), want (%d, %d)", origEnv[rounds.VarUid], origEnv[i.VarUid], tc.wantRounds, tc.wantI)
			}

			_, bi, brounds, bInitX, bCond, _ := makeAST()
			naiveHoisted := []statements.Statement{
				statements.NewAssignStatement(bi, values.NewJavaLiteral(0, intType), true),
				statements.NewAssignStatement(brounds, values.NewJavaLiteral(0, intType), true),
				bInitX,
				statements.NewWhileStatement(bCond, []statements.Statement{
					statements.NewExpressionStatement(values.NewBinaryExpression(brounds, values.NewJavaLiteral(1, intType), values.INC, intType)),
				}),
			}
			bugEnv := evalLoopIntStatements(t, naiveHoisted, 500)
			if bugEnv[brounds.VarUid] != tc.naiveBugRounds || bugEnv[bi.VarUid] != tc.naiveBugI {
				t.Fatalf("naive hoisted control = (rounds=%d, i=%d), want (%d, %d)", bugEnv[brounds.VarUid], bugEnv[bi.VarUid], tc.naiveBugRounds, tc.naiveBugI)
			}

			_, ni, nrounds, _, _, normIn := makeAST()
			normalized := NormalizeLoopForms(normIn)
			if len(normalized) != 3 {
				t.Fatalf("normalized statements = %d, want 3 (must not hoist %s)", len(normalized), tc.name)
			}
			if _, ok := normalized[2].(*statements.DoWhileStatement); !ok {
				t.Fatalf("normalized[2] = %T, want *statements.DoWhileStatement", normalized[2])
			}
			normEnv := evalLoopIntStatements(t, normalized, 500)
			if normEnv[nrounds.VarUid] != tc.wantRounds || normEnv[ni.VarUid] != tc.wantI {
				t.Fatalf("normalized eval = (rounds=%d, i=%d), want (%d, %d)", normEnv[nrounds.VarUid], normEnv[ni.VarUid], tc.wantRounds, tc.wantI)
			}
		})
	}

	for _, bc := range []struct {
		name        string
		op          string
		nested      bool
		wantAcc     int
		naiveBugAcc int
	}{
		{name: "body_inc_expr_stmt", op: values.INC, nested: false, wantAcc: 3, naiveBugAcc: 6},
		{name: "body_dec_expr_stmt", op: values.DEC, nested: false, wantAcc: -3, naiveBugAcc: -6},
		{name: "body_nested_prefix_inc_expr", op: values.INC, nested: true, wantAcc: 3, naiveBugAcc: 6},
	} {
		t.Run(bc.name, func(t *testing.T) {
			makeAST := func() (x, i, acc *values.JavaRef, initX statements.Statement, cond values.JavaValue, body []statements.Statement, stmts []statements.Statement) {
				x, _ = makeIntLoopVar("x")
				i, _ = makeIntLoopVar("i")
				acc, _ = makeIntLoopVar("acc")
				initI := statements.NewAssignStatement(i, values.NewJavaLiteral(0, intType), true)
				initAcc := statements.NewAssignStatement(acc, values.NewJavaLiteral(0, intType), true)
				initX = statements.NewAssignStatement(x, values.NewJavaLiteral(0, intType), true)
				cond = values.NewBinaryExpression(i, values.NewJavaLiteral(3, intType), values.LT, boolType)
				if bc.nested {
					upd := values.NewUnaryExpression(values.NewSlotValue(x, intType), bc.op, intType)
					body = []statements.Statement{
						statements.NewAssignStatement(acc, values.NewBinaryExpression(acc, upd, values.ADD, intType), false),
						statements.NewExpressionStatement(values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType)),
					}
				} else {
					body = []statements.Statement{
						statements.NewExpressionStatement(values.NewBinaryExpression(x, values.NewJavaLiteral(1, intType), bc.op, intType)),
						statements.NewAssignStatement(acc, values.NewBinaryExpression(acc, x, values.ADD, intType), false),
						statements.NewExpressionStatement(values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType)),
					}
				}
				guard := statements.NewIfStatement(cond, body, []statements.Statement{loopTestBreak()})
				loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{initX, guard})
				stmts = []statements.Statement{initI, initAcc, loop}
				return
			}

			_, _, acc, _, _, _, orig := makeAST()
			if got := evalLoopIntStatements(t, orig, 500)[acc.VarUid]; got != bc.wantAcc {
				t.Fatalf("orig acc = %d, want %d", got, bc.wantAcc)
			}

			_, bi, bacc, bInitX, bCond, bBody, _ := makeAST()
			bugEnv := evalLoopIntStatements(t, []statements.Statement{
				statements.NewAssignStatement(bi, values.NewJavaLiteral(0, intType), true),
				statements.NewAssignStatement(bacc, values.NewJavaLiteral(0, intType), true),
				bInitX,
				statements.NewWhileStatement(bCond, bBody),
			}, 500)
			if bugEnv[bacc.VarUid] != bc.naiveBugAcc {
				t.Fatalf("naive hoisted control acc = %d, want %d", bugEnv[bacc.VarUid], bc.naiveBugAcc)
			}

			_, _, nacc, _, _, _, normIn := makeAST()
			normalized := NormalizeLoopForms(normIn)
			if len(normalized) != 3 {
				t.Fatalf("normalized len = %d, want 3 (must not hoist %s)", len(normalized), bc.name)
			}
			if _, ok := normalized[2].(*statements.DoWhileStatement); !ok {
				t.Fatalf("normalized[2] = %T, want *statements.DoWhileStatement", normalized[2])
			}
			if got := evalLoopIntStatements(t, normalized, 500)[nacc.VarUid]; got != bc.wantAcc {
				t.Fatalf("normalized acc = %d, want %d", got, bc.wantAcc)
			}
		})
	}

	t.Run("fail_closed_opaque_and_name_collision_guards", func(t *testing.T) {
		x, _ := makeIntLoopVar("dupVar")
		i, _ := makeIntLoopVar("i")
		// Case A: opaque CustomStatement inside body fails closed.
		initX := statements.NewAssignStatement(x, values.NewJavaLiteral(0, intType), true)
		cond := values.NewBinaryExpression(i, values.NewJavaLiteral(3, intType), values.LT, boolType)
		opaqueBody := []statements.Statement{statements.NewCustomStatement(func(*class_context.ClassContext) string { return "" }, nil)}
		loopA := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{
			initX, statements.NewIfStatement(cond, opaqueBody, []statements.Statement{loopTestBreak()}),
		})
		if out := NormalizeLoopForms([]statements.Statement{loopA}); len(out) != 1 || func() bool { _, ok := out[0].(*statements.DoWhileStatement); return !ok }() {
			t.Fatalf("opaque body was hoisted: %#v", out)
		}

		// Case B: opaque CustomValue in condition fails closed.
		opaqueCond := &values.CustomValue{Flag: "opaque_probe"}
		cleanBody := []statements.Statement{statements.NewExpressionStatement(values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType))}
		loopB := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{
			statements.NewAssignStatement(x, values.NewJavaLiteral(0, intType), true),
			statements.NewIfStatement(opaqueCond, cleanBody, []statements.Statement{loopTestBreak()}),
		})
		if out := NormalizeLoopForms([]statements.Statement{loopB}); len(out) != 1 || func() bool { _, ok := out[0].(*statements.DoWhileStatement); return !ok }() {
			t.Fatalf("opaque condition was hoisted: %#v", out)
		}

		// Case C: same-spelling sibling declaration in remaining statements fails closed.
		siblingSameName, _ := makeIntLoopVar("dupVar")
		loopC := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{
			statements.NewAssignStatement(x, values.NewJavaLiteral(0, intType), true),
			statements.NewIfStatement(cond, cleanBody, []statements.Statement{loopTestBreak()}),
		})
		siblingDecl := statements.NewAssignStatement(siblingSameName, values.NewJavaLiteral(7, intType), true)
		if out := NormalizeLoopForms([]statements.Statement{loopC, siblingDecl}); len(out) != 2 || func() bool { _, ok := out[0].(*statements.DoWhileStatement); return !ok }() {
			t.Fatalf("prefix hoisted across same-spelling remaining declaration: %#v", out)
		}
	})

	t.Run("read_only_const_prefix_still_normalizes_and_preserves_semantics", func(t *testing.T) {
		limit, _ := makeIntLoopVar("limit")
		i, _ := makeIntLoopVar("i")
		acc, _ := makeIntLoopVar("acc")
		initAcc := statements.NewAssignStatement(acc, values.NewJavaLiteral(0, intType), true)
		initI := statements.NewAssignStatement(i, values.NewJavaLiteral(0, intType), true)
		initLimit := statements.NewAssignStatement(limit, values.NewJavaLiteral(3, intType), true)
		cond := values.NewBinaryExpression(i, limit, values.LT, boolType)
		body := []statements.Statement{
			statements.NewAssignStatement(acc, values.NewBinaryExpression(acc, limit, values.ADD, intType), false),
			statements.NewExpressionStatement(values.NewBinaryExpression(i, values.NewJavaLiteral(1, intType), values.INC, intType)),
		}
		guard := statements.NewIfStatement(cond, body, []statements.Statement{loopTestBreak()})
		loop := statements.NewDoWhileStatement(values.NewJavaLiteral(true, boolType), []statements.Statement{initLimit, guard})
		orig := []statements.Statement{initAcc, initI, loop}

		origEnv := evalLoopIntStatements(t, orig, 500)
		if origEnv[acc.VarUid] != 9 || origEnv[i.VarUid] != 3 {
			t.Fatalf("orig read-only prefix eval = (acc=%d, i=%d), want (9, 3)", origEnv[acc.VarUid], origEnv[i.VarUid])
		}

		normalized := NormalizeLoopForms(orig)
		if len(normalized) != 3 {
			t.Fatalf("normalized statements = %d, want 3 (initAcc, initLimit, forLoop)", len(normalized))
		}
		if _, ok := normalized[2].(*statements.ForStatement); !ok {
			t.Fatalf("normalized[2] = %T, want *statements.ForStatement", normalized[2])
		}
		normEnv := evalLoopIntStatements(t, normalized, 500)
		if normEnv[acc.VarUid] != 9 || normEnv[i.VarUid] != 3 {
			t.Fatalf("normalized read-only prefix eval = (acc=%d, i=%d), want (9, 3)", normEnv[acc.VarUid], normEnv[i.VarUid])
		}
	})
}

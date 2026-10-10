package rewriter

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestDeclarationPlacementKeepsDistinctScopedIdentities(t *testing.T) {
	for _, pair := range [][2]string{{"java.nio.ByteBuffer", "java.nio.file.Path"}, {"java.lang.String", "java.lang.String"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			makeArm := func(typ string) (*values.JavaRef, *statements.AssignStatement, *statements.IfStatement) {
				ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass(typ))
				ref.Id.SetName("var7")
				definition := statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, ref.Type()), true)
				arm := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{definition, statements.NewReturnStatement(ref)}}
				return ref, definition, arm
			}
			first, firstDecl, left := makeArm(pair[0])
			second, secondDecl, right := makeArm(pair[1])
			root := []statements.Statement{left, right}
			placeCrossScopeDeclarations(&root, nil, true)
			if len(root) != 2 || root[0] != left || root[1] != right || !firstDecl.IsFirst || !secondDecl.IsFirst {
				t.Fatal("already dominating declarations were hoisted because unrelated ids share a printed name")
			}
			if first.Id == second.Id || first.Id.String() != "var7" || second.Id.String() != "var7" {
				t.Fatal("distinct identities or temporary probe names leaked")
			}
		})
	}
}

func TestDeclarationDominanceReadsDependenciesWithoutRendering(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass("Payload"))
	ref.Id.SetName("var7")
	renders := 0
	capture := values.NewCustomValue(func(*class_context.ClassContext) string {
		renders++
		return "() -> null" // A rendering is not the capture dependency graph.
	}, func() types.JavaType { return types.NewJavaClass("java.util.function.Supplier") })
	capture.Flag, capture.CapturesKnown, capture.Captures = "lambda", true, []values.JavaValue{ref}
	throw := &statements.CustomStatement{ThrownValue: ref, StringFunc: func(*class_context.ClassContext) string {
		renders++
		return "throw null;"
	}}
	for _, use := range []statements.Statement{statements.NewReturnStatement(capture), throw} {
		if topLevelDeclDominatesAllUses([]statements.Statement{use}, ref.Id) {
			t.Fatal("an explicit capture/throw dependency was lost because it was absent from rendered text")
		}
	}
	if renders != 0 || ref.Id.String() != "var7" {
		t.Fatal("typed declaration analysis rendered an expression or changed a variable name")
	}
}

func TestDeclarationDominanceDoesNotCountProbeLiterals(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass("Payload"))
	ref.Id.SetName("var7")
	for _, code := range []string{`"__jdec_dom_probe__"`, `helper("__jdec_dom_probe__")`, `0 /* __jdec_dom_probe__ */`, "0 // __jdec_dom_probe__"} {
		opaque := values.NewCustomValue(func(*class_context.ClassContext) string { return code }, func() types.JavaType { return types.NewJavaClass("java.lang.Object") })
		if !topLevelDeclDominatesAllUses([]statements.Statement{statements.NewReturnStatement(opaque)}, ref.Id) {
			t.Fatalf("literal/comment was mistaken for an identity reference: %s", code)
		}
	}
}

func TestDeclarationDominanceKeepsOpaqueIdentityAndRestoresName(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass("Payload"))
	ref.Id.SetName("var7")
	other := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, ref.Type())
	other.Id.SetName("var7")
	for _, target := range []*values.JavaRef{ref, other} {
		opaque := values.NewCustomValue(func(ctx *class_context.ClassContext) string {
			return `helper("__jdec_dom_probe__", ` + target.String(ctx) + `)`
		}, ref.Type)
		got := topLevelDeclDominatesAllUses([]statements.Statement{statements.NewReturnStatement(opaque)}, ref.Id)
		if got != (target != ref) || ref.Id.String() != "var7" || other.Id.String() != "var7" {
			t.Fatal("opaque fallback confused distinct identities or leaked its temporary name")
		}
	}
	opaque := values.NewCustomValue(func(*class_context.ClassContext) string { panic("unavailable legacy renderer") }, ref.Type)
	if topLevelDeclDominatesAllUses([]statements.Statement{statements.NewReturnStatement(opaque)}, ref.Id) || ref.Id.String() != "var7" {
		t.Fatal("failed legacy rendering must preserve the name and conservative uncovered-use result")
	}
}

func TestDeclarationDominanceChecksInitializerBeforeBinding(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaPrimer(types.JavaInteger))
	definition := statements.NewAssignStatement(ref, ref, true)
	for _, root := range [][]statements.Statement{{definition}, {&statements.ForStatement{InitVar: definition}}} {
		if topLevelDeclDominatesAllUses(root, ref.Id) {
			t.Fatal("a declaration cannot supply the old value used by its own initializer")
		}
	}
}

func TestDeclarationDominanceSkipsStringCharacterCommentAndTextBlockTokens(t *testing.T) {
	const name = "__jdec_dom_probe__"
	for _, code := range []string{
		`"__jdec_dom_probe__"`, `'__jdec_dom_probe__'`, `/* __jdec_dom_probe__ */`, "// __jdec_dom_probe__",
		"\"\"\"\n__jdec_dom_probe__\n\"\"\"", `"escaped \\"" /* __jdec_dom_probe__ */`,
		`prefix$__jdec_dom_probe__`, `__jdec_dom_probe__$suffix`, `变量__jdec_dom_probe__`,
	} {
		if codeContainsIdentifier(code, name) {
			t.Fatalf("non-identifier text counted as a variable dependency: %q", code)
		}
	}
	for _, code := range []string{`f(__jdec_dom_probe__)`, "// ignored\n__jdec_dom_probe__", `"ignored" + __jdec_dom_probe__`} {
		if !codeContainsIdentifier(code, name) {
			t.Fatalf("actual identifier dependency was skipped: %q", code)
		}
	}
}

func TestDeclarationPlacementStillCoversEscapingIdentity(t *testing.T) {
	ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaClass("java.lang.String"))
	definition := statements.NewAssignStatement(ref, values.NewJavaLiteral(nil, ref.Type()), true)
	branch := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{definition}}
	root := []statements.Statement{branch, statements.NewReturnStatement(ref)}
	placeCrossScopeDeclarations(&root, nil, true)
	if len(root) != 3 || definition.IsFirst || root[0].(*statements.AssignStatement).LeftValue.(*values.JavaRef).Id != ref.Id {
		t.Fatal("actual undominated identity was not given one enclosing declaration")
	}
}

func TestDeclarationDominanceIncludesControlHeads(t *testing.T) {
	for _, shape := range []string{"if", "while", "do while", "switch", "monitor", "for condition", "for update"} {
		t.Run(shape, func(t *testing.T) {
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, types.NewJavaPrimer(types.JavaBoolean))
			body := []statements.Statement{statements.NewAssignStatement(ref, values.NewJavaLiteral(false, ref.Type()), true)}
			var container statements.Statement = &statements.IfStatement{Condition: ref, IfBody: body}
			switch shape {
			case "while":
				container = &statements.WhileStatement{ConditionValue: ref, Body: body}
			case "do while":
				container = &statements.DoWhileStatement{ConditionValue: ref, Body: body}
			case "switch":
				container = &statements.SwitchStatement{Value: ref, Cases: []*statements.CaseItem{{Body: body}}}
			case "monitor":
				container = &statements.SynchronizedStatement{Argument: ref, Body: body}
			case "for condition":
				container = &statements.ForStatement{Condition: &statements.ConditionStatement{Condition: ref}, SubStatements: body}
			case "for update":
				container = &statements.ForStatement{EndExp: statements.NewExpressionStatement(ref), SubStatements: body}
			}
			if topLevelDeclDominatesAllUses([]statements.Statement{container}, ref.Id) {
				t.Fatal("a body declaration cannot cover an enclosing control-head use")
			}
		})
	}
}

func TestDeclarationDominanceBoundsImplicitScopes(t *testing.T) {
	for _, shape := range []string{"for initializer", "catch entry"} {
		t.Run(shape, func(t *testing.T) {
			var typ types.JavaType = types.NewJavaClass("java.lang.Exception")
			if shape == "for initializer" {
				typ = types.NewJavaPrimer(types.JavaInteger)
			}
			ref := values.NewJavaRef(utils.NewRootVariableId().Next(), nil, typ)
			use := statements.NewReturnStatement(ref)
			var container statements.Statement = &statements.TryCatchStatement{Exception: []*values.JavaRef{ref}, CatchBodies: [][]statements.Statement{{use}}}
			if shape == "for initializer" {
				container = &statements.ForStatement{InitVar: statements.NewAssignStatement(ref, values.NewJavaLiteral(0, ref.Type()), true), Condition: &statements.ConditionStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}, EndExp: statements.NewAssignStatement(ref, values.NewJavaLiteral(1, ref.Type()), false), SubStatements: []statements.Statement{use}}
			}
			if !topLevelDeclDominatesAllUses([]statements.Statement{container}, ref.Id) {
				t.Fatal("entry definition did not cover its own child scope")
			}
			if topLevelDeclDominatesAllUses([]statements.Statement{container, use}, ref.Id) {
				t.Fatal("child entry definition escaped into a following sibling")
			}
		})
	}
}

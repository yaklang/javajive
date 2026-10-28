package rewriter

import (
	"slices"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func finallyScopeFixture() (*statements.TryCatchStatement, *statements.AssignStatement, *statements.ExpressionStatement) {
	tr := primaryResourceFixture()
	ref := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Object"))
	ref.WebDeclType = ref.Type().Copy()
	store := statements.NewAssignStatement(ref, values.JavaNull, true)
	store.HasOriginPC, store.OriginPC = true, 10
	call := layerTestCall(100)
	call.Descriptor = "(Ljava/lang/Object;)V"
	call.FuncType = types.NewJavaFuncType(call.Descriptor, []types.JavaType{ref.Type()}, types.NewJavaPrimer(types.JavaVoid))
	call.Arguments = []values.JavaValue{ref}
	tail := statements.NewExpressionStatement(call)
	tr.TryBody = []statements.Statement{store, tr.TryBody[0], tail}
	return tr, store, tail
}

func TestFinallySourceContinuationHoistsOnlyProvedDeclarationScope(t *testing.T) {
	for _, variant := range []string{"original", "final declaration", "no web declaration", "changed web type", "missing original store", "parameter", "this", "self initializer", "duplicate declaration", "nested declaration", "opaque initializer", "cyclic initializer", "catch falls through", "unknown tail", "protected tail"} {
		t.Run(variant, func(t *testing.T) {
			tr, store, tail := finallyScopeFixture()
			ref := store.LeftValue.(*values.JavaRef)
			switch variant {
			case "final declaration":
				store.Final = true
			case "no web declaration":
				ref.WebDeclType = nil
			case "changed web type":
				ref.WebDeclType = types.NewJavaClass("java.lang.String")
			case "missing original store":
				store.HasOriginPC = false
			case "parameter":
				ref.IsParam = true
			case "this":
				ref.IsThis = true
			case "self initializer":
				store.JavaValue = ref
			case "duplicate declaration":
				copy := *store
				copy.OriginPC = 11
				tr.TryBody = append([]statements.Statement{&copy}, tr.TryBody...)
			case "nested declaration":
				tr.TryBody[0] = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{store}, nil)
			case "opaque initializer":
				store.JavaValue = &values.CustomValue{}
			case "cyclic initializer":
				call := layerTestCall(10)
				call.Arguments = []values.JavaValue{call}
				store.JavaValue = call
			case "catch falls through":
				tr.CatchBodies[0] = tr.CatchBodies[0][:2]
			case "unknown tail":
				tail.Expression.(*values.FunctionCallExpression).HasOriginPC = false
			case "protected tail":
				tail.Expression.(*values.FunctionCallExpression).OriginPC = 12
			}
			before := slices.Clone(tr.TryBody)
			original := *store
			region, known := RecoverCatchAllFinallyForSource(tr)
			want := variant == "original" || variant == "final declaration"
			if known != want {
				t.Fatalf("source scope proved=%v want=%v", known, want)
			}
			if !slices.Equal(before, tr.TryBody) || store.IsFirst != original.IsFirst || store.JavaValue != original.JavaValue || store.HasOriginPC != original.HasOriginPC {
				t.Fatal("source scope view changed the canonical declaration or initializer")
			}
			if !want {
				return
			}
			if len(region.SourcePrologue) != 1 || len(region.SourceEpilogue) != 1 || region.SourceEpilogue[0] != tail || len(region.TryBody) != 1 {
				t.Fatal("source view lost a declaration, initializer or original continuation")
			}
			declaration := region.SourcePrologue[0].(*statements.AssignStatement)
			assignment := region.TryBody[0].(*statements.AssignStatement)
			if !declaration.IsDeclare || declaration.IsFirst || declaration.JavaValue != nil || declaration.HasOriginPC || declaration.LeftValue != ref || declaration.Final != original.Final {
				t.Fatal("the source prologue must be a bare declaration of the same binding")
			}
			if assignment == store || assignment.IsDeclare || assignment.IsFirst || assignment.Final || assignment.LeftValue != store.LeftValue || assignment.JavaValue != store.JavaValue || assignment.OriginPC != store.OriginPC || !assignment.HasOriginPC {
				t.Fatal("the protected initializer was moved, copied as an effect or rebound")
			}
		})
	}
}

func TestFinallySourceScopeRequiresActualBinderUses(t *testing.T) {
	tr, store, tail := finallyScopeFixture()
	other := values.NewJavaRef(utils.NewRootVariableId(), store.LeftValue, store.LeftValue.Type())
	tail.Expression.(*values.FunctionCallExpression).Arguments = []values.JavaValue{other}
	// The value history mentions the declaration, but the actual operand has
	// another binder. Neither history nor an eventual printed name grants it.
	if prefix, declarations, known := finallyContinuationDeclarationScope(tr, []statements.Statement{tail}, nil); known || prefix != nil || declarations != nil {
		t.Fatal("source scope followed a value history instead of the actual use")
	}
}

func TestFinallySourceScopeMovesBareDeclarationWithRetainedOriginalStore(t *testing.T) {
	for _, scenario := range []string{"bare", "initializing", "nested original store", "missing store receipt", "missing declaration receipt", "changed seed", "changed source type", "fake bare initializer", "duplicate declaration", "metadata-only store"} {
		t.Run(scenario, func(t *testing.T) {
			tr, store, tail := finallyScopeFixture()
			ref := store.LeftValue.(*values.JavaRef)
			seed := layerTestCall(10)
			seed.Descriptor = "()Ljava/lang/Object;"
			seed.FuncType = types.NewJavaFuncType(seed.Descriptor, nil, types.NewJavaClass("java.lang.Object"))
			store.JavaValue, ref.Val, ref.WebDeclType = seed, seed, nil
			if scenario != "missing declaration receipt" {
				ref.MarkOriginalLocalDeclaration(10, 2, seed)
			}
			if scenario != "missing store receipt" {
				store.MarkOriginalLocalStore(10, 2)
			}
			declaration := &statements.AssignStatement{LeftValue: ref, IsDeclare: true}
			if scenario != "initializing" {
				store.IsFirst = false
				tr.TryBody = append([]statements.Statement{declaration}, tr.TryBody...)
			}
			switch scenario {
			case "nested original store":
				tr.TryBody[1] = statements.NewIfStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), []statements.Statement{store}, nil)
			case "changed seed":
				store.JavaValue = values.JavaNull
			case "changed source type":
				ref.ResetVarType(types.NewJavaClass("java.lang.String"))
			case "fake bare initializer":
				declaration.JavaValue = seed
			case "duplicate declaration":
				copy := *declaration
				tr.TryBody = append([]statements.Statement{&copy}, tr.TryBody...)
			case "metadata-only store":
				tr.EntryInitializers = append(tr.EntryInitializers, store)
				tr.TryBody = append(tr.TryBody[:1:1], tr.TryBody[2:]...)
			}
			before := slices.Clone(tr.TryBody)
			prefix, prologue, known := finallyContinuationDeclarationScope(tr, []statements.Statement{tail}, nil)
			want := scenario == "bare" || scenario == "initializing" || scenario == "nested original store"
			if known != want || !slices.Equal(before, tr.TryBody) {
				t.Fatalf("scope=%v want=%v or changed original tree", known, want)
			}
			if want {
				if len(prologue) != 1 || prologue[0].(*statements.AssignStatement).LeftValue != ref || !finallyContinuationKeepsLexicalBindings([]statements.Statement{prefix}, []statements.Statement{tail}) {
					t.Fatal("declaration move lost original binding")
				}
				if store.JavaValue != seed || store.OriginPC != 10 {
					t.Fatal("initializer moved or changed")
				}
			}
		})
	}
}

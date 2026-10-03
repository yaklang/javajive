package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestCheckedEscapePreciseRethrowRequiresUnchangedOriginalCatchBinding(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "CatchProofMetadata.java")
	source := `public class CatchProofMetadata {static void effect()throws Throwable{} static void run()throws Throwable{try{effect();}catch(Throwable failure){throw failure;}}}`
	if e := os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); e != nil {
		t.Fatalf("compile %v\n%s", e, out)
	}
	raw := readClassBytes(t, dir, "CatchProofMetadata")
	for _, variant := range []string{"original unchanged parameter", "different printed name same ID", "same printed name different ID", "parameter reassigned", "copied ref writes parameter", "folded assignment", "for header assignment", "while condition assignment", "switch arm assignment", "lambda captured assignment", "alias rethrow", "cast rethrow", "decoded entry value", "wrong decoded entry", "missing entry origin", "missing handler metadata", "wrong handler entry", "catch type mismatch", "missing catch ID", "method parameter is not catch binding", "missing throw origin", "conflicting copy of throw PC", "opaque closure", "value cycle", "statement cycle", "proof budget", "reassigned same-ID nested catch"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(obj)
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			var method *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				if pool.GetUtf8(int(m.NameIndex)).Value == "run" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil || len(code.ExceptionTable) != 1 {
				t.Fatal("original catch table absent")
			}
			dec := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
			if e = dec.ParseOpcode(); e != nil {
				t.Fatal(e)
			}
			throwPC := -1
			for _, op := range dec.Opcodes() {
				if op.Instr.OpCode == core.OP_ATHROW {
					throwPC = int(op.CurrentOffset)
				}
			}
			if throwPC < 0 {
				t.Fatal("original ATHROW absent")
			}
			caught := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("java.lang.Throwable"))
			caught.Id.SetName("failure")
			thrown := &statements.CustomStatement{ThrownValue: caught, HasOriginPC: true, OriginPC: throwPC}
			entry := code.ExceptionTable[0]
			tr := &statements.TryCatchStatement{Exception: []*values.JavaRef{caught}, Handlers: []statements.CatchHandler{{EntryPC: int(entry.HandlerPc), ProtectedRanges: [][2]int{{int(entry.StartPc), int(entry.EndPc)}}}}, CatchBodies: [][]statements.Statement{{thrown}}}
			body := []statements.Statement{tr}
			assign := statements.NewAssignStatement(caught, values.JavaNull, false)
			switch variant {
			case "different printed name same ID":
				copy := *caught
				copy.VarUid = "different"
				thrown.ThrownValue = &copy
			case "same printed name different ID":
				other := values.NewJavaRef(utils.NewRootVariableId(), nil, caught.Type())
				other.Id.SetName("failure")
				thrown.ThrownValue = other
			case "parameter reassigned":
				tr.CatchBodies[0] = []statements.Statement{assign, thrown}
			case "copied ref writes parameter":
				copy := *caught
				assign.LeftValue = &copy
				tr.CatchBodies[0] = []statements.Statement{assign, thrown}
			case "folded assignment":
				tr.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(&values.AssignmentExpression{Target: caught, Value: values.JavaNull}), thrown}
			case "for header assignment":
				tr.CatchBodies[0] = []statements.Statement{&statements.ForStatement{InitVar: assign, SubStatements: []statements.Statement{thrown}}}
			case "while condition assignment":
				tr.CatchBodies[0] = []statements.Statement{&statements.WhileStatement{ConditionValue: &values.AssignmentExpression{Target: caught, Value: values.JavaNull}, Body: []statements.Statement{thrown}}}
			case "switch arm assignment":
				tr.CatchBodies[0] = []statements.Statement{&statements.SwitchStatement{Value: values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), Cases: []*statements.CaseItem{{Body: []statements.Statement{assign, thrown}}}}}
			case "lambda captured assignment":
				tr.CatchBodies[0] = []statements.Statement{statements.NewExpressionStatement(&values.CustomValue{Flag: "lambda", CapturesKnown: true, Captures: []values.JavaValue{&values.AssignmentExpression{Target: caught, Value: values.JavaNull}}}), thrown}
			case "alias rethrow":
				alias := values.NewJavaRef(utils.NewRootVariableId(), caught, caught.Type())
				tr.CatchBodies[0] = []statements.Statement{statements.NewAssignStatement(alias, caught, true), thrown}
				thrown.ThrownValue = alias
			case "cast rethrow":
				thrown.ThrownValue = &values.CastExpression{Value: caught, TargetType: caught.Type()}
			case "decoded entry value", "wrong decoded entry", "missing entry origin":
				v := &values.CustomValue{Flag: "exception", HasOriginPC: variant != "missing entry origin", OriginPC: int(entry.HandlerPc)}
				if variant == "wrong decoded entry" {
					v.OriginPC++
				}
				thrown.ThrownValue = v
			case "missing handler metadata":
				tr.Handlers = nil
			case "wrong handler entry":
				tr.Handlers[0].EntryPC++
			case "catch type mismatch":
				tr.Handlers[0].CatchAll = true
			case "missing catch ID":
				caught.Id = nil
			case "method parameter is not catch binding":
				caught.IsParam = true
			case "missing throw origin":
				thrown.HasOriginPC = false
			case "conflicting copy of throw PC":
				body = append(body, &statements.CustomStatement{ThrownValue: values.NewJavaRef(utils.NewRootVariableId(), nil, caught.Type()), OriginPC: throwPC, HasOriginPC: true})
			case "opaque closure":
				tr.CatchBodies[0] = append([]statements.Statement{statements.NewExpressionStatement(&values.CustomValue{})}, thrown)
			case "value cycle":
				v := &values.CustomValue{Flag: "lambda", CapturesKnown: true}
				v.Captures = []values.JavaValue{v}
				tr.CatchBodies[0] = append([]statements.Statement{statements.NewExpressionStatement(v)}, thrown)
			case "statement cycle":
				loop := &statements.WhileStatement{ConditionValue: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				loop.Body = []statements.Statement{loop, thrown}
				tr.CatchBodies[0] = []statements.Statement{loop}
			case "proof budget":
				for i := 0; i < 9000; i++ {
					tr.CatchBodies[0] = append(tr.CatchBodies[0], statements.NewReturnStatement(nil))
				}
			case "reassigned same-ID nested catch":
				inner := *tr
				inner.CatchBodies = [][]statements.Statement{{assign, thrown}}
				tr.CatchBodies = [][]statements.Statement{{&inner}}
			}
			got := d.preciseCatchRethrows(body, code)
			want := variant == "original unchanged parameter" || variant == "different printed name same ID" || variant == "decoded entry value"
			if got[throwPC] != want {
				t.Fatalf("precise=%v want%v map=%v", got[throwPC], want, got)
			}
			// Removing redundant broad classification must not suppress the original
			// checked invoke, even when its declaration is absent from caller throws.
			if variant == "original unchanged parameter" {
				var attrs []AttributeInfo
				for _, a := range method.Attributes {
					if _, checked := a.(*ExceptionsAttribute); !checked {
						attrs = append(attrs, a)
					}
				}
				method.Attributes = attrs
				for _, checked := range []bool{false, true} {
					exceptions := []string(nil)
					if checked {
						exceptions = []string{"java/io/IOException"}
					}
					d.FuncCtx = &class_context.ClassContext{InvocationMetadata: func(owner string) (callbinding.Class, bool) {
						if owner == "CatchProofMetadata" {
							return callbinding.Class{Name: owner, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "effect", Desc: "()V", ExceptionsKnown: true, Exceptions: exceptions}}}, true
						}
						return callbinding.Class{}, false
					}}
					bridge, e := d.methodNeedsCheckedEscape(code, body, method)
					if e != nil || bridge != checked {
						t.Fatalf("original checked=%v bridge=%v err%v", checked, bridge, e)
					}
				}
			}
		})
	}
}

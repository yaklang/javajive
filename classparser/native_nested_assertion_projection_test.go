package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// A fresh real compiler packet is mandatory even when a source chain looks like
// an assertion. False-arm effects, changed instruction identities, duplicate
// flag reads and opaque leaves must never gain permission from the new shape.
func TestNativeNestedAssertionProjectionRequiresOriginalClosedPacket(t *testing.T) {
	files := nativeCompileDebugClasses(t, nativeMemberAssertionFixture, "none")
	for _, variant := range []string{"nested", "deep", "direct", "prefix", "two prefixes", "nested prefixes", "prefix OR", "prefix eager", "nil prefix", "cyclic prefix", "prefix duplicate flag", "inner else", "outer else", "shared return", "different return", "missing return origin", "return value", "unproved return join", "extra effect", "empty arm", "nil arm", "opaque leaf", "cycle", "depth", "wrong flag origin", "duplicate flag", "wrong throw origin", "missing throw origin", "wrong new origin", "wrong invoke origin", "wrong descriptor", "wrong call owner", "wrong call kind", "wrong allocation owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(files["MemberAssertionOwner$Child.class"])
			if e != nil {
				t.Fatal(e)
			}
			plan, ok := nativeMemberAssertionProof(obj, "MemberAssertionOwner", nil)
			if !ok {
				t.Fatal("original compiler protocol")
			}
			sites := plan.reads["check(ZLjava/lang/Object;)V"]
			if len(sites) != 1 {
				t.Fatal("original assertion site")
			}
			var pc int
			var packet *nativeAssertionPacket
			for p, v := range sites {
				pc, packet = p, v
			}
			boolType := types.NewJavaPrimer(types.JavaBoolean)
			flag := &values.JavaClassMember{Name: "MemberAssertionOwner$Child", Member: nativeAssertionField, Description: "Z", JavaType: boolType, OriginPC: pc, HasOriginPC: true}
			allocation := &values.NewExpression{JavaType: types.NewJavaClass("java.lang.AssertionError"), OriginPC: packet.newPC, HasOriginPC: true}
			call := &values.FunctionCallExpression{Object: allocation, ClassName: "java.lang.AssertionError", FunctionName: "<init>", Descriptor: packet.descriptor, Kind: values.InvokeSpecial, OriginPC: packet.invokePC, HasOriginPC: true, Arguments: []values.JavaValue{values.NewJavaLiteral("payload", types.NewJavaClass("java.lang.String"))}}
			allocation.ConstructorCall = call
			thrown := statements.NewThrowStatement(allocation)
			thrown.OriginPC = packet.throwPC
			thrown.HasOriginPC = true
			predicate := values.NewJavaLiteral(false, boolType)
			inner := &statements.IfStatement{Condition: predicate, IfBody: []statements.Statement{thrown}}
			outer := &statements.IfStatement{Condition: values.NewUnaryExpression(flag, values.Not, boolType), IfBody: []statements.Statement{inner}}
			var work *workbudget.Budget
			switch variant {
			case "prefix", "two prefixes", "nested prefixes", "prefix OR", "prefix eager", "nil prefix", "cyclic prefix", "prefix duplicate flag":
				prefix := values.JavaValue(values.NewJavaLiteral(true, boolType))
				op := values.LOGICAL_AND
				switch variant {
				case "two prefixes":
					prefix = values.NewBinaryExpression(prefix, values.NewJavaLiteral(false, boolType), values.LOGICAL_AND, boolType)
				case "nested prefixes":
					outer.Condition = values.NewBinaryExpression(prefix, outer.Condition, values.LOGICAL_AND, boolType)
				case "prefix OR":
					op = values.LOGICAL_OR
				case "prefix eager":
					op = values.AND
				case "nil prefix":
					prefix = nil
				case "cyclic prefix":
					x := values.NewBinaryExpression(prefix, prefix, values.LOGICAL_AND, boolType)
					x.Values[0] = x
					prefix = x
				case "prefix duplicate flag":
					prefix = values.NewUnaryExpression(flag, values.Not, boolType)
				}
				if prefix == nil {
					outer.Condition = &values.JavaExpression{Op: op, Values: []values.JavaValue{nil, outer.Condition}, Typ: boolType}
				} else {
					outer.Condition = values.NewBinaryExpression(prefix, outer.Condition, op, boolType)
				}
			case "deep":
				outer.IfBody = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, boolType), IfBody: []statements.Statement{inner}}}
			case "direct":
				outer.IfBody = []statements.Statement{thrown}
			case "shared return", "different return", "missing return origin", "return value", "unproved return join":
				outer.ElseBody = []statements.Statement{&statements.ReturnStatement{OriginPC: packet.joinPC, HasOriginPC: true}}
				r := &statements.ReturnStatement{OriginPC: packet.joinPC, HasOriginPC: true}
				inner.ElseBody = []statements.Statement{r}
				switch variant {
				case "different return":
					r.OriginPC++
				case "missing return origin":
					r.HasOriginPC = false
				case "return value":
					r.JavaValue = predicate
				case "unproved return join":
					packet.voidReturnJoin = false
				}
			case "outer else":
				outer.ElseBody = []statements.Statement{&statements.ReturnStatement{}}
			case "inner else":
				inner.ElseBody = []statements.Statement{&statements.ReturnStatement{}}
			case "extra effect":
				inner.IfBody = append([]statements.Statement{&statements.ReturnStatement{}}, inner.IfBody...)
			case "empty arm":
				inner.IfBody = nil
			case "nil arm":
				inner.IfBody = []statements.Statement{(*statements.CustomStatement)(nil)}
			case "opaque leaf":
				x := statements.NewSourceTransferStatement("break", "")
				x.ThrownValue = allocation
				x.OriginPC = thrown.OriginPC
				x.HasOriginPC = true
				inner.IfBody = []statements.Statement{x}
			case "cycle":
				inner.IfBody = []statements.Statement{inner}
			case "depth":
				for i := 0; i < 65; i++ {
					outer.IfBody = []statements.Statement{&statements.IfStatement{Condition: predicate, IfBody: outer.IfBody}}
				}
			case "wrong flag origin":
				flag.OriginPC++
			case "duplicate flag":
				inner.Condition = values.NewUnaryExpression(flag, values.Not, boolType)
			case "wrong throw origin":
				thrown.OriginPC++
			case "missing throw origin":
				thrown.HasOriginPC = false
			case "wrong new origin":
				allocation.OriginPC++
			case "wrong invoke origin":
				call.OriginPC++
			case "wrong descriptor":
				call.Descriptor = "()V"
			case "wrong call owner":
				call.ClassName = "java.lang.IllegalStateException"
			case "wrong call kind":
				call.Kind = values.InvokeVirtual
			case "wrong allocation owner":
				allocation.JavaType = types.NewJavaClass("java.lang.IllegalStateException")
			case "budget":
				work = workbudget.New(context.Background(), workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(obj)
			d.nativeSourceAssertions = plan
			d.Work = work
			projected, e := d.prepareNativeAssertions("check", "(ZLjava/lang/Object;)V", []statements.Statement{outer})
			want := variant == "nested" || variant == "deep" || variant == "direct" || variant == "shared return" || variant == "prefix" || variant == "two prefixes" || variant == "nested prefixes"
			if (e == nil) != want {
				t.Fatalf("projection=%v want=%t", e, want)
			}
			if want {
				count := 1
				if variant == "shared return" {
					count = 2
				}
				if len(projected) != count {
					t.Fatal("original statement placement")
				}
				leaf := projected[0]
				if variant == "prefix" || variant == "two prefixes" || variant == "nested prefixes" {
					guard, ok := leaf.(*statements.IfStatement)
					if !ok || guard.Condition == nil || len(guard.IfBody) != 1 || len(guard.ElseBody) != 0 {
						t.Fatal("prefix is not retained outside assertion")
					}
					leaf = guard.IfBody[0]
				}
				if _, ok := leaf.(*nativeAssertStatement); !ok {
					t.Fatal("compiler assertion not reconstructed")
				}
			}
			// No caller-visible mutation is allowed, including failed partial proofs.
			if outer.Condition == nil || allocation.ConstructorCall != call || inner.Condition == nil {
				t.Fatal("original source graph mutated")
			}
		})
	}
}

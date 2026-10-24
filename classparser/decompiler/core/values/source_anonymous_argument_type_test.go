package values

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestSourceAnonymousArgumentTypeRequiresReadOnlyAllocationCertificate(t *testing.T) {
	for _, variant := range []string{"original", "class parent", "missing callback", "unknown certificate", "foreign class", "foreign method", "missing new", "missing invoke", "wrong receiver", "wrong owner", "static", "virtual", "missing special flag", "malformed Signature", "own formals", "foreign formal", "multiple interfaces", "deep Signature", "typed nil type", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			allocation := &NewExpression{JavaType: types.NewJavaClass("probe.Owner$1"), OriginPC: 3, HasOriginPC: true}
			call := &FunctionCallExpression{ClassName: "probe.Owner$1", FunctionName: "<init>", Descriptor: "(Lprobe/Owner;)V", Object: allocation, Kind: InvokeSpecial, IsSpecialInvoke: true, OriginPC: 7, HasOriginPC: true}
			allocation.ConstructorCall = call
			ctx := &class_context.ClassContext{ClassName: "probe.Owner", FunctionName: "make", CurrentMethodDesc: "()V", ClassSig: "<T:Ljava/lang/Object;>Ljava/lang/Object;", ClassTypeParams: []string{"T"}, TypeParams: []string{"T"}}
			signature := "Ljava/lang/Object;Lprobe/Mapper<TT;TT;>;"
			ctx.SourceAnonymousParentSignature = func(owner, descriptor, sourceOwner, name, desc string, newPC, pc int) (string, bool) {
				return signature, variant != "unknown certificate" && owner == "probe.Owner$1" && descriptor == "(Lprobe/Owner;)V" && sourceOwner == "probe.Owner" && name == "make" && desc == "()V" && newPC == 3 && pc == 7
			}
			switch variant {
			case "class parent":
				signature = "Lprobe/Parent<TT;>;"
			case "missing callback":
				ctx.SourceAnonymousParentSignature = nil
			case "foreign method":
				ctx.FunctionName = "other"
			case "foreign class":
				ctx.ClassName = "probe.Foreign"
			case "missing new":
				allocation.HasOriginPC = false
			case "missing invoke":
				call.HasOriginPC = false
			case "wrong receiver":
				call.Object = JavaNull
			case "wrong owner":
				call.ClassName = "probe.Foreign$1"
			case "static":
				call.IsStatic = true
			case "virtual":
				call.Kind = InvokeVirtual
			case "missing special flag":
				call.IsSpecialInvoke = false
			case "malformed Signature":
				signature = "Ljava/lang/Object;Lprobe/Mapper<TT;"
			case "own formals":
				signature = "<U:Ljava/lang/Object;>Ljava/lang/Object;Lprobe/Mapper<TU;TU;>;"
			case "foreign formal":
				signature = "Ljava/lang/Object;Lprobe/Mapper<TU;TU;>;"
			case "multiple interfaces":
				signature += "Ljava/lang/Runnable;"
			case "deep Signature":
				signature = "Ljava/lang/Object;" + strings.Repeat("Lprobe/Box<", 34) + "TT;" + strings.Repeat(">;", 34)
			case "typed nil type":
				allocation.JavaType = (*types.JavaTypeWrap)(nil)
			case "work":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxGraphEdges: 1})
			case "memory":
				ctx.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				canceled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Work = workbudget.New(canceled, workbudget.Limits{})
			}
			beforeType, beforeCall := allocation.JavaType, allocation.ConstructorCall
			got := sourceAnonymousArgumentType(ctx, allocation)
			positive := variant == "original" || variant == "class parent"
			if (got != nil) != positive {
				t.Fatalf("original allocation %s returned %v", variant, got)
			}
			if allocation.JavaType != beforeType || allocation.ConstructorCall != beforeCall {
				t.Fatal("read-only type query changed original allocation")
			}
			if positive {
				want := "Mapper<T, T>"
				if variant == "class parent" {
					want = "Parent<T>"
				}
				if got.String(ctx) != want {
					t.Fatal("original lexical type arguments", got.String(ctx))
				}
			}
		})
	}
}

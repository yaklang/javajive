package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestInheritedSamTargetRequiresCompleteOriginalDeclarationAndEveryPath(t *testing.T) {
	const root = "proof/Result"
	const base = "proof/Base"
	const object = "java/lang/Object"
	const erased = "()Ljava/lang/Object;"
	constant := func(desc string) values.JavaValue {
		return values.NewCustomValue(func(*class_context.ClassContext) string { return desc }, func() types.JavaType {
			t.Fatal("declaration equations queried a mutable operand type")
			return nil
		})
	}
	for _, change := range []string{
		"original", "deep", "renamed formals", "same diamond", "bounded declaration size",
		"missing metadata", "missing declaration", "wrong metadata identity", "different declaration signature",
		"unknown members", "unknown parents", "class instead of interface", "duplicate physical parent",
		"wrong physical parent", "raw inherited edge", "unknown side parent", "cycle", "conflicting diamond",
		"different inherited methods", "fixed class named T", "nested generic edge", "array edge",
		"unused root formal", "stronger root bound", "own raw declaration", "missing selected declaration", "missing own override", "missing method metadata",
		"private method", "static method", "bridge method", "duplicate selected method", "different method signature",
		"other descriptor", "method-owned variable", "unknown throws variable", "malformed method signature",
		"work", "edge work", "memory", "canceled", "node limit", "signature limit",
	} {
		t.Run(change, func(t *testing.T) {
			iface := func(name, signature string, parents ...string) callbinding.Class {
				return callbinding.Class{Name: name, Signature: signature, Parents: parents, IsInterface: true, MembersComplete: true, ParentsComplete: true}
			}
			graph := map[string]callbinding.Class{
				root: iface(root, "<T:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Base<TT;>;", object, base),
				base: iface(base, "<V:Ljava/lang/Object;>Ljava/lang/Object;", object),
			}
			m := callbinding.Method{Name: "call", Desc: erased, Public: true, Generic: true, Signature: "()TV;"}
			var work *workbudget.Budget
			rootMeta, baseMeta := graph[root], graph[base]
			switch change {
			case "deep":
				rootMeta.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Middle<TT;>;"
				rootMeta.Parents = []string{object, "proof/Middle"}
				graph["proof/Middle"] = iface("proof/Middle", "<X:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Base<TX;>;", object, base)
			case "renamed formals":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<T:", "<Renamed:")
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "TT;", "TRenamed;")
			case "same diamond", "conflicting diamond", "different inherited methods":
				rootMeta.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Left<TT;>;Lproof/Right<TT;>;"
				rootMeta.Parents = []string{object, "proof/Left", "proof/Right"}
				graph["proof/Left"] = iface("proof/Left", "<L:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Base<TL;>;", object, base)
				graph["proof/Right"] = iface("proof/Right", "<R:Ljava/lang/Object;>Ljava/lang/Object;Lproof/Base<TR;>;", object, base)
				if change == "conflicting diamond" {
					r := graph["proof/Right"]
					r.Signature = strings.ReplaceAll(r.Signature, "<TR;>", "<Ljava/lang/String;>")
					graph[r.Name] = r
				}
				if change == "different inherited methods" {
					r := graph["proof/Right"]
					r.Methods = []callbinding.Method{{Name: "call", Desc: erased, Public: true, Signature: "()Ljava/lang/String;"}}
					graph[r.Name] = r
				}
			case "bounded declaration size", "node limit":
				for i := 0; i < 62; i++ {
					name := fmt.Sprintf("proof/Marker%d", i)
					rootMeta.Signature += "L" + name + ";"
					rootMeta.Parents = append(rootMeta.Parents, name)
					graph[name] = iface(name, "Ljava/lang/Object;", object)
				}
				if change == "node limit" {
					marker := graph["proof/Marker0"]
					marker.Signature += "Lproof/Extra;"
					marker.Parents = append(marker.Parents, "proof/Extra")
					graph[marker.Name] = marker
					graph["proof/Extra"] = iface("proof/Extra", "Ljava/lang/Object;", object)
				}
			case "wrong metadata identity":
				baseMeta.Name = "other/Base"
			case "unknown members":
				baseMeta.MembersComplete = false
			case "unknown parents":
				baseMeta.ParentsComplete = false
			case "class instead of interface":
				baseMeta.IsInterface = false
			case "duplicate physical parent":
				rootMeta.Parents = append(rootMeta.Parents, base)
			case "wrong physical parent":
				rootMeta.Parents = []string{object}
			case "raw inherited edge":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<TT;>", "")
			case "unknown side parent":
				rootMeta.Signature += "Lproof/Missing;"
				rootMeta.Parents = append(rootMeta.Parents, "proof/Missing")
			case "cycle":
				baseMeta.Signature += "Lproof/Result<TV;>;"
				baseMeta.Parents = append(baseMeta.Parents, root)
			case "fixed class named T":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<TT;>", "<LT;>")
			case "nested generic edge":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<TT;>", "<Ljava/util/List<TT;>;>")
			case "array edge":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<TT;>", "<[TT;>")
			case "unused root formal":
				rootMeta.Signature = strings.ReplaceAll(rootMeta.Signature, "<T:", "<Unused:Ljava/lang/Object;T:")
			case "stronger root bound":
				rootMeta.Signature = strings.Replace(rootMeta.Signature, "<T:Ljava/lang/Object;>", "<T:Ljava/lang/Number;>", 1)
			case "own raw declaration", "missing own override":
				rootMeta.Methods = []callbinding.Method{{Name: "call", Desc: erased, Public: true}}
			case "private method":
				m.Public = false
			case "static method":
				m.Static = true
			case "bridge method":
				m.Bridge = true
			case "other descriptor":
				m.Desc = "()Ljava/lang/String;"
			case "method-owned variable":
				m.Signature = "<V:Ljava/lang/Object;>()TV;"
			case "unknown throws variable":
				m.Signature = "()TV;^TE;"
			case "malformed method signature":
				m.Signature = "()TV;junk"
			case "signature limit":
				rootMeta.Signature += strings.Repeat("Lproof/Extra;", 1024)
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "edge work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphEdges: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			baseMeta.Methods = []callbinding.Method{m}
			if change == "duplicate selected method" {
				baseMeta.Methods = append(baseMeta.Methods, m)
			}
			graph[root], graph[base] = rootMeta, baseMeta
			ctx := &class_context.ClassContext{Work: work, InvocationMetadata: func(owner string) (callbinding.Class, bool) {
				meta, known := graph[owner]
				if change == "missing method metadata" && owner == base {
					meta.Methods = nil
				}
				return meta, known && !(change == "missing metadata" && owner == base)
			}}
			ctx.LambdaTargetClassSig = func(owner string) (string, map[string]string, bool) {
				meta, known := graph[owner]
				methods := map[string]string{}
				for _, member := range meta.Methods {
					methods[class_context.MethodDescKey(member.Name, member.Desc)] = member.Signature
				}
				if change == "missing own override" && owner == root {
					delete(methods, class_context.MethodDescKey("call", erased))
				}
				if owner == base {
					if change == "missing selected declaration" {
						delete(methods, class_context.MethodDescKey("call", erased))
					}
					if change == "different method signature" {
						methods[class_context.MethodDescKey("call", erased)] = "()Ljava/lang/String;"
					}
					if change == "different declaration signature" {
						meta.Signature = "<Other:Ljava/lang/Object;>Ljava/lang/Object;"
					}
				}
				return meta.Signature, methods, known && !(change == "missing declaration" && owner == base)
			}
			d := &Decompiler{FunctionContext: ctx, InvokeDynamicName: "call"}
			got := inferDeclaredLambdaTarget(d, types.NewJavaClass(root), constant(erased), constant("()Ljava/lang/String;"))
			want := change == "original" || change == "deep" || change == "renamed formals" || change == "same diamond" || change == "bounded declaration size"
			if (got != nil) != want {
				t.Fatalf("generic inherited target=%v want proved=%v", got, want)
			}
			if want {
				p, known := types.AsParameterizedType(got)
				if !known || len(p.TypeArgs) != 1 {
					t.Fatal("missing root-owned type parameter")
				}
				name, known := types.RawClassFQN(p.TypeArgs[0])
				if !known || name != "java.lang.String" {
					t.Fatal("parent variable was rebound by spelling", name)
				}
			}
		})
	}
}

package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestGenericFunctionalThrowsRequiresOriginalCovariantConstraints(t *testing.T) {
	for _, variant := range []string{"valid", "renamed", "checked", "wrong return", "raw", "contravariant", "invariant", "result constraint", "second constraint", "mismatched erasure", "wrong exceptions", "unknown exceptions", "missing ancestor", "extra interface bound", "wrong SAM", "generic SAM", "SAM checked", "SAM static", "ambiguous", "incomplete", "wrong static", "varargs rival", "different-arity varargs", "inherited SAM", "duplicate SAM", "functional bound", "raw generic receiver"} {
		t.Run(variant, func(t *testing.T) {
			name, formal, fi := "require", "E", "p/Factory"
			if variant == "renamed" {
				name, formal, fi = "dispatch", "Fault", "p/Renamed"
			}
			desc := "(JJL" + fi + ";)J"
			sig := "<" + formal + ":Ljava/lang/Throwable;>(JJL" + fi + "<+T" + formal + ";>;)J^T" + formal + ";"
			method := callbinding.Method{Name: name, Desc: desc, Signature: sig, Static: true, Generic: true, ExceptionsKnown: true, Exceptions: []string{"java/lang/Throwable"}}
			sam := callbinding.Method{Name: "create", Desc: "()Ljava/lang/Throwable;", Signature: "()TX;", Generic: true, ExceptionsKnown: true}
			actual := "()Ljava/lang/RuntimeException;"
			metadata := constructorThrowsBindingMetadata()
			switch variant {
			case "checked":
				actual = "()Ljava/io/IOException;"
			case "wrong return":
				actual = "()Ljava/lang/String;"
			case "raw":
				method.Signature = "<E:Ljava/lang/Throwable;>(JJLp/Factory;)J^TE;"
			case "contravariant":
				method.Signature = "<E:Ljava/lang/Throwable;>(JJLp/Factory<-TE;>;)J^TE;"
			case "invariant":
				method.Signature = "<E:Ljava/lang/Throwable;>(JJLp/Factory<TE;>;)J^TE;"
			case "result constraint":
				method.Signature = "<E:Ljava/lang/Throwable;>(JJLp/Factory<+TE;>;)TE;^TE;"
			case "second constraint":
				method.Signature = "<E:Ljava/lang/Throwable;>(TELjava/lang/Object;Lp/Factory<+TE;>;)J^TE;"
			case "mismatched erasure":
				method.Signature = "<E:Ljava/lang/Exception;>(JJLp/Factory<+TE;>;)J^TE;"
			case "wrong exceptions":
				method.Exceptions = []string{"java/lang/Exception"}
			case "unknown exceptions":
				method.ExceptionsKnown = false
			case "missing ancestor":
				delete(metadata, "java/lang/Exception")
			case "extra interface bound":
				method.Signature = "<E:Ljava/lang/Throwable;:Lp/Marker;>(JJLp/Factory<+TE;>;)J^TE;"
			case "wrong SAM":
				sam.Name = "other"
			case "generic SAM":
				sam.Signature = "<X:Ljava/lang/Throwable;>()TX;"
			case "SAM checked":
				sam.Exceptions = []string{"java/io/IOException"}
			case "SAM static":
				sam.Static = true
			case "wrong static":
				method.Static = false
			}
			metadata[fi] = callbinding.Class{Name: fi, Signature: "<X:Ljava/lang/Throwable;>Ljava/lang/Object;", Parents: []string{"java/lang/Object"}, IsInterface: true, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{sam}}
			owner := callbinding.Class{Name: "p/Owner", Parents: []string{"java/lang/Object"}, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{method}}
			if variant == "raw generic receiver" {
				owner.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				owner.Methods[0].Static = false
			}
			if variant == "varargs rival" || variant == "different-arity varargs" {
				rival := callbinding.Method{Name: name, Desc: "([Ljava/lang/Object;)J", Varargs: true, Static: true, ExceptionsKnown: true}
				if variant == "varargs rival" {
					rival.Desc = "(JJ[Ljava/lang/Object;)J"
				}
				owner.Methods = append(owner.Methods, rival)
			}
			if variant == "inherited SAM" || variant == "duplicate SAM" || variant == "functional bound" {
				f := metadata[fi]
				if variant == "inherited SAM" {
					f.Parents = append(f.Parents, "p/Marker")
				}
				if variant == "duplicate SAM" {
					f.Methods = append(f.Methods, sam)
				}
				if variant == "functional bound" {
					f.Signature = "<X:Ljava/lang/Throwable;:Lp/Marker;>Ljava/lang/Object;"
				}
				metadata[fi] = f
			}
			if variant == "ambiguous" {
				owner.Methods = append(owner.Methods, method)
			}
			if variant == "incomplete" {
				owner.ParentsComplete = false
			}
			metadata[owner.Name] = owner
			view, exception, known := genericFunctionalThrows(func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }, owner.Name, name, desc, variant != "raw generic receiver", 2, "create", "()Ljava/lang/Throwable;", actual, nil)
			want := variant == "valid" || variant == "renamed"
			if known != want {
				t.Fatalf("known=%v want=%v exception=%s", known, want, exception)
			}
			if known {
				p, ok := types.AsParameterizedType(view)
				if !ok || len(p.TypeArgs) != 1 || exception != "java/lang/RuntimeException" {
					t.Fatal("wrong functional view", view, exception)
				}
			}
		})
	}
}

func TestGenericFunctionalThrowsReceiverSeparatesClassAndMethodVariables(t *testing.T) {
	for _, variant := range []string{"class result", "unused class variable", "renamed formals", "class input", "method result", "method shadows class", "dependent class bound", "unknown signature", "wrong erased result", "inherited receiver", "intersection class bound"} {
		t.Run(variant, func(t *testing.T) {
			metadata := constructorThrowsBindingMetadata()
			owner := "p/Container"
			classSig := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			sig := "<E:Ljava/lang/Throwable;>(Lp/Factory<+TE;>;)TT;^TE;"
			desc := "(Lp/Factory;)Ljava/lang/Object;"
			switch variant {
			case "unused class variable":
				sig = "<E:Ljava/lang/Throwable;>(Lp/Factory<+TE;>;)J^TE;"
				desc = "(Lp/Factory;)J"
			case "renamed formals":
				classSig = "<Value:Ljava/lang/Object;>Ljava/lang/Object;"
				sig = "<Problem:Ljava/lang/Throwable;>(Lp/Factory<+TProblem;>;)TValue;^TProblem;"
			case "class input":
				sig = "<E:Ljava/lang/Throwable;>(TT;Lp/Factory<+TE;>;)TT;^TE;"
				desc = "(Ljava/lang/Object;Lp/Factory;)Ljava/lang/Object;"
			case "method result":
				sig = "<E:Ljava/lang/Throwable;>(Lp/Factory<+TE;>;)TE;^TE;"
				desc = "(Lp/Factory;)Ljava/lang/Throwable;"
			case "method shadows class":
				classSig = "<E:Ljava/lang/Object;>Ljava/lang/Object;"
				sig = "<E:Ljava/lang/Throwable;>(Lp/Factory<+TE;>;)TE;^TE;"
				desc = "(Lp/Factory;)Ljava/lang/Throwable;"
			case "intersection class bound":
				classSig = "<T:Ljava/lang/Object;:Lp/Marker;>Ljava/lang/Object;"
			case "dependent class bound":
				classSig = "<T:Ljava/lang/Object;U:TT;>Ljava/lang/Object;"
			case "unknown signature":
				classSig = ""
			case "wrong erased result":
				desc = "(Lp/Factory;)Ljava/lang/String;"
			}
			metadata[owner] = callbinding.Class{Name: owner, Signature: classSig, Parents: []string{"java/lang/Object"}, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "take", Desc: desc, Signature: sig, Generic: true, ExceptionsKnown: true, Exceptions: []string{"java/lang/Throwable"}}}}
			metadata["p/Factory"] = callbinding.Class{Name: "p/Factory", Signature: "<R:Ljava/lang/Object;>Ljava/lang/Object;", Parents: []string{"java/lang/Object"}, MembersComplete: true, ParentsComplete: true, IsInterface: true, Methods: []callbinding.Method{{Name: "get", Desc: "()Ljava/lang/Object;", Signature: "()TR;", Generic: true, ExceptionsKnown: true}}}
			if variant == "inherited receiver" {
				metadata["p/Child"] = callbinding.Class{Name: "p/Child", Parents: []string{owner}, MembersComplete: true, ParentsComplete: true}
				owner = "p/Child"
			}
			argument := 0
			if variant == "class input" {
				argument = 1
			}
			view, receiver, exception, known := genericFunctionalThrowsReceiver(func(n string) (callbinding.Class, bool) { c, ok := metadata[n]; return c, ok }, owner, "take", desc, false, argument, "get", "()Ljava/lang/Object;", "()Ljava/lang/RuntimeException;", nil)
			want := variant == "class result" || variant == "unused class variable" || variant == "renamed formals"
			if known != want {
				t.Fatalf("known=%v want=%v", known, want)
			}
			if known {
				if view == nil || receiver == nil || exception != "java/lang/RuntimeException" {
					t.Fatal("missing independent views")
				}
				p, ok := types.AsParameterizedType(receiver)
				if !ok || p.RawClassName != "p.Container" || len(p.TypeArgs) != 1 {
					t.Fatal("wrong generic receiver", receiver)
				}
			}
		})
	}
}

package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestAdversarialMemberOwnClassBoundRequiresExactProjection(t *testing.T) {
	files := nativeCompileDebugClasses(t, ownDependentBoundFixture(), "none")
	obj := mustParseLowering(t, files["SuffixOwner$Member.class"])
	original, known := nativeMethodLocalOriginalSignature(obj, obj.Attributes, nil)
	if !known {
		t.Fatal("original signature")
	}
	for _, variant := range []string{"exact", "changed binder", "changed arity", "changed secondary bound", "changed erasure", "changed supertype", "same erasure wrong binder", "wrong scope", "method scope", "missing scope", "unknown outer", "budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			scopes := []types.LexicalTypeScope{{Signature: "<T:Ljava/lang/Number;>Ljava/lang/Object;"}, {Signature: original}}
			fresh := "<U:Ljava/lang/Number;>Ljava/lang/Object;"
			var work *workbudget.Budget
			switch variant {
			case "changed binder":
				fresh = "<V:Ljava/lang/Number;>Ljava/lang/Object;"
			case "changed arity":
				fresh = "<U:Ljava/lang/Number;V:Ljava/lang/Object;>Ljava/lang/Object;"
			case "changed secondary bound":
				fresh = "<U:Ljava/lang/Number;:Ljava/io/Serializable;>Ljava/lang/Object;"
			case "changed erasure":
				fresh = "<U:Ljava/lang/Integer;>Ljava/lang/Object;"
			case "changed supertype":
				fresh = "<U:Ljava/lang/Number;>Ljava/lang/Object;Ljava/io/Serializable;"
			case "same erasure wrong binder":
				fresh = "<T:Ljava/lang/Number;U:TT;>Ljava/lang/Object;"
			case "wrong scope":
				scopes[1].Signature = "<U:Ljava/lang/Object;>Ljava/lang/Object;"
			case "method scope":
				scopes[1].Method = true
			case "missing scope":
				scopes = nil
			case "unknown outer":
				scopes[0].Signature = "Ljava/lang/Object;"
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				base, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(base, workbudget.Limits{})
			}
			if got := memberOwnScopeProjectedClassSignature(obj, fresh, scopes, work); got != (variant == "exact") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

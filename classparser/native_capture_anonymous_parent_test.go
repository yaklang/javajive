package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeCapturedAnonymousParentRequiresOriginalInstantiationAndClosedBounds(t *testing.T) {
	files := nativeCompileClasses(t, anonymousCapturedParentFixture)
	variants := []string{"original", "satisfied bound", "unsatisfied bound", "unsatisfied intersection", "parameterized bound", "free target formal", "wrong definition Signature", "missing Signature", "duplicate Signature", "free producer formal", "wildcard producer", "wrong parent", "wrong arity", "missing group", "failed group", "foreign owner", "missing producer", "foreign producer", "wrong method", "missing NEW origin", "changed NEW PC", "missing invoke origin", "changed invoke PC", "wrong descriptor", "wrong invoke kind", "static invoke", "wrong receiver", "nonallocation seed", "missing provider", "missing bound hierarchy", "budget", "memory", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(files["CapturedParentOwner.class"])
			prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
			if prepared == nil {
				t.Fatal("original prepared family")
			}
			family := prepared.family
			if family == nil || family.anonymous == nil {
				t.Fatal("original owned family")
			}
			d := z.nativeMemberReader(root)
			d.nativeMemberRoot, d.nativeAnonymousRoot = family, family.anonymous
			d.FuncCtx = &class_context.ClassContext{ClassName: root.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/CharSequence;)LCapturedParentReader;"}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			provider := d.FuncCtx.InvocationMetadata
			target, known := provider("CapturedParentBox")
			if !known {
				t.Fatal("original parent declaration")
			}
			definition, _ := Parse(files["CapturedParentBox.class"])
			producer := family.anonymous.children["CapturedParentOwner$1"]
			if producer == nil {
				t.Fatal("original producer")
			}
			seed := &values.NewExpression{JavaType: types.NewJavaClass("CapturedParentOwner$1"), HasOriginPC: true, OriginPC: producer.newPC}
			call := &values.FunctionCallExpression{ClassName: producer.object.GetClassName(), FunctionName: "<init>", Descriptor: producer.descriptor, Kind: values.InvokeSpecial, Object: seed, HasOriginPC: true, OriginPC: producer.invokePC}
			seed.ConstructorCall = call
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, seed.Type().Copy())
			declaration := statements.NewAssignStatement(ref, seed, true)
			setSignature := func(obj *ClassObject, signature string) {
				for _, attr := range obj.Attributes {
					if s, ok := attr.(*SignatureAttribute); ok {
						s.SignatureIndex = sourceBridgePoolString(t, obj, signature)
						return
					}
				}
				t.Fatal("original Signature absent")
			}
			switch variant {
			case "satisfied bound", "unsatisfied bound", "unsatisfied intersection", "parameterized bound", "free target formal":
				sig := "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"
				if variant == "unsatisfied bound" {
					sig = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
				}
				if variant == "unsatisfied intersection" {
					sig = "<T::Ljava/lang/CharSequence;:Ljava/io/Serializable;>Ljava/lang/Object;"
				}
				if variant == "parameterized bound" {
					sig = "<T::Ljava/lang/Comparable<Ljava/lang/String;>;>Ljava/lang/Object;"
				}
				if variant == "free target formal" {
					sig = "<T:Ljava/lang/Object;>Ljava/util/ArrayList<TX;>;"
				}
				setSignature(definition, sig)
				target.Signature = sig
			case "wrong definition Signature":
				target.Signature = "<T:Ljava/lang/Number;>Ljava/lang/Object;"
			case "missing Signature":
				var attrs []AttributeInfo
				for _, attr := range producer.object.Attributes {
					if _, ok := attr.(*SignatureAttribute); !ok {
						attrs = append(attrs, attr)
					}
				}
				producer.object.Attributes = attrs
			case "duplicate Signature":
				for _, attr := range producer.object.Attributes {
					if s, ok := attr.(*SignatureAttribute); ok {
						copy := *s
						producer.object.Attributes = append(producer.object.Attributes, &copy)
						break
					}
				}
			case "free producer formal":
				setSignature(producer.object, "LCapturedParentBox<TX;>;")
			case "wildcard producer":
				setSignature(producer.object, "LCapturedParentBox<+Ljava/lang/CharSequence;>;")
			case "wrong parent":
				setSignature(producer.object, "Ljava/util/ArrayList<Ljava/lang/CharSequence;>;")
			case "wrong arity":
				setSignature(producer.object, "LCapturedParentBox<Ljava/lang/CharSequence;Ljava/lang/Object;>;")
			case "missing group":
				d.nativeAnonymousRoot = nil
			case "failed group":
				family.anonymous.failed = true
			case "foreign owner":
				family.anonymous.owner = "Foreign"
			case "missing producer":
				delete(family.anonymous.children, "CapturedParentOwner$1")
			case "foreign producer":
				producer.object = root
			case "wrong method":
				d.FuncCtx.CurrentMethodDesc = "()V"
			case "missing NEW origin":
				seed.HasOriginPC = false
			case "changed NEW PC":
				seed.OriginPC++
			case "missing invoke origin":
				call.HasOriginPC = false
			case "changed invoke PC":
				call.OriginPC++
			case "wrong descriptor":
				call.Descriptor = "()V"
			case "wrong invoke kind":
				call.Kind = values.InvokeVirtual
			case "static invoke":
				call.IsStatic = true
			case "wrong receiver":
				call.Object = ref
			case "nonallocation seed":
				declaration.JavaValue = ref
			case "missing provider":
				provider = nil
			case "missing bound hierarchy":
				setSignature(producer.object, "LCapturedParentBox<Ljava/lang/String;>;")
				setSignature(definition, "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;")
				target.Signature = "<T::Ljava/lang/CharSequence;>Ljava/lang/Object;"
				provider = func(string) (callbinding.Class, bool) { return callbinding.Class{}, false }
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, closed := d.nativeCapturedAnonymousParentType(declaration, "LCapturedParentOwner$1;", "LCapturedParentBox;", target, definition, provider)
			good := variant == "original" || variant == "satisfied bound"
			if closed != good {
				t.Fatalf("closed=%v", closed)
			}
			if closed {
				p, ok := types.AsParameterizedType(got)
				if !ok || len(p.TypeArgs) != 1 || p.TypeArgs[0].String(d.FuncCtx) != "CharSequence" {
					t.Fatal("wrong original parent instantiation")
				}
			}
			if seed.Type() == nil || declaration.JavaValue == seed && seed.Type().String(d.FuncCtx) != "CapturedParentOwner$1" {
				t.Fatal("proof changed producer runtime type")
			}
		})
	}
}

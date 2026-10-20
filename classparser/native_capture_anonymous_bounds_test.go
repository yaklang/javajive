package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

const capturedAnonymousDependentBoundsFixture = `class NominalCaptureBound{}
abstract class DependentCaptureBox<T,U>{abstract T item();}
abstract class DependentCaptureDerived<A,B> extends DependentCaptureBox<A,B>{}
abstract class DependentCaptureReader{DependentCaptureReader(){read();}abstract Object read();}
class DependentCaptureOwner{DependentCaptureReader make(final CharSequence token){final DependentCaptureBox<CharSequence,String> selected=new DependentCaptureBox<CharSequence,String>(){CharSequence item(){return token;}};return new DependentCaptureReader(){Object read(){return selected.item();}};}}`

// The supported ordered-bound parser deliberately refuses dependent bounds.
// A caller's variable spelled T must not bypass that refusal. These original
// two-argument declarations provide a positive control before metadata changes;
// supporting U extends T later will require a separate declaration proof.
func TestNativeCapturedAnonymousBoundsCannotBorrowCallerFormals(t *testing.T) {
	files := nativeCompileClasses(t, capturedAnonymousDependentBoundsFixture)
	for _, mode := range []string{"original", "caller T is Object", "caller T is Number", "nominal argument cannot borrow caller formal", "nominal bound cannot borrow caller erasure", "dependent target bound", "unsatisfied dependent bound", "cyclic target bounds", "free target bound", "wrong physical parent", "compatible but nonphysical parent", "incomplete target hierarchy", "wrong target identity"} {
		t.Run(mode, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			owner, _ := Parse(files["DependentCaptureOwner.class"])
			prepared := z.prepareNativeMemberFamily(owner, snapshotJDECEnv())
			if prepared == nil || prepared.family == nil || prepared.family.anonymous == nil {
				t.Fatal("original dependent-bound family")
			}
			d := z.nativeMemberReader(owner)
			d.nativeMemberRoot, d.nativeAnonymousRoot = prepared.family, prepared.family.anonymous
			d.FuncCtx = &class_context.ClassContext{ClassName: owner.GetClassName(), FunctionName: "make", CurrentMethodDesc: "(Ljava/lang/CharSequence;)LDependentCaptureReader;"}
			d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
			provider := d.FuncCtx.InvocationMetadata
			definition, _ := Parse(files["DependentCaptureBox.class"])
			target, ok := provider("DependentCaptureBox")
			if !ok {
				t.Fatal("target declaration")
			}
			producer := d.nativeAnonymousRoot.children["DependentCaptureOwner$1"]
			if producer == nil {
				t.Fatal("producer identity")
			}
			setSig := func(o *ClassObject, text string) {
				for _, a := range o.Attributes {
					if s, ok := a.(*SignatureAttribute); ok {
						s.SignatureIndex = sourceBridgePoolString(t, o, text)
						return
					}
				}
				t.Fatal("original Signature missing")
			}
			switch mode {
			case "caller T is Object", "caller T is Number":
				bound := "java/lang/Object"
				if mode == "caller T is Number" {
					bound = "java/lang/Number"
				}
				d.FuncCtx.ClassSig = "<T:L" + bound + ";>Ljava/lang/Object;"
				d.FuncCtx.TypeParams = []string{"T"}
			case "nominal argument cannot borrow caller formal":
				setSig(producer.object, "LDependentCaptureBox<LNominalCaptureBound;Ljava/lang/String;>;")
				d.FuncCtx.ClassSig = "<NominalCaptureBound:Ljava/lang/Object;>Ljava/lang/Object;"
				d.FuncCtx.TypeParams = []string{"NominalCaptureBound"}
			case "nominal bound cannot borrow caller erasure":
				target.Signature = "<T:LNominalCaptureBound;U:Ljava/lang/Object;>Ljava/lang/Object;"
				setSig(definition, target.Signature)
				d.FuncCtx.ClassSig = "<NominalCaptureBound:Ljava/lang/Object;>Ljava/lang/Object;"
				d.FuncCtx.TypeParams = []string{"NominalCaptureBound"}
			case "dependent target bound":
				target.Signature = "<T:Ljava/lang/Object;U:TT;>Ljava/lang/Object;"
				setSig(definition, target.Signature)
				d.FuncCtx.ClassSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				d.FuncCtx.TypeParams = []string{"T"}
			case "unsatisfied dependent bound":
				target.Signature = "<T:Ljava/lang/Object;U:TT;>Ljava/lang/Object;"
				setSig(definition, target.Signature)
				setSig(producer.object, "LDependentCaptureBox<Ljava/lang/String;Ljava/lang/CharSequence;>;")
				// This unrelated same-name Object bound must not make it valid.
				d.FuncCtx.ClassSig = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
				d.FuncCtx.TypeParams = []string{"T"}
			case "cyclic target bounds":
				target.Signature = "<T:TU;U:TT;>Ljava/lang/Object;"
				setSig(definition, target.Signature)
			case "free target bound":
				target.Signature = "<T:Ljava/lang/Object;U:TX;>Ljava/lang/Object;"
				setSig(definition, target.Signature)
			case "compatible but nonphysical parent":
				setSig(producer.object, "LDependentCaptureDerived<Ljava/lang/CharSequence;Ljava/lang/String;>;")
			case "wrong physical parent":
				setSig(producer.object, "Ljava/util/ArrayList<Ljava/lang/String;>;")
			}
			originalProvider := provider
			provider = func(n string) (callbinding.Class, bool) {
				m, k := originalProvider(n)
				if n == "DependentCaptureBox" {
					m.Signature = target.Signature
					if mode == "incomplete target hierarchy" {
						m.ParentsComplete = false
					}
					if mode == "wrong target identity" {
						m.Name = "Foreign"
					}
				}
				return m, k
			}
			seed := &values.NewExpression{JavaType: types.NewJavaClass(producer.object.GetClassName()), OriginPC: producer.newPC, HasOriginPC: true}
			seed.ConstructorCall = &values.FunctionCallExpression{ClassName: producer.object.GetClassName(), FunctionName: "<init>", Descriptor: producer.descriptor, Object: seed, Kind: values.InvokeSpecial, OriginPC: producer.invokePC, HasOriginPC: true}
			ref := values.NewJavaRef(utils.NewRootVariableId(), nil, seed.Type().Copy())
			declaration := statements.NewAssignStatement(ref, seed, true)
			got, known := d.nativeCapturedAnonymousParentType(declaration, "LDependentCaptureOwner$1;", "LDependentCaptureBox;", target, definition, provider)
			good := mode == "original" || mode == "caller T is Object" || mode == "caller T is Number"
			if known != good {
				t.Fatalf("target-bound namespace admitted=%v want=%v", known, good)
			}
			if good {
				view, ok := types.AsParameterizedType(got)
				if !ok || len(view.TypeArgs) != 2 {
					t.Fatal("complete target arguments lost")
				}
				want := []string{"Ljava/lang/CharSequence;", "Ljava/lang/String;"}
				for i, a := range view.TypeArgs {
					if s, k := values.SourceTypeErasure(a, d.FuncCtx); !k || s != want[i] {
						t.Fatal("bound substitution changed an argument", i, s, k)
					}
				}
			}
		})
	}
}

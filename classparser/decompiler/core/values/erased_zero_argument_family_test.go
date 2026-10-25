package values

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestErasedZeroArgumentCovarianceRequiresOriginalOverrideEvidence(t *testing.T) {
	for _, variant := range []string{"proved", "renamed", "unproved bridge", "wrong target", "unknown flags", "nonbridge collision", "duplicate bridge", "narrow return", "private target", "static ancestor", "final ancestor", "generic method ancestor", "missing ancestor", "incomplete ancestor", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			f, _, ctx, classes := erasedZeroArgumentFixture()
			cell := classes["proof/Cell"]
			cell.Methods[0].AccessFlagsKnown = true
			cell.Methods[0].AccessFlags = 1
			cell.Methods = append(cell.Methods, callbinding.Method{Name: f.FunctionName, Desc: "()Ljava/lang/Object;", Public: true, Bridge: true, AccessFlags: 0x1041, AccessFlagsKnown: true, ExceptionsKnown: true, CovariantBridgeTarget: f.Descriptor})
			cell.Parents = append(cell.Parents, "proof/Getter")
			parent := callbinding.Class{Name: "proof/Getter", Public: true, IsInterface: true, MembersComplete: true, ParentsComplete: true, Parents: []string{"java/lang/Object"}, Methods: []callbinding.Method{{Name: f.FunctionName, Desc: "()Ljava/lang/Object;", Public: true, AccessFlags: 0x401, AccessFlagsKnown: true, ExceptionsKnown: true}}}
			switch variant {
			case "renamed":
				f.FunctionName = "produce"
				cell.Methods[0].Name, cell.Methods[1].Name, parent.Methods[0].Name = f.FunctionName, f.FunctionName, f.FunctionName
			case "unproved bridge":
				cell.Methods[1].CovariantBridgeTarget = ""
			case "wrong target":
				cell.Methods[1].CovariantBridgeTarget = "()Ljava/lang/String;"
			case "unknown flags":
				cell.Methods[1].AccessFlagsKnown = false
			case "nonbridge collision":
				cell.Methods[1].Bridge = false
			case "duplicate bridge":
				cell.Methods = append(cell.Methods, cell.Methods[1])
			case "narrow return":
				cell.Methods[1].Desc = "()Ljava/lang/String;"
			case "private target":
				cell.Methods[0].AccessFlags = 2
			case "static ancestor":
				parent.Methods[0].Static = true
			case "final ancestor":
				parent.Methods[0].AccessFlags = 0x11
			case "generic method ancestor":
				parent.Methods[0].Generic = true
				parent.Methods[0].Signature = "<E:Ljava/lang/Object;>()TE;"
			case "incomplete ancestor":
				parent.MembersComplete = false
			case "cycle":
				parent.Parents = []string{cell.Name}
			}
			classes[cell.Name] = cell
			if variant != "missing ancestor" {
				classes[parent.Name] = parent
			}
			out, known := f.planErasedZeroArgumentResult(ctx)
			if known != (variant == "proved" || variant == "renamed") {
				t.Fatalf("covariant result view=%v", known)
			}
			if known && (out.Witness() != f.Witness() || out.Object.(*CastExpression).Value != f.Object) {
				t.Fatal("covariant proof changed the original getter or receiver")
			}
		})
	}
}

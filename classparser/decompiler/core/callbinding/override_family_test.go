package callbinding

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func overrideMethod(result string) Method {
	return Method{Name: "append", Desc: "([Ljava/lang/Object;)" + result, Public: true, Varargs: true, AccessFlags: 0x81, AccessFlagsKnown: true}
}

func TestVariableArityOverrideFamilyRequiresOriginalEvidence(t *testing.T) {
	for _, scenario := range []string{"covariant", "same return", "inherited leaf", "no bridge evidence", "wrong bridge target", "orphan bridge", "duplicate bridge", "duplicate ordinary", "missing access", "package promoted", "private promoted", "final parent", "static parent", "generic parent", "different return", "primitive widening", "override removes varargs", "unrelated inherited", "unknown ancestor", "work", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			child, parent := overrideMethod("[Ljava/lang/Object;"), overrideMethod("Ljava/lang/Object;")
			bridge := parent
			bridge.Varargs, bridge.Bridge, bridge.AccessFlags = false, true, 0x1041
			bridge.CovariantBridgeTarget = child.Desc
			owner := Class{Name: "x/Child", Parents: []string{"x/Base"}, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []Method{child, bridge}}
			base := Class{Name: "x/Base", Parents: []string{"java/lang/Object"}, Public: true, MembersComplete: true, ParentsComplete: true, Methods: []Method{parent}}
			classes := map[string]Class{"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true}}
			w := Witness{Owner: owner.Name, Name: child.Name, Desc: child.Desc, Kind: Virtual}
			sam := "(Ljava/lang/Object;)V"
			var work *workbudget.Budget
			want := scenario == "covariant" || scenario == "same return" || scenario == "inherited leaf"
			switch scenario {
			case "same return":
				base.Methods[0] = child
				owner.Methods = owner.Methods[:1]
			case "inherited leaf":
				classes["x/Leaf"] = Class{Name: "x/Leaf", Parents: []string{owner.Name}, Public: true, MembersComplete: true, ParentsComplete: true}
				w.Owner = "x/Leaf"
			case "no bridge evidence":
				owner.Methods[1].CovariantBridgeTarget = ""
			case "wrong bridge target":
				owner.Methods[1].CovariantBridgeTarget = parent.Desc
			case "orphan bridge":
				base.Methods = nil
			case "duplicate bridge":
				owner.Methods = append(owner.Methods, bridge)
			case "duplicate ordinary":
				owner.Methods = append(owner.Methods, child)
			case "missing access":
				base.Methods[0].AccessFlagsKnown = false
			case "package promoted":
				base.Methods[0].AccessFlags = 0x80
			case "private promoted":
				base.Methods[0].AccessFlags = 0x82
			case "final parent":
				base.Methods[0].AccessFlags |= 0x10
			case "static parent":
				base.Methods[0].Static = true
			case "generic parent":
				base.Signature = "<T:Ljava/lang/Object;>Ljava/lang/Object;"
			case "different return":
				owner.Methods = []Method{overrideMethod("Ljava/lang/String;")}
				base.Methods = []Method{overrideMethod("[Ljava/lang/Object;")}
				w.Desc = owner.Methods[0].Desc
			case "primitive widening":
				owner.Methods = []Method{overrideMethod("I")}
				base.Methods = []Method{overrideMethod("J")}
				w.Desc = owner.Methods[0].Desc
			case "override removes varargs":
				owner.Methods = []Method{parent}
				owner.Methods[0].Varargs, owner.Methods[0].AccessFlags = false, 1
				w.Desc, sam = parent.Desc, "(Ljava/lang/Object;Ljava/lang/Object;)V"
			case "unrelated inherited":
				other := base
				other.Name = "x/Other"
				classes[other.Name] = other
				owner.Methods = nil
				owner.Parents = append(owner.Parents, other.Name)
				w.Desc = parent.Desc
			case "unknown ancestor":
				base.Parents = append(base.Parents, "x/Unknown")
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			classes[owner.Name], classes[base.Name] = owner, base
			plan := SelectVariableArityReference(w, sam, func(n string) (Class, bool) { c, ok := classes[n]; return c, ok }, work)
			if plan.Supported != want || want && (plan.DeclaringOwner != owner.Name || plan.Selected.Desc != child.Desc) {
				t.Fatalf("%+v wanted supported=%v", plan, want)
			}
		})
	}
}

// Independent chain model: the nearest declaration wins; its varargs bit is
// authoritative even when a more distant declaration had the other bit. Test
// chains with gaps as well as changes to the nearest declaration's varargs bit.
func TestVariableArityOverridesMatchNearestDeclarationModel(t *testing.T) {
	random := rand.New(rand.NewSource(8864))
	for world := 0; world < 200; world++ {
		n := 2 + random.Intn(8)
		classes := map[string]Class{"java/lang/Object": {Name: "java/lang/Object", Public: true, MembersComplete: true, ParentsComplete: true}}
		last, nearest := "java/lang/Object", ""
		var selected Method
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("model/Owner%d", i)
			c := Class{Name: name, Parents: []string{last}, Public: true, MembersComplete: true, ParentsComplete: true}
			if i == 0 || random.Intn(2) == 0 {
				m := overrideMethod("Ljava/lang/Object;")
				m.Varargs = random.Intn(2) == 0
				if !m.Varargs {
					m.AccessFlags = 1
				}
				c.Methods, selected, nearest = []Method{m}, m, name
			}
			classes[name], last = c, name
		}
		plan := SelectVariableArityReference(Witness{Owner: last, Name: selected.Name, Desc: selected.Desc, Kind: Virtual}, "(Ljava/lang/Object;Ljava/lang/Object;)V", func(n string) (Class, bool) { c, k := classes[n]; return c, k }, nil)
		if plan.Supported != selected.Varargs || plan.Supported && plan.DeclaringOwner != nearest {
			t.Fatalf("world %d nearest=%s varargs=%v got=%+v", world, nearest, selected.Varargs, plan)
		}
	}
}

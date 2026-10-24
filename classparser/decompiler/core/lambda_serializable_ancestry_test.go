package core

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestLambdaSerializableAncestryRequiresCompleteTypedParents(t *testing.T) {
	for _, variant := range []string{"ordinary", "direct", "indirect", "diamond", "missing", "incomplete", "wrong identity", "cycle", "too deep", "budget exhausted"} {
		t.Run(variant, func(t *testing.T) {
			graph := map[string]callbinding.Class{}
			add := func(name string, parents ...string) {
				graph[name] = callbinding.Class{Name: name, IsInterface: true, ParentsComplete: true, Parents: parents}
			}
			add("Port", "java/lang/Object")
			serializable, complete := false, true
			var budget *workbudget.Budget
			switch variant {
			case "direct":
				add("Port", "java/io/Serializable")
				serializable = true
			case "indirect":
				add("Port", "GenericPort")
				add("GenericPort", "java/io/Serializable")
				serializable = true
			case "diamond":
				add("Port", "Left", "Right")
				add("Left", "Base")
				add("Right", "Base")
				add("Base")
			case "missing":
				add("Port", "Missing")
				complete = false
			case "incomplete":
				cl := graph["Port"]
				cl.ParentsComplete = false
				graph["Port"] = cl
				complete = false
			case "wrong identity":
				cl := graph["Port"]
				cl.Name = "OtherPort"
				graph["Port"] = cl
				complete = false
			case "cycle":
				add("Port", "Parent")
				add("Parent", "Port")
				complete = false
			case "too deep":
				for i := 0; i < 40; i++ {
					name := "Port"
					if i > 0 {
						name = string(rune('a' + i))
					}
					add(name, string(rune('b'+i)))
				}
				complete = false
			case "budget exhausted":
				budget = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				complete = false
			}
			provider := func(n string) (callbinding.Class, bool) { cl, known := graph[n]; return cl, known }
			got, known := LambdaInterfaceSerializable(provider, "Port", budget)
			if got != serializable || known != complete {
				t.Fatal("typed serialization ancestry", got, known, serializable, complete)
			}
		})
	}
}

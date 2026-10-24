package core

import (
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// LambdaInterfaceSerializable distinguishes implementing Serializable from
// requesting the SerializedLambda protocol. In particular a normal metafactory
// can create a Serializable instance with serialization deliberately disabled;
// javac's lambda syntax would instead add writeReplace and a helper route.
// An absent ancestor is unknown, never evidence of non-serializability.
func LambdaInterfaceSerializable(provider callbinding.Provider, owner string, work *workbudget.Budget) (bool, bool) {
	owner = strings.ReplaceAll(owner, ".", "/")
	active, complete := map[string]bool{}, map[string]bool{}
	steps := 0
	var visit func(string, int) (bool, bool)
	visit = func(name string, depth int) (bool, bool) {
		steps++
		if name == "" || len(name) > 65535 || depth > 32 || steps > 1024 || work != nil && work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false, false
		}
		if name == "java/io/Serializable" {
			return true, true
		}
		if name == "java/lang/Object" || complete[name] {
			return false, true
		}
		if provider == nil || active[name] || len(active)+len(complete) >= 128 || work != nil && work.CheckAlloc(int64(len(active)+len(complete)+1)*128) != nil {
			return false, false
		}
		declaration, known := provider(name)
		if !known || declaration.Name != name || !declaration.IsInterface || !declaration.ParentsComplete || len(declaration.Parents) > 64 {
			return false, false
		}
		active[name] = true
		defer delete(active, name)
		closed := true
		for _, parent := range declaration.Parents {
			serializable, known := visit(parent, depth+1)
			if serializable {
				return true, true
			}
			closed = closed && known
		}
		if closed {
			complete[name] = true
		}
		return false, closed
	}
	return visit(owner, 0)
}

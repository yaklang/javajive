package callbinding

import (
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// WithReferenceAncestry supplements only the reference inheritance capability.
// A complete member table is a separate claim: an ancestry-only result cannot
// select an invocation, establish accessibility/finality or erase generic
// declaration binders. Original declaration results remain authoritative.
func WithReferenceAncestry(declarations Provider, parents func(string) ([]string, bool), work *workbudget.Budget) Provider {
	return func(name string) (Class, bool) {
		if work != nil && work.Charge(workbudget.CounterGraphScans, int64(len(name))+1) != nil {
			return Class{}, false
		}
		name = strings.ReplaceAll(name, ".", "/")
		if declarations != nil {
			if declaration, known := declarations(name); known {
				return declaration, declaration.Name == name
			}
		}
		if parents == nil {
			return Class{}, false
		}
		ancestry, known := parents(name)
		if !known || work != nil && (work.Charge(workbudget.CounterGraphScans, int64(len(ancestry))+1) != nil || work.CheckAlloc(int64(len(ancestry))*24+64) != nil) {
			return Class{}, false
		}
		return Class{Name: name, Parents: append([]string(nil), ancestry...), ParentsComplete: true}, true
	}
}

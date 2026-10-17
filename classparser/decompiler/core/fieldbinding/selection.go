// Package fieldbinding compares JVM symbolic field resolution with Java source
// field lookup. Both use original declaration tables, never a rendered name.
package fieldbinding

import (
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
)

type Field struct {
	Name, Descriptor string
	Access           uint16
}
type Class struct {
	Name, Super string
	Interfaces  []string
	Fields      []Field
	Accessible  bool
	Complete    bool
}
type Provider func(string) (Class, bool)
type declaration struct {
	owner string
	field Field
}

// Selection returns a nonthrowing receiver view only when source lookup would
// otherwise select a different physical field or be provably ambiguous. An
// unknown or ambiguous JVM target supplies no conversion permission. Widen
// must prove ordinary reference widening from the original hierarchy.
func Selection(p Provider, widen func(string, string) bool, receiver, owner, name, descriptor, caller string, work *workbudget.Budget) (string, bool) {
	if p == nil || widen == nil || receiver == "" || owner == "" || name == "" || descriptor == "" {
		return "", false
	}
	w := walker{provider: p, remaining: 65536, active: map[string]bool{}, work: work}
	wanted := w.lookup(owner, name, descriptor, true)
	if !wanted.complete || !wanted.found || wanted.ambiguous || wanted.declaration.field.Access&8 != 0 {
		return "", false
	}
	actual := w.lookup(receiver, name, "", false)
	if !actual.complete || !actual.found {
		return "", false
	}
	if !actual.ambiguous && actual.declaration == wanted.declaration {
		return "", true
	}
	for _, candidate := range []string{owner, wanted.declaration.owner, caller} {
		view, known := p(candidate)
		if !known || view.Name != candidate || !view.Accessible || !widen(receiver, candidate) {
			continue
		}
		selected := w.lookup(candidate, name, "", false)
		if !selected.complete || !selected.found || selected.ambiguous || selected.declaration != wanted.declaration {
			continue
		}
		access := wanted.declaration.field.Access
		allowed := access&1 != 0 || wanted.declaration.owner == caller || access&2 == 0 && pkg(wanted.declaration.owner) == pkg(caller)
		// Outside its declaration package a protected instance field needs a
		// qualifier whose type is the accessing subclass (or its subtype).
		if !allowed && access&4 != 0 && access&2 == 0 {
			allowed = widen(caller, wanted.declaration.owner) && widen(candidate, caller)
		}
		if allowed {
			return candidate, true
		}
	}
	return "", false
}
func pkg(name string) string {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i]
	}
	return ""
}

type walker struct {
	provider  Provider
	remaining int
	active    map[string]bool
	work      *workbudget.Budget
}

func (w *walker) take() bool {
	if w.remaining <= 0 {
		return false
	}
	w.remaining--
	return w.work == nil || w.work.Charge(workbudget.CounterGraphScans, 1) == nil
}

// Completeness and ambiguity are independent facts. A known ambiguous Java
// lookup can be repaired using an independently resolved JVM target; missing
// declarations cannot grant permission for any new receiver view.
type lookupResult struct {
	declaration                declaration
	found, complete, ambiguous bool
}

func (w *walker) lookup(owner, name, descriptor string, jvm bool) lookupResult {
	if owner == "" {
		return lookupResult{complete: true}
	}
	if !w.take() || len(w.active) >= 64 || w.active[owner] {
		return lookupResult{}
	}
	c, known := w.provider(owner)
	if !known || c.Name != owner || !c.Complete {
		return lookupResult{}
	}
	w.active[owner] = true
	defer delete(w.active, owner)
	var own *Field
	for _, f := range c.Fields {
		if !w.take() {
			return lookupResult{}
		}
		if f.Name == name && (!jvm || f.Descriptor == descriptor) {
			if own != nil {
				return lookupResult{}
			}
			copy := f
			own = &copy
		}
	}
	if own != nil {
		return lookupResult{declaration: declaration{owner, *own}, found: true, complete: true}
	}
	inherited := lookupResult{complete: true}
	merge := func(d lookupResult) bool {
		if !d.complete {
			return false
		}
		inherited.ambiguous = inherited.ambiguous || d.ambiguous
		if !d.found {
			return true
		}
		if inherited.found && inherited.declaration != d.declaration {
			inherited.ambiguous = true
		}
		if !inherited.found {
			inherited.declaration = d.declaration
		}
		inherited.found = true
		return true
	}
	for _, parent := range c.Interfaces {
		if !merge(w.lookup(parent, name, descriptor, jvm)) {
			return lookupResult{}
		}
	}
	// JVMS5.4.3.2 consults interfaces before the superclass. Multiple distinct
	// JVM candidates remain unproved; Java ambiguity is retained as known data.
	if jvm && inherited.found {
		return inherited
	}
	if !merge(w.lookup(c.Super, name, descriptor, jvm)) {
		return lookupResult{}
	}
	return inherited
}

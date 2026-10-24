package callbinding

import (
	"slices"
	"sort"
	"strings"

	"github.com/yaklang/javajive/internal/workbudget"
)

// VariableArityReferencePlan proves only which non-generic accessible
// declaration a bound instance or class static reference selects for its SAM inputs. It
// grants no factory, return-adaptation, checked-exception, adapter-body, source-access, capture or
// compiler-metadata permission. Those independent obligations remain at the
// original factory. In particular, an array-pack body is not an overload proof.
type VariableArityReferencePlan struct {
	// ExpandedParameters are the chosen non-generic declaration's physical
	// fixed and component ports, after each notional SAM input has passed
	// the variable-arity phase's invocation conversion. They grant no
	// result adaptation or original compiler/factory permission.
	ExpandedParameters []string
	DeclaringOwner     string
	Supported          bool
	Reason             string
	Selected           Method
}

type variableArityCandidate struct {
	owner      string
	method     Method
	parameters []string
}

type variableArityTruth struct{ value, known bool }
type variableArityQuery struct {
	provider  Provider
	work      *workbudget.Budget
	remaining int64
	types     map[[2]string]variableArityTruth
}

func (q *variableArityQuery) charge(n int64) bool {
	if n < 0 || n > q.remaining {
		return false
	}
	q.remaining -= n
	return q.work == nil || q.work.Charge(workbudget.CounterGraphScans, n) == nil
}

func (q *variableArityQuery) allocate(n int64) bool {
	return n >= 0 && (q.work == nil || q.work.CheckAlloc(n) == nil)
}

// SelectVariableArityReference follows JLS 15.12.2.2-.5 and 15.13.1. The
// instantiated SAM descriptor supplies notional, already typed arguments;
// they are not poly expressions. No potentially applicable method is removed
// merely because its declared arity differs from the original target's arity.
// Unknown metadata/conversion is never treated as a negative applicability
// result. Override-equivalent declarations are normalized only with original
// public instance access, ancestry and return-substitutability evidence. Bridges
// additionally require a certified forwarding body; generic families stay unknown.
func SelectVariableArityReference(w Witness, sam string, provider Provider, work *workbudget.Budget) VariableArityReferencePlan {
	out := VariableArityReferencePlan{}
	reject := func(reason string) VariableArityReferencePlan { out.Reason = reason; return out }
	if provider == nil || w.Owner == "" || w.Name == "" || strings.ContainsAny(w.Name, "<>") || w.Kind != Virtual && w.Kind != Interface && w.Kind != Static {
		return reject("bound instance or class static reference required")
	}
	arguments, _, err := Descriptor(sam)
	if err != nil || len(arguments) > 255 {
		return reject("invalid instantiated SAM descriptor")
	}
	if _, _, err := Descriptor(w.Desc); err != nil {
		return reject("invalid original invocation descriptor")
	}
	q := &variableArityQuery{provider: provider, work: work, remaining: 1 << 20, types: map[[2]string]variableArityTruth{}}
	var family []variableArityCandidate
	active, done := map[string]bool{}, map[string]bool{}
	var collect func(string, int) bool
	collect = func(name string, depth int) bool {
		if depth >= 128 || active[name] || !q.charge(int64(len(name))+1) {
			return false
		}
		if done[name] {
			return true
		}
		if len(done)+len(active) >= 512 || !q.allocate(int64(len(done)+len(active)+1)*256) {
			return false
		}
		class, known := provider(name)
		if !known || class.Name != name || !class.MembersComplete || !class.ParentsComplete || len(class.Methods) > 4096 || len(class.Parents) > 64 || !q.charge(int64(len(class.Methods)+len(class.Parents))+1) {
			return false
		}
		if name == w.Owner && (!class.Public || class.IsInterface != (w.Kind == Interface)) {
			return false
		}
		active[name] = true
		defer delete(active, name)
		declarations := map[string]bool{}
		for _, method := range class.Methods {
			if method.Name != w.Name {
				continue
			}
			parameters, _, err := Descriptor(method.Desc)
			if err != nil || declarations[method.Desc] || len(family) >= 256 || len(method.Exceptions) > 128 || !q.charge(int64(len(method.Desc)+len(method.Signature)+len(method.Exceptions))+1) || !q.allocate(int64(len(family)+1)*256+int64(len(method.Exceptions))*16) {
				return false
			}
			declarations[method.Desc] = true
			if method.SourceInaccessible {
				if !method.AccessFlagsKnown || method.AccessFlags&2 == 0 || method.Public {
					return false
				}
				continue
			}

			method.Exceptions = slices.Clone(method.Exceptions)
			family = append(family, variableArityCandidate{name, method, parameters})
		}
		parents := slices.Clone(class.Parents)
		sort.Strings(parents)
		for _, parent := range parents {
			if !collect(parent, depth+1) {
				return false
			}
		}
		done[name] = true
		return true
	}
	if !collect(w.Owner, 0) || len(family) == 0 {
		return reject("complete source overload family unavailable")
	}
	var normalized bool
	family, normalized = q.sourceOverrideFamily(family, w.Kind, len(arguments))
	if !normalized || len(family) == 0 {
		return reject("source override family unavailable")
	}
	targets := 0
	for _, candidate := range family {
		if candidate.method.Desc == w.Desc && candidate.method.Varargs {
			targets++
		}
	}
	if targets != 1 {
		return reject("original target is not a unique variable-arity declaration")
	}
	// A varargs declaration is also considered as an array-typed fixed-arity
	// declaration. Any applicable earlier phase prevents a packing reference.
	for _, loose := range []bool{false, true} {
		for _, candidate := range family {
			if len(candidate.parameters) != len(arguments) {
				continue
			}
			applies, known := q.applicable(arguments, candidate.parameters, loose)
			if !known {
				return reject("fixed-arity applicability unknown")
			}
			if applies {
				return reject("an earlier fixed-arity phase applies")
			}
		}
	}
	var applicable []variableArityCandidate
	for _, candidate := range family {
		if !candidate.method.Varargs || len(arguments) < len(candidate.parameters)-1 {
			continue
		}
		parameters := make([]string, len(arguments))
		for i := range parameters {
			parameters[i] = candidate.variableParameter(i)
		}
		applies, known := q.applicable(arguments, parameters, true)
		if !known {
			return reject("variable-arity applicability unknown")
		}
		if applies {
			applicable = append(applicable, candidate)
		}
	}
	winner := -1
	for i, candidate := range applicable {
		moreThanAll := true
		for j, other := range applicable {
			if i == j {
				continue
			}
			more, known := q.moreSpecific(candidate, other, len(arguments))
			if !known {
				return reject("variable-arity specificity unknown")
			}
			moreThanAll = moreThanAll && more
		}
		if moreThanAll {
			if winner >= 0 {
				return reject("multiple most-specific source declarations")
			}
			winner = i
		}
	}
	if winner < 0 || applicable[winner].method.Desc != w.Desc {
		return reject("original invocation is not the unique source selection")
	}
	if !q.charge(1) {
		return reject("source binding budget exhausted")
	}
	selected := applicable[winner]
	if !q.charge(int64(len(arguments))+1) || !q.allocate(int64(len(arguments))*32+64) {
		return reject("expanded input port budget exhausted")
	}
	ports := make([]string, len(arguments))
	for i := range ports {
		ports[i] = selected.variableParameter(i)
	}
	out.ExpandedParameters = ports
	out.Supported, out.Selected, out.DeclaringOwner = true, applicable[winner].method, applicable[winner].owner
	return out
}

func (c variableArityCandidate) variableParameter(i int) string {
	if i < len(c.parameters)-1 {
		return c.parameters[i]
	}
	return c.parameters[len(c.parameters)-1][1:]
}

func (q *variableArityQuery) applicable(arguments, parameters []string, loose bool) (bool, bool) {
	unknown := false
	for i, actual := range arguments {
		value, known := q.converts(actual, parameters[i], loose)
		if known && !value {
			return false, true
		}
		unknown = unknown || !known
	}
	return !unknown, !unknown
}

func (q *variableArityQuery) moreSpecific(a, b variableArityCandidate, arity int) (bool, bool) {
	for i := 0; i < arity; i++ {
		value, known := q.subtype(a.variableParameter(i), b.variableParameter(i))
		if !known || !value {
			return value, known
		}
	}
	// JLS 15.12.2.5 also compares the next variable parameter when the
	// second declaration has k+1 formals. This matters with zero pack items.
	if len(b.parameters) == arity+1 {
		return q.subtype(a.variableParameter(arity), b.variableParameter(arity))
	}
	if !q.charge(1) {
		return false, false
	}
	return true, true
}

var variableArityWrappers = map[string]string{
	"Z": "Ljava/lang/Boolean;", "B": "Ljava/lang/Byte;", "S": "Ljava/lang/Short;", "C": "Ljava/lang/Character;",
	"I": "Ljava/lang/Integer;", "J": "Ljava/lang/Long;", "F": "Ljava/lang/Float;", "D": "Ljava/lang/Double;",
}

func (q *variableArityQuery) converts(actual, formal string, loose bool) (bool, bool) {
	if !q.charge(1) {
		return false, false
	}
	if Reference(actual) == Reference(formal) || !loose {
		return q.subtype(actual, formal)
	}
	if wrapper := variableArityWrappers[actual]; wrapper != "" {
		return q.subtype(wrapper, formal)
	}
	for primitive, wrapper := range variableArityWrappers {
		if actual == wrapper {
			return variableArityPrimitiveWidening(primitive, formal), true
		}
	}
	return false, true
}

func variableArityPrimitiveWidening(actual, formal string) bool {
	if actual == formal {
		return true
	}
	return strings.Contains(map[string]string{"B": "SIJFD", "S": "IJFD", "C": "IJFD", "I": "JFD", "J": "FD", "F": "D"}[actual], formal) && len(formal) == 1
}

func (q *variableArityQuery) subtype(actual, formal string) (value, known bool) {
	if !q.charge(int64(len(actual)+len(formal)) + 1) {
		return false, false
	}
	key := [2]string{actual, formal}
	if result, found := q.types[key]; found {
		return result.value, result.known
	}
	defer func() {
		if len(q.types) < 4096 && q.allocate(int64(len(q.types)+1)*128) {
			q.types[key] = variableArityTruth{value, known}
		}
	}()
	if actual == formal {
		return true, true
	}
	if !Reference(actual) || !Reference(formal) {
		return variableArityPrimitiveWidening(actual, formal), true
	}
	if formal == "Ljava/lang/Object;" {
		return true, true
	}
	if strings.HasPrefix(actual, "[") || strings.HasPrefix(formal, "[") {
		if !strings.HasPrefix(actual, "[") {
			return false, true
		}
		if formal == "Ljava/lang/Cloneable;" || formal == "Ljava/io/Serializable;" {
			return true, true
		}
		if !strings.HasPrefix(formal, "[") {
			return false, true
		}
		a, b := actual[1:], formal[1:]
		if !Reference(a) || !Reference(b) {
			return a == b, true
		}
		return q.subtype(a, b)
	}
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string, int) (bool, bool)
	visit = func(name string, depth int) (bool, bool) {
		if !q.charge(int64(len(name))+1) || depth >= 128 || active[name] || len(done)+len(active) >= 512 {
			return false, false
		}
		if name == Name(formal) {
			return true, true
		}
		if done[name] {
			return false, true
		}
		class, found := q.provider(name)
		if !found || class.Name != name || !class.ParentsComplete || len(class.Parents) > 64 || !q.charge(int64(len(class.Parents))+1) {
			return false, false
		}
		active[name] = true
		defer delete(active, name)
		unknown := false
		for _, parent := range class.Parents {
			value, known := visit(parent, depth+1)
			if value {
				return value, known
			}
			unknown = unknown || !known
		}
		if !unknown {
			done[name] = true
		}
		return false, !unknown
	}
	return visit(Name(actual), 0)
}

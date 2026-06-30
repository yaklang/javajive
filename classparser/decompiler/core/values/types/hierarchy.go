package types

import "os"

// hierarchy.go 是 Phase 2 的类型层级/LUB 设施。反编译器此前完全没有类层级查询: 条件表达式/相位合并
// 求公共类型时, MergeTypes 对「两个不同引用类型」只能退回第一臂类型 (types[0]), 于是
//
//	List x = cond ? new ArrayList() : Collections.EMPTY_LIST;  // 误定型 ArrayList -> javac 拒绝
//	Number n = cond ? intVal : longVal;                        // 误定型 Integer -> javac 拒绝
//
// 这里用一张「高频 JDK 类型 -> 直接父类型」小表算最近公共祖先 (least upper bound), 给出能容纳每个臂的
// 声明类型。表是保守的: 任一臂不在表内, 或 LUB 退化到 Object, 都放弃 (回退旧的首臂行为), 把作用面收在
// 我们确切知道层级关系的 JDK 类型上。jar 内自定义类型的层级 (需 resolver 读 super/interfaces) 留待后续
// 阶段按需接入。Kill-switch: JDEC_TYPELUB_OFF。

// jdkSuperEdges maps a dot-form FQN to its DIRECT supertypes (superclass first, then key interfaces)
// for the high-frequency JDK families the decompiler must merge. Every chain terminates at
// java.lang.Object, so any two in-table types share at least Object.
var jdkSuperEdges = map[string][]string{
	// Number family (the EnumSchema Integer/Long/BigInteger -> Number LUB).
	"java.lang.Integer":                         {"java.lang.Number", "java.lang.Comparable"},
	"java.lang.Long":                            {"java.lang.Number", "java.lang.Comparable"},
	"java.lang.Short":                           {"java.lang.Number", "java.lang.Comparable"},
	"java.lang.Byte":                            {"java.lang.Number", "java.lang.Comparable"},
	"java.lang.Double":                          {"java.lang.Number", "java.lang.Comparable"},
	"java.lang.Float":                           {"java.lang.Number", "java.lang.Comparable"},
	"java.math.BigInteger":                      {"java.lang.Number", "java.lang.Comparable"},
	"java.math.BigDecimal":                      {"java.lang.Number", "java.lang.Comparable"},
	"java.util.concurrent.atomic.AtomicInteger": {"java.lang.Number"},
	"java.util.concurrent.atomic.AtomicLong":    {"java.lang.Number"},
	"java.lang.Number":                          {"java.lang.Object"},

	// CharSequence family.
	"java.lang.String":        {"java.lang.CharSequence", "java.lang.Comparable"},
	"java.lang.StringBuilder": {"java.lang.CharSequence"},
	"java.lang.StringBuffer":  {"java.lang.CharSequence"},
	"java.lang.CharSequence":  {"java.lang.Object"},

	// Collection family (the DaitchMokotoffSoundex ArrayList/List -> List LUB).
	"java.util.ArrayList":              {"java.util.AbstractList", "java.util.List", "java.util.RandomAccess"},
	"java.util.LinkedList":             {"java.util.AbstractSequentialList", "java.util.List", "java.util.Deque"},
	"java.util.Vector":                 {"java.util.AbstractList", "java.util.List", "java.util.RandomAccess"},
	"java.util.Stack":                  {"java.util.Vector"},
	"java.util.AbstractList":           {"java.util.AbstractCollection", "java.util.List"},
	"java.util.AbstractSequentialList": {"java.util.AbstractList"},
	"java.util.AbstractCollection":     {"java.util.Collection"},
	"java.util.HashSet":                {"java.util.AbstractSet", "java.util.Set"},
	"java.util.LinkedHashSet":          {"java.util.HashSet", "java.util.Set"},
	"java.util.TreeSet":                {"java.util.AbstractSet", "java.util.SortedSet"},
	"java.util.AbstractSet":            {"java.util.AbstractCollection", "java.util.Set"},
	"java.util.SortedSet":              {"java.util.Set"},
	"java.util.List":                   {"java.util.Collection"},
	"java.util.Set":                    {"java.util.Collection"},
	"java.util.Queue":                  {"java.util.Collection"},
	"java.util.Deque":                  {"java.util.Queue"},
	"java.util.Collection":             {"java.lang.Iterable"},
	"java.lang.Iterable":               {"java.lang.Object"},
	"java.util.RandomAccess":           {"java.lang.Object"},

	// Map family.
	"java.util.HashMap":       {"java.util.AbstractMap", "java.util.Map"},
	"java.util.LinkedHashMap": {"java.util.HashMap", "java.util.Map"},
	"java.util.TreeMap":       {"java.util.AbstractMap", "java.util.SortedMap"},
	"java.util.Hashtable":     {"java.util.Dictionary", "java.util.Map"},
	"java.util.Properties":    {"java.util.Hashtable"},
	"java.util.AbstractMap":   {"java.util.Map"},
	"java.util.SortedMap":     {"java.util.Map"},
	"java.util.Dictionary":    {"java.lang.Object"},
	"java.util.Map":           {"java.lang.Object"},

	// Reflection family. Method/Field/Constructor share TWO common ancestors: the class
	// java.lang.reflect.AccessibleObject and the interface java.lang.reflect.Member. We deliberately
	// map them to Member ONLY (omitting AccessibleObject) so the LUB of a `cond ? method : field` merge
	// is `Member`, not `AccessibleObject`: the operations actually invoked on such a merged value in
	// real code are getName()/getDeclaringClass()/getModifiers() -- all declared on Member, NONE on
	// AccessibleObject (which only carries setAccessible/getAnnotation). Picking the class
	// AccessibleObject (which the concrete-class tie-break would otherwise prefer) would merely trade
	// "bad type in conditional" for "cannot find symbol: method getName()". The intersection type
	// `AccessibleObject & Member` that javac actually computes is not denotable in a declaration, so
	// Member is the single use-correct choice (fastjson2 FieldReader.toString / compareTo
	// `Member m = method != null ? method : field; m.getName()`).
	"java.lang.reflect.Method":      {"java.lang.reflect.Member"},
	"java.lang.reflect.Field":       {"java.lang.reflect.Member"},
	"java.lang.reflect.Constructor": {"java.lang.reflect.Member"},
	"java.lang.reflect.Member":      {"java.lang.Object"},

	// Interfaces that bottom out at Object.
	"java.lang.Comparable": {"java.lang.Object"},
}

// jdkInterfaceSet marks which table entries are interfaces, so the LUB tie-break can prefer a concrete
// superclass (Number) over an equally-near interface (Comparable) when both are common ancestors.
var jdkInterfaceSet = map[string]bool{
	"java.lang.Comparable":     true,
	"java.lang.CharSequence":   true,
	"java.lang.Iterable":       true,
	"java.util.Collection":     true,
	"java.util.List":           true,
	"java.util.Set":            true,
	"java.util.SortedSet":      true,
	"java.util.Queue":          true,
	"java.util.Deque":          true,
	"java.util.Map":            true,
	"java.util.SortedMap":      true,
	"java.util.RandomAccess":   true,
	"java.lang.reflect.Member": true,
}

// ancestorDepths returns the BFS distance from `name` to each of its (reflexive) ancestors, or nil if
// `name` is an unknown type (not in the table and not Object). Used to score common ancestors.
func ancestorDepths(name string) map[string]int {
	if name != "java.lang.Object" {
		if _, ok := jdkSuperEdges[name]; !ok {
			return nil
		}
	}
	depth := map[string]int{name: 0}
	queue := []string{name}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, sup := range jdkSuperEdges[cur] {
			if _, seen := depth[sup]; !seen {
				depth[sup] = depth[cur] + 1
				queue = append(queue, sup)
			}
		}
	}
	return depth
}

// commonSuperName returns the nearest common ancestor (least upper bound) of two dot-FQN types, by
// minimum combined BFS depth. Ties prefer a concrete class over an interface, then the
// lexicographically smaller name for determinism. Returns "" when either type is unknown.
func commonSuperName(a, b string) string {
	if a == b {
		return a
	}
	da := ancestorDepths(a)
	db := ancestorDepths(b)
	if da == nil || db == nil {
		return ""
	}
	best := ""
	bestScore := 1 << 30
	bestIsClass := false
	for anc, d1 := range da {
		d2, ok := db[anc]
		if !ok {
			continue
		}
		score := d1 + d2
		isClass := !jdkInterfaceSet[anc]
		better := false
		switch {
		case best == "":
			better = true
		case score < bestScore:
			better = true
		case score == bestScore:
			if isClass != bestIsClass {
				better = isClass // prefer concrete class
			} else {
				better = anc < best // deterministic tie-break
			}
		}
		if better {
			best, bestScore, bestIsClass = anc, score, isClass
		}
	}
	return best
}

// classNameOf extracts a non-array class FQN from a JavaType, or ("",false) for primitives/arrays/
// non-class types (which have no hierarchy here and must fall back to the legacy merge).
func classNameOf(t JavaType) (string, bool) {
	if t == nil || t.IsArray() {
		return "", false
	}
	if jc, ok := t.RawType().(*JavaClass); ok && jc.Name != "" {
		return jc.Name, true
	}
	return "", false
}

// commonSuperType computes a declared type that accepts every arm: the least upper bound across all
// arms via the JDK hierarchy table. Returns nil (caller keeps the legacy first-arm fallback) when any
// arm is not a known JDK class or when the LUB degrades to Object (avoids widening regressions where
// the result is later dereferenced for a more specific member). Gated by JDEC_TYPELUB_OFF.
func commonSuperType(arms []JavaType) JavaType {
	if os.Getenv("JDEC_TYPELUB_OFF") != "" {
		return nil
	}
	name := ""
	have := false
	for _, t := range arms {
		n, ok := classNameOf(t)
		if !ok {
			return nil
		}
		if !have {
			name, have = n, true
			continue
		}
		name = commonSuperName(name, n)
		if name == "" || name == "java.lang.Object" {
			return nil
		}
	}
	if !have || name == "java.lang.Object" || name == "" {
		return nil
	}
	return NewJavaClass(name)
}

// CommonSuperType is the exported two-argument LUB used by phi/ternary type merging outside this
// package (Phase 1c phi declaration typing). It returns nil when no useful (sub-Object) LUB exists.
func CommonSuperType(a, b JavaType) JavaType {
	return commonSuperType([]JavaType{a, b})
}

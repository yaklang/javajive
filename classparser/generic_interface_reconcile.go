package javaclassparser

import (
	"slices"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

type inheritedIfaceView struct {
	raw     bool
	args    []types.JavaType
	argKeys []string
}

func canonicalTypeKey(t types.JavaType) string {
	if t == nil {
		return ""
	}
	return t.String(&class_context.ClassContext{})
}

func sameStringSlice(a, b []string) bool {
	return slices.Equal(a, b)
}

// ownClassFormalParamSet returns the formal type-parameter names declared directly
// on c.obj's Signature attribute (prior to Dumper populating FuncCtx.TypeParams).
func (c *ClassObjectDumper) ownClassFormalParamSet() map[string]bool {
	out := map[string]bool{}
	if c == nil || c.obj == nil {
		return out
	}
	for _, attr := range c.obj.Attributes {
		sigAttr, ok := attr.(*SignatureAttribute)
		if !ok || sigAttr == nil {
			continue
		}
		sigStr, err := c.obj.getUtf8(sigAttr.SignatureIndex)
		if err != nil || sigStr == "" {
			break
		}
		for _, name := range types.ClassFormalTypeParamNames(sigStr) {
			out[name] = true
		}
		break
	}
	return out
}

// collectInheritedIfaceViews walks the supertype hierarchy of (rootRaw, rootType)
// following JLS 4.8 raw erasure propagation and instantiating generic edges via
// SiblingClassSig + SiblingSuperTypes.
func collectInheritedIfaceViews(ctx *class_context.ClassContext, rootRaw string, rootType types.JavaType, includeRoot bool) map[string][]inheritedIfaceView {
	out := map[string][]inheritedIfaceView{}
	if ctx == nil || ctx.SiblingClassSig == nil || rootRaw == "" {
		return out
	}
	visited := map[string]bool{}
	var walk func(nodeRaw string, nodeType types.JavaType, viaRaw bool, depth int)
	walk = func(nodeRaw string, nodeType types.JavaType, viaRaw bool, depth int) {
		if depth > 32 || len(visited) >= 256 || nodeRaw == "" || nodeRaw == "java.lang.Object" {
			return
		}
		internal := strings.ReplaceAll(nodeRaw, ".", "/")
		classSig, _, sigOK := ctx.SiblingClassSig(internal)
		var formals []string
		if sigOK && classSig != "" {
			formals = types.ClassFormalTypeParamNames(classSig)
		}

		pt, isPT := types.AsParameterizedType(nodeType)
		nodeRawFlag := viaRaw
		var args []types.JavaType
		var sigma map[string]types.JavaType
		if len(formals) > 0 {
			if !nodeRawFlag && isPT && pt != nil && len(pt.TypeArgs) == len(formals) {
				args = pt.TypeArgs
				sigma = make(map[string]types.JavaType, len(formals))
				for i, f := range formals {
					sigma[f] = args[i]
				}
			} else {
				nodeRawFlag = true
				args = nil
			}
		}

		argKeys := make([]string, len(args))
		for i, a := range args {
			argKeys[i] = canonicalTypeKey(a)
		}
		visitKey := internal + "|"
		if nodeRawFlag {
			visitKey += "<raw>"
		} else {
			visitKey += strings.Join(argKeys, ",")
		}
		if visited[visitKey] {
			return
		}
		visited[visitKey] = true

		if depth > 0 || includeRoot {
			if nodeRawFlag {
				out[nodeRaw] = append(out[nodeRaw], inheritedIfaceView{raw: true})
			} else if len(args) > 0 {
				out[nodeRaw] = append(out[nodeRaw], inheritedIfaceView{args: args, argKeys: argKeys})
			}
		}

		if sigOK && classSig != "" {
			sup, sigIfaces := types.ParseClassSignatureSupers(classSig)
			walkEdge := func(st types.JavaType) {
				if st == nil {
					return
				}
				raw := genericSupertypeRawName(st)
				if raw == "" || raw == "java.lang.Object" {
					return
				}
				if nodeRawFlag {
					walk(raw, nil, true, depth+1)
					return
				}
				sub := st
				if len(sigma) > 0 {
					sub = types.SubstituteTypeVars(st, sigma)
				}
				walk(raw, sub, false, depth+1)
			}
			walkEdge(sup)
			for _, it := range sigIfaces {
				walkEdge(it)
			}
			if sup != nil || len(sigIfaces) > 0 {
				return
			}
		}
		if ctx.SiblingSuperTypes != nil {
			if rawParents, ok := ctx.SiblingSuperTypes(internal); ok {
				for _, pInternal := range rawParents {
					pDot := strings.ReplaceAll(pInternal, "/", ".")
					if pDot == "" || pDot == "java.lang.Object" {
						continue
					}
					walk(pDot, nil, nodeRawFlag, depth+1)
				}
			}
		}
	}
	walk(rootRaw, rootType, false, 0)
	return out
}

func unifyGenericPattern(pattern, target types.JavaType, bound map[string]types.JavaType) bool {
	if pattern == nil || target == nil {
		return true
	}
	if name, ok := types.TypeVariableName(pattern); ok && name != "" {
		if prev, exists := bound[name]; exists {
			return canonicalTypeKey(prev) == canonicalTypeKey(target)
		}
		bound[name] = target
		return true
	}
	pp, okP := types.AsParameterizedType(pattern)
	tp, okT := types.AsParameterizedType(target)
	if okP && okT && pp != nil && tp != nil && pp.RawClassName == tp.RawClassName && len(pp.TypeArgs) == len(tp.TypeArgs) {
		for i := range pp.TypeArgs {
			if !unifyGenericPattern(pp.TypeArgs[i], tp.TypeArgs[i], bound) {
				return false
			}
		}
		return true
	}
	return canonicalTypeKey(pattern) == canonicalTypeKey(target)
}

func sanitizeSuperInterfaceTypeArg(ctx *class_context.ClassContext, arg types.JavaType, ownFormals map[string]bool) types.JavaType {
	if arg == nil || types.IsWildcardType(arg) {
		return types.NewJavaClass("java.lang.Object")
	}
	if name, ok := types.TypeVariableName(arg); ok {
		if ownFormals[name] || (ctx != nil && (ctx.IsTypeParam(name) || ctx.RawEraseTypeVars[name])) {
			return arg
		}
		return types.NewJavaClass("java.lang.Object")
	}
	if pt, ok := types.AsParameterizedType(arg); ok && pt != nil {
		newArgs := make([]types.JavaType, len(pt.TypeArgs))
		for i, ta := range pt.TypeArgs {
			if ta == nil || types.IsWildcardType(ta) {
				newArgs[i] = ta
				continue
			}
			newArgs[i] = sanitizeSuperInterfaceTypeArg(ctx, ta, ownFormals)
		}
		return types.NewParameterizedType(pt.RawClassName, newArgs)
	}
	return arg
}

// parameterizeIfaceFromViews finds an instantiation for raw interface iface such
// that every shared generic ancestor agrees with targetViews.
func (c *ClassObjectDumper) parameterizeIfaceFromViews(ctx *class_context.ClassContext, iface string, targetViews map[string][]inheritedIfaceView, ownFormals map[string]bool) (types.JavaType, string, bool) {
	internal := strings.ReplaceAll(iface, ".", "/")
	ifaceSig, _, sigOK := ctx.SiblingClassSig(internal)
	if !sigOK || ifaceSig == "" {
		return nil, "", false
	}
	formals := types.ClassFormalTypeParamNames(ifaceSig)
	if len(formals) == 0 {
		return nil, "", false
	}
	symArgs := make([]types.JavaType, len(formals))
	for i, f := range formals {
		symArgs[i] = types.NewJavaTypeVariable(f)
	}
	symViews := collectInheritedIfaceViews(ctx, iface, types.NewParameterizedType(iface, symArgs), true)

	bound := map[string]types.JavaType{}
	matchedAny := false
	for anc, symList := range symViews {
		tList := targetViews[anc]
		if len(tList) == 0 {
			continue
		}
		for _, sv := range symList {
			if sv.raw || len(sv.args) == 0 {
				continue
			}
			for _, tv := range tList {
				if tv.raw || len(tv.args) != len(sv.args) {
					continue
				}
				matchedAny = true
				for i := range sv.args {
					if !unifyGenericPattern(sv.args[i], tv.args[i], bound) {
						return nil, "", false
					}
				}
			}
		}
	}
	if !matchedAny {
		return nil, "", false
	}

	erasures := types.ClassFormalTypeParamErasures(ifaceSig)
	resolvedArgs := make([]types.JavaType, len(formals))
	for i, f := range formals {
		if arg, ok := bound[f]; ok && arg != nil {
			resolvedArgs[i] = sanitizeSuperInterfaceTypeArg(ctx, arg, ownFormals)
			continue
		}
		erased := erasures[f]
		if erased == "" {
			erased = "java.lang.Object"
		}
		resolvedArgs[i] = types.NewJavaClass(erased)
	}
	paramType := types.NewParameterizedType(iface, resolvedArgs)
	rendered := paramType.String(ctx)
	if !strings.Contains(rendered, "<") {
		return nil, "", false
	}
	return paramType, rendered, true
}

// reconcileGenericSuperInterfaces detects and resolves generic superinterface
// inheritance conflicts (Case A: raw interface vs parameterized ancestor, Case B:
// parameterized interface vs raw ancestor, Case C: disagreeing type arguments).
func (c *ClassObjectDumper) reconcileGenericSuperInterfaces(funcCtx *class_context.ClassContext, rawSuper string, rawIfaces []string, genericSuperByRaw map[string]string, genericSuperTypeByRaw map[string]types.JavaType) bool {
	if c == nil || funcCtx == nil || funcCtx.SiblingClassSig == nil || len(rawIfaces) == 0 {
		return false
	}
	reconciled := false
	ownFormals := c.ownClassFormalParamSet()

	var superViews map[string][]inheritedIfaceView
	if rawSuper != "" {
		superViews = collectInheritedIfaceViews(funcCtx, rawSuper, genericSuperTypeByRaw[rawSuper], false)
	}

	// Pass 1 (Case A): raw direct interface clashing with parameterized ancestor from superclass.
	if len(superViews) > 0 {
		for _, iface := range rawIfaces {
			if genericSuperByRaw[iface] != "" {
				continue
			}
			ifViews := collectInheritedIfaceViews(funcCtx, iface, nil, true)
			needsParam := false
			for anc, ivs := range ifViews {
				svs := superViews[anc]
				if len(svs) == 0 {
					continue
				}
				hasParamSuper := false
				for _, sv := range svs {
					if !sv.raw && len(sv.args) > 0 {
						hasParamSuper = true
						break
					}
				}
				if !hasParamSuper {
					continue
				}
				for _, iv := range ivs {
					if iv.raw {
						needsParam = true
						break
					}
				}
				if needsParam {
					break
				}
			}
			if needsParam {
				if pType, rendered, ok := c.parameterizeIfaceFromViews(funcCtx, iface, superViews, ownFormals); ok {
					genericSuperByRaw[iface] = rendered
					genericSuperTypeByRaw[iface] = pType
					reconciled = true
				}
			}
		}
	}

	// Pass 2 (Case B): parameterized direct interface clashing with raw ancestor
	// inherited from superclass or another raw direct interface.
	rawAncestors := map[string]bool{}
	for anc, svs := range superViews {
		for _, sv := range svs {
			if sv.raw {
				rawAncestors[anc] = true
			}
		}
	}
	otherIfaceViews := make([]map[string][]inheritedIfaceView, len(rawIfaces))
	for i, iface := range rawIfaces {
		otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, iface, genericSuperTypeByRaw[iface], true)
	}
	for i, iface := range rawIfaces {
		if genericSuperByRaw[iface] == "" {
			continue
		}
		views := otherIfaceViews[i]
		clashRaw := false
		for anc, ivs := range views {
			hasParam := false
			for _, iv := range ivs {
				if !iv.raw && len(iv.args) > 0 {
					hasParam = true
					break
				}
			}
			if !hasParam {
				continue
			}
			if rawAncestors[anc] {
				clashRaw = true
				break
			}
			for j := range rawIfaces {
				if j == i || genericSuperByRaw[rawIfaces[j]] != "" {
					continue
				}
				for _, ov := range otherIfaceViews[j][anc] {
					if ov.raw {
						clashRaw = true
						break
					}
				}
				if clashRaw {
					break
				}
			}
			if clashRaw {
				break
			}
		}
		if clashRaw {
			delete(genericSuperByRaw, iface)
			delete(genericSuperTypeByRaw, iface)
			otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, iface, nil, true)
			reconciled = true
		}
	}

	// Pass 3 (Case C): disagreeing parameterized instantiations between superclass
	// and interface, or between two direct interfaces.
	if len(superViews) > 0 {
		for i, iface := range rawIfaces {
			if genericSuperByRaw[iface] == "" {
				continue
			}
			views := otherIfaceViews[i]
			disagrees := false
			for anc, ivs := range views {
				svs := superViews[anc]
				for _, iv := range ivs {
					if iv.raw || len(iv.args) == 0 {
						continue
					}
					for _, sv := range svs {
						if !sv.raw && len(sv.args) > 0 && !sameStringSlice(iv.argKeys, sv.argKeys) {
							disagrees = true
							break
						}
					}
					if disagrees {
						break
					}
				}
				if disagrees {
					break
				}
			}
			if !disagrees {
				continue
			}
			if rawSuper != "" && genericSuperByRaw[rawSuper] != "" {
				delete(genericSuperByRaw, rawSuper)
				delete(genericSuperTypeByRaw, rawSuper)
				delete(genericSuperByRaw, iface)
				delete(genericSuperTypeByRaw, iface)
				otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, iface, nil, true)
				superViews = collectInheritedIfaceViews(funcCtx, rawSuper, nil, false)
				reconciled = true
			} else if pType, rendered, ok := c.parameterizeIfaceFromViews(funcCtx, iface, superViews, ownFormals); ok {
				genericSuperByRaw[iface] = rendered
				genericSuperTypeByRaw[iface] = pType
				otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, iface, pType, true)
				reconciled = true
			} else {
				delete(genericSuperByRaw, iface)
				delete(genericSuperTypeByRaw, iface)
				otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, iface, nil, true)
				reconciled = true
			}
		}
	}

	for i := 0; i < len(rawIfaces); i++ {
		if genericSuperByRaw[rawIfaces[i]] == "" {
			continue
		}
		for j := i + 1; j < len(rawIfaces); j++ {
			if genericSuperByRaw[rawIfaces[j]] == "" {
				continue
			}
			vi, vj := otherIfaceViews[i], otherIfaceViews[j]
			disagrees := false
			for anc, ivs := range vi {
				jvs := vj[anc]
				for _, a := range ivs {
					if a.raw || len(a.args) == 0 {
						continue
					}
					for _, b := range jvs {
						if !b.raw && len(b.args) > 0 && !sameStringSlice(a.argKeys, b.argKeys) {
							disagrees = true
							break
						}
					}
					if disagrees {
						break
					}
				}
				if disagrees {
					break
				}
			}
			if disagrees {
				delete(genericSuperByRaw, rawIfaces[i])
				delete(genericSuperTypeByRaw, rawIfaces[i])
				delete(genericSuperByRaw, rawIfaces[j])
				delete(genericSuperTypeByRaw, rawIfaces[j])
				otherIfaceViews[i] = collectInheritedIfaceViews(funcCtx, rawIfaces[i], nil, true)
				otherIfaceViews[j] = collectInheritedIfaceViews(funcCtx, rawIfaces[j], nil, true)
				reconciled = true
				break
			}
		}
	}

	return reconciled
}

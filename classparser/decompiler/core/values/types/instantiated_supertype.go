package types

import (
	"reflect"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

// ProjectInstantiatedSupertype composes original generic extends/implements
// edges. Each edge must agree with the physical hierarchy. All paths are
// checked, including diamonds: conflicting instantiations grant no source view.
// Raw generic owners and enclosing-owner segments require separate evidence.
func ProjectInstantiatedSupertype(ctx *class_context.ClassContext, source JavaType, target string, provider callbinding.Provider) (JavaType, bool) {
	if ctx == nil || source == nil || target == "" || provider == nil {
		return nil, false
	}
	typeNodes := 0
	typeActive := map[JavaType]bool{}
	var shape func(JavaType, int) bool
	shape = func(t JavaType, depth int) bool {
		typeNodes++
		if t == nil || reflect.ValueOf(t).Kind() == reflect.Pointer && reflect.ValueOf(t).IsNil() || depth > 32 || typeNodes > 1024 || typeActive[t] || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphEdges, 1) != nil {
			return false
		}
		typeActive[t] = true
		defer delete(typeActive, t)
		if t.IsArray() {
			return shape(t.ElementType(), depth+1)
		}
		if p, ok := AsParameterizedType(t); ok {
			if len(p.TypeArgs) > 255 || len(p.OwnerSegments) > 1 {
				return false
			}
			for _, arg := range p.TypeArgs {
				if arg == nil || IsWildcardType(arg) {
					return false
				}
				if _, primitive := arg.RawType().(*JavaPrimer); primitive {
					return false
				}
				if !shape(arg, depth+1) {
					return false
				}
			}
		}
		return !IsWildcardType(t)
	}
	if !shape(source, 0) {
		return nil, false
	}
	remaining := 1024
	active := map[string]bool{}
	var found JavaType
	var walk func(JavaType, int) bool
	walk = func(view JavaType, depth int) bool {
		remaining--
		if view == nil || view.IsArray() || depth > 32 || remaining < 0 || ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return false
		}
		name, args := "", []JavaType(nil)
		if p, ok := AsParameterizedType(view); ok {
			if len(p.OwnerSegments) > 1 {
				return false
			}
			name = dotToInternal(p.RawClassName)
			args = p.TypeArgs
		} else if raw, ok := RawClassFQN(view); ok {
			name = dotToInternal(raw)
		}
		if name == "" || active[name] {
			return false
		}
		active[name] = true
		defer delete(active, name)
		meta, ok := provider(name)
		if !ok || meta.Name != name || !meta.ParentsComplete || len(meta.Parents) > 64 || len(meta.Signature) > 8192 {
			return false
		}
		if ctx.Work != nil && (ctx.Work.CheckAlloc(int64(len(meta.Signature)+len(args)*64)) != nil || ctx.Work.Charge(workbudget.CounterGraphEdges, int64(len(meta.Signature)+len(meta.Parents))) != nil) {
			return false
		}
		depthSig := 0
		for _, ch := range meta.Signature {
			if ch == '<' {
				depthSig++
				if depthSig > 32 {
					return false
				}
			}
			if ch == '>' {
				depthSig--
				if depthSig < 0 {
					return false
				}
			}
		}
		if depthSig != 0 {
			return false
		}
		formals := ClassFormalTypeParamNames(meta.Signature)
		if len(formals) != len(args) {
			return false
		}
		sigma := map[string]JavaType{}
		for i, n := range formals {
			if args[i] == nil || IsWildcardType(args[i]) {
				return false
			}
			sigma[n] = args[i]
		}
		if meta.Signature != "" {
			own, refs, valid := SignatureTypeVariableReferences(meta.Signature)
			if !valid || len(own) != len(formals) {
				return false
			}
			for _, n := range refs {
				if sigma[n] == nil {
					return false
				}
			}
		}
		if name == strings.ReplaceAll(target, ".", "/") {
			if found != nil && !reflect.DeepEqual(found.RawType(), view.RawType()) {
				return false
			}
			found = view
			return true
		}
		if meta.Signature == "" {
			for _, p := range meta.Parents {
				if !walk(NewJavaClass(internalToDot(p)), depth+1) {
					return false
				}
			}
			return true
		}
		super, interfaces := ParseClassSignatureSupers(meta.Signature)
		edges := append([]JavaType{super}, interfaces...)
		// Interfaces carry Object as the Signature's superclass even though the
		// metadata graph intentionally represents only their superinterfaces.
		if meta.IsInterface {
			if raw, ok := RawClassFQN(super); !ok || dotToInternal(raw) != "java/lang/Object" {
				return false
			}
			edges = interfaces
		}
		if len(edges) != len(meta.Parents) {
			return false
		}
		physical := map[string]bool{}
		for _, p := range meta.Parents {
			if physical[p] {
				return false
			}
			physical[p] = true
		}
		for _, edge := range edges {
			raw, ok := RawClassFQN(edge)
			if !ok || !physical[dotToInternal(raw)] {
				return false
			}
			delete(physical, dotToInternal(raw))
			if !walk(SubstituteTypeVars(edge, sigma), depth+1) {
				return false
			}
		}
		return true
	}
	if !walk(source, 0) || found == nil {
		return nil, false
	}
	return found.Copy(), true
}

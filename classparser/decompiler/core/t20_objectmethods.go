package core

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func init() {
	SetFamilyAdapter(FamilyRecord, objectMethodsAdapter)
}

type objectMethodsPlan struct {
	RecordClass string
	Names       []string
	Getters     []*values.JavaClassMember
}

func objectMethodsAdapter(req CallSiteRequest, d *Decompiler, sim StackSimulation, resultType types.JavaType) DispatchResult {
	plan, err := validateObjectMethodsRequest(req)
	if err != nil {
		return invalidDispatch(req, FamilyRecord, DiagBootstrapArgMismatch, err.Error(), resultType)
	}
	receiver, other := objectMethodsOperands(req)
	name := req.CallSiteName
	return okDispatch(req, FamilyRecord, renderObjectMethods(name, plan, receiver, other, resultType), values.EffectCall|values.EffectReadMemory)
}

func validateObjectMethodsRequest(req CallSiteRequest) (*objectMethodsPlan, error) {
	if !req.Identity.Equal(IdentityObjectMethods) {
		return nil, fmt.Errorf("not ObjectMethods.bootstrap identity")
	}
	if len(req.StaticArgs) < 2 {
		return nil, fmt.Errorf("ObjectMethods.bootstrap requires record class and names, got %d static args", len(req.StaticArgs))
	}
	recordClass := classNameFromValue(req.StaticArgs[0])
	if recordClass == "" {
		return nil, fmt.Errorf("ObjectMethods.bootstrap record class is not a Class constant")
	}
	namesRaw, ok := LiteralStringData(req.StaticArgs[1])
	if !ok {
		return nil, fmt.Errorf("ObjectMethods.bootstrap names is not a string constant")
	}
	var names []string
	if strings.TrimSpace(namesRaw) != "" {
		for _, n := range strings.Split(namesRaw, ";") {
			n = strings.TrimSpace(n)
			if n != "" {
				names = append(names, n)
			}
		}
	}
	getters := make([]*values.JavaClassMember, 0, len(req.StaticArgs)-2)
	for i, arg := range req.StaticArgs[2:] {
		cm, ok := arg.(*values.JavaClassMember)
		if !ok || cm == nil {
			return nil, fmt.Errorf("ObjectMethods.bootstrap getter %d is not a method handle member", i)
		}
		if cm.RefKind != 0 && cm.RefKind != RefGetField && cm.RefKind != RefInvokeVirtual && cm.RefKind != RefInvokeSpecial {
			return nil, fmt.Errorf("ObjectMethods.bootstrap getter %d has illegal refkind %d", i, cm.RefKind)
		}
		getters = append(getters, cm)
	}
	if len(getters) != len(names) {
		return nil, fmt.Errorf("ObjectMethods.bootstrap names count %d != getter handle count %d", len(names), len(getters))
	}
	for i, n := range names {
		got := getterFieldName(getters[i])
		if got != n {
			return nil, fmt.Errorf("ObjectMethods.bootstrap getter order mismatch at %d: names %q vs handle %q (not a silent permutation)", i, n, got)
		}
	}
	return &objectMethodsPlan{RecordClass: recordClass, Names: names, Getters: getters}, nil
}

func getterFieldName(cm *values.JavaClassMember) string {
	if cm == nil {
		return ""
	}
	return cm.Member
}

func objectMethodsOperands(req CallSiteRequest) (receiver, other values.JavaValue) {
	n := len(req.DynamicArgs)
	if n == 0 {
		return nil, nil
	}
	if n == 1 {
		return req.DynamicArgs[0], nil
	}
	// Popped last-param-first: equals(this, other) → [other, this]
	return req.DynamicArgs[n-1], req.DynamicArgs[0]
}

func classNameFromValue(v values.JavaValue) string {
	if v == nil {
		return ""
	}
	if cv, ok := v.(*values.JavaClassValue); ok && cv != nil && cv.JavaType != nil {
		raw := cv.JavaType.RawType()
		if jc, ok := raw.(*types.JavaClass); ok && jc != nil {
			return strings.ReplaceAll(jc.Name, "/", ".")
		}
		s := strings.TrimSpace(cv.JavaType.String(&class_context.ClassContext{}))
		s = strings.TrimSuffix(s, ".class")
		return s
	}
	if cm, ok := v.(*values.JavaClassMember); ok && cm != nil {
		return strings.ReplaceAll(cm.Name, "/", ".")
	}
	return ""
}

func renderObjectMethods(call string, plan *objectMethodsPlan, receiver, other values.JavaValue, resultType types.JavaType) values.JavaValue {
	typ := resultType
	return values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
		recv := "this"
		if receiver != nil {
			recv = receiver.String(funcCtx)
		}
		simple := plan.RecordClass
		if i := strings.LastIndex(simple, "."); i >= 0 {
			simple = simple[i+1:]
		}
		if i := strings.LastIndex(simple, "$"); i >= 0 {
			simple = simple[i+1:]
		}
		switch call {
		case "toString":
			if len(plan.Names) == 0 {
				return fmt.Sprintf("%q", simple+"[]")
			}
			parts := make([]string, 0, len(plan.Names))
			for _, fn := range plan.Names {
				parts = append(parts, fn+`=" + `+recv+"."+fn)
			}
			return `"` + simple + `[` + strings.Join(parts, ` + ", `) + ` + "]"`
		case "hashCode":
			if len(plan.Names) == 0 {
				return "0"
			}
			args := make([]string, 0, len(plan.Names))
			for i, fn := range plan.Names {
				args = append(args, hashExpr(funcCtx, recv, fn, getterReturnDesc(plan.Getters, i)))
			}
			if funcCtx != nil {
				funcCtx.Import("java.util.Objects")
			}
			return "java.util.Objects.hash(" + strings.Join(args, ", ") + ")"
		case "equals":
			oth := "null"
			if other != nil {
				oth = other.String(funcCtx)
			}
			cast := simple
			if plan.RecordClass != "" {
				if funcCtx != nil {
					cast = funcCtx.ShortTypeName(plan.RecordClass)
				} else {
					cast = simple
				}
			}
			parts := []string{recv + " == " + oth, oth + " instanceof " + cast}
			for i, fn := range plan.Names {
				parts = append(parts, equalExpr(funcCtx, recv, "("+cast+")"+oth, fn, getterReturnDesc(plan.Getters, i)))
			}
			return strings.Join(parts, " && ")
		default:
			return "/* ObjectMethods " + call + " */ null"
		}
	}, func() types.JavaType {
		if typ == nil {
			return types.NewJavaClass("java.lang.Object")
		}
		return typ
	})
}

func getterReturnDesc(getters []*values.JavaClassMember, i int) string {
	if i < 0 || i >= len(getters) || getters[i] == nil {
		return ""
	}
	desc := getters[i].Description
	if strings.HasPrefix(desc, "(") {
		if k := strings.IndexByte(desc, ')'); k >= 0 && k+1 < len(desc) {
			return desc[k+1:]
		}
	}
	return desc
}

func hashExpr(funcCtx *class_context.ClassContext, recv, field, desc string) string {
	acc := recv + "." + field
	switch desc {
	case "[B", "[S", "[I", "[J", "[F", "[D", "[C", "[Z":
		if funcCtx != nil {
			funcCtx.Import("java.util.Arrays")
		}
		return "java.util.Arrays.hashCode(" + acc + ")"
	}
	if strings.HasPrefix(desc, "[") {
		if funcCtx != nil {
			funcCtx.Import("java.util.Arrays")
		}
		return "java.util.Arrays.hashCode(" + acc + ")"
	}
	return acc
}

func equalExpr(funcCtx *class_context.ClassContext, recv, other, field, desc string) string {
	a := recv + "." + field
	b := other + "." + field
	switch desc {
	case "F":
		return "Float.compare(" + a + ", " + b + ") == 0"
	case "D":
		return "Double.compare(" + a + ", " + b + ") == 0"
	case "J":
		return a + " == " + b
	case "I", "S", "B", "C", "Z":
		return a + " == " + b
	}
	if strings.HasPrefix(desc, "[") {
		if funcCtx != nil {
			funcCtx.Import("java.util.Arrays")
		}
		// Shallow Arrays.equals, never deepEquals.
		return "java.util.Arrays.equals(" + a + ", " + b + ")"
	}
	if funcCtx != nil {
		funcCtx.Import("java.util.Objects")
	}
	return "java.util.Objects.equals(" + a + ", " + b + ")"
}

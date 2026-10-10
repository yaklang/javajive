package core

import (
	"strconv"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// The instantiated SAM checks arguments BEFORE the implementation handle runs.
// Its erased entry descriptor and dynamic input restrictions are distinct from
// a consumer's generic Signature. Emit the original reference checks at a raw
// SAM entry, then call the original static handle through non-throwing descriptor
// views. The views also prevent narrower same-name overloads from replacing it.
// This input-only proof excludes captures, unboxing and result adaptation.
func methodRefCheckedInput(d *Decompiler, req CallSiteRequest, static []values.JavaValue, impl *values.JavaClassMember, raw types.JavaType) values.JavaValue {
	if d == nil || d.FunctionContext == nil || impl == nil || impl.RefKind != RefInvokeStatic || len(req.DynamicArgs) != 0 || len(static) != 3 || d.blockPartialFunctionalTarget || class_context.SafeIdentifier(impl.Member) != impl.Member {
		return nil
	}
	inputs, result, err := callbinding.Descriptor(impl.Description)
	erased, erasedResult, erasedErr := callbinding.Descriptor(t19MethodTypeDesc(static[0]))
	actual, actualResult, actualErr := callbinding.Descriptor(t19MethodTypeDesc(static[2]))
	if err != nil || erasedErr != nil || actualErr != nil || len(inputs) == 0 || len(inputs) != len(actual) || len(erased) != len(actual) || result != actualResult || erasedResult != actualResult {
		return nil
	}
	ctx := d.FunctionContext
	if ctx.InvocationMetadata == nil {
		return nil
	}
	// Widening views must come from original, complete parent declarations.
	// A provider entry for a different symbol cannot authorize a source cast.
	parents := func(name string) (callbinding.Class, bool) {
		if ctx.Work != nil && ctx.Work.Charge(workbudget.CounterGraphScans, 1) != nil {
			return callbinding.Class{}, false
		}
		declaration, known := ctx.InvocationMetadata(name)
		return declaration, known && declaration.Name == name && declaration.ParentsComplete
	}
	changed := false
	for i, parameter := range actual {
		if !callbinding.Assignable(parameter, inputs[i], parents) || !callbinding.Assignable(parameter, erased[i], parents) {
			return nil
		}
		changed = changed || callbinding.Reference(parameter) && parameter != erased[i] && parameter != inputs[i]
	}
	if !changed {
		return nil
	}
	rawName, known := types.RawClassFQN(raw)
	if !known {
		return nil
	}
	erasedInterface := types.NewJavaClass(rawName)
	declaration, known := ctx.InvocationMetadata(strings.ReplaceAll(rawName, ".", "/"))
	if !known || declaration.Name != strings.ReplaceAll(rawName, ".", "/") || !declaration.IsInterface {
		return nil
	}
	sam, err := callbinding.FamilyOf(callbinding.Witness{Owner: declaration.Name, Name: req.CallSiteName, Desc: t19MethodTypeDesc(static[0]), Kind: callbinding.Interface}, ctx.InvocationMetadata)
	if err != nil || !sam.Complete || sam.Target == nil || sam.Target.Static {
		return nil
	}
	family, err := callbinding.FamilyOf(callbinding.Witness{Owner: strings.ReplaceAll(impl.Name, ".", "/"), Name: impl.Member, Desc: impl.Description, Kind: callbinding.Static}, ctx.InvocationMetadata)
	if err != nil || !family.Complete || family.Target == nil || !family.Target.Static {
		return nil
	}
	for _, method := range family.Methods {
		if method.Generic || method.Varargs || method.Bridge {
			return nil
		}
	}
	owner := strings.ReplaceAll(impl.Name, "/", ".")
	ownerDeclaration, known := ctx.InvocationMetadata(strings.ReplaceAll(owner, ".", "/"))
	if !known || ownerDeclaration.Name != strings.ReplaceAll(owner, ".", "/") {
		return nil
	}
	parameters := make([]types.JavaType, len(erased))
	checked := make([]types.JavaType, len(actual))
	views := make([]types.JavaType, len(inputs))
	for i := range actual {
		parameters[i], err = types.ParseDescriptor(erased[i])
		if err != nil {
			return nil
		}
		checked[i], err = types.ParseDescriptor(actual[i])
		if err != nil {
			return nil
		}
		views[i], err = types.ParseDescriptor(inputs[i])
		if err != nil {
			return nil
		}
	}
	value := values.NewStreamingCustomValue(func(ctx *class_context.ClassContext, out *workbudget.Writer) error {
		// A source value may obscure the original type name. Use the shared
		// class/interface binding proof; interface static methods cannot use
		// a null primary, while class static methods may discard that primary.
		var prefix string
		if ownerDeclaration.IsInterface {
			prefix = ctx.StaticInterfaceCallPrefix(owner, impl.Member, impl.Description)
		} else {
			prefix = ctx.StaticClassCallPrefix(owner, impl.Member, impl.Description)
		}
		if err := ctx.StaticMethodImports.Error(); err != nil {
			return err
		}
		// Choose parameter AND checked-local names after source locals, handlers
		// and lexical types are bound. Ordinary generated locals use varN.
		if ctx.Work != nil {
			entries := int64(len(ctx.LocalNames) + len(ctx.Arguments) + len(ctx.CatchEntryNames) + len(parameters)*2)
			if err := ctx.Work.Charge(workbudget.CounterGraphScans, entries); err != nil {
				return err
			}
			if err := ctx.CheckAlloc(entries * 96); err != nil {
				return err
			}
		}
		used := map[string]bool{}
		// A generated parameter cannot reclassify the leading type/package
		// name of the already-proved static selection as a value name.
		root := strings.SplitN(prefix, ".", 2)[0]
		if class_context.SafeIdentifier(root) == root {
			used[root] = true
		}
		for _, name := range ctx.Arguments {
			used[name] = true
		}
		for _, name := range ctx.LocalNames {
			used[name] = true
		}
		for _, name := range ctx.CatchEntryNames {
			used[name] = true
		}
		names := make([]string, len(parameters))
		for i, next := 0, 0; i < len(names); next++ {
			if ctx.Work != nil {
				if err := ctx.Work.Charge(workbudget.CounterGraphScans, 1); err != nil {
					return err
				}
			}
			name := "lambdaArgument" + strconv.Itoa(next)
			if used[name] || used[name+"Checked"] || ctx.LexicalTypeNames[name] || ctx.LexicalTypeNames[name+"Checked"] {
				continue
			}
			names[i], used[name], used[name+"Checked"] = name, true, true
			i++
		}
		// Freeze the erased creation target before any consumer can retarget
		// this poly expression. It checks only the interface created here.
		if err := out.WriteString("(" + erasedInterface.String(ctx) + ") ("); err != nil {
			return err
		}
		if err := out.WriteString("("); err != nil {
			return err
		}
		for i, parameter := range parameters {
			if i != 0 {
				if err := out.WriteString(", "); err != nil {
					return err
				}
			}
			if err := out.WriteString(parameter.String(ctx) + " " + names[i]); err != nil {
				return err
			}
		}
		if err := out.WriteString(") -> { "); err != nil {
			return err
		}
		for i, parameter := range checked {
			if erased[i] == actual[i] {
				continue
			}
			typ := parameter.String(ctx)
			if err := out.WriteString(typ + " " + names[i] + "Checked = (" + typ + ") " + names[i] + "; "); err != nil {
				return err
			}
		}
		if result != "V" {
			if err := out.WriteString("return "); err != nil {
				return err
			}
		}
		if err := out.WriteString(prefix + class_context.SafeIdentifier(impl.Member) + "("); err != nil {
			return err
		}
		for i, view := range views {
			if i != 0 {
				if err := out.WriteString(", "); err != nil {
					return err
				}
			}
			name := names[i]
			if erased[i] != actual[i] {
				name += "Checked"
			}
			if err := out.WriteString("(" + view.String(ctx) + ") " + name); err != nil {
				return err
			}
		}
		return out.WriteString("); })")
	}, func() types.JavaType { return erasedInterface })
	value.Flag, value.NoOuterCapture, value.CapturesKnown = "lambda", true, true
	value.InstantiatedMtdDesc = t19MethodTypeDesc(static[2])
	return value
}

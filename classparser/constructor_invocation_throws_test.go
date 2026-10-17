package javaclassparser

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func constructorThrowsBindingMetadata() map[string]callbinding.Class {
	result := map[string]callbinding.Class{}
	for name, parents := range map[string][]string{
		"java/lang/Object": nil, "java/lang/Throwable": {"java/lang/Object"},
		"java/lang/Exception": {"java/lang/Throwable"}, "java/lang/RuntimeException": {"java/lang/Exception"},
		"java/io/IOException": {"java/lang/Exception"}, "java/io/FileNotFoundException": {"java/io/IOException"},
		"java/sql/SQLException": {"java/lang/Exception"}, "java/lang/String": {"java/lang/Object"},
		"p/Marker": {"java/lang/Object"}, "p/Marked": {"java/io/IOException", "p/Marker"},
		"E": {"java/lang/Throwable"}, "p/NotInterface": {"java/lang/Object"},
	} {
		result[name] = callbinding.Class{Name: name, Parents: parents, ParentsComplete: true, MembersComplete: true, Public: true, IsInterface: name == "p/Marker"}
	}
	return result
}

func TestAdversarialConstructorGenericInvocationThrowsBindingRequiresCompleteConstraints(t *testing.T) {
	const loose = "<E:Ljava/lang/Throwable;>(ZLjava/lang/Throwable;)V^TE;"
	const argument = "<E:Ljava/lang/Throwable;>(TE;)V^TE;"
	for _, test := range []struct {
		name, class, sig, desc string
		actuals, erased, want  []string
		mutate                 func(map[string]callbinding.Class)
	}{
		{name: "throws-only Throwable", sig: loose, desc: "(ZLjava/lang/Throwable;)V", actuals: []string{"Z", "Ljava/lang/Throwable;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "throws-only Exception", sig: "<E:Ljava/lang/Exception;>()V^TE;", desc: "()V", erased: []string{"java/lang/Exception"}, want: []string{"java/lang/RuntimeException"}},
		{name: "explicit checked argument precedes throws preference", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/io/IOException"}},
		{name: "explicit broad argument stays checked", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/lang/Throwable;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/Throwable"}},
		{name: "explicit unchecked argument", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/lang/RuntimeException;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "null supplies no lower bound", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"null"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "ordered lower-bound join", sig: "<E:Ljava/lang/Throwable;>(TE;TE;)V^TE;", desc: "(Ljava/lang/Throwable;Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;", "Ljava/io/FileNotFoundException;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/io/IOException"}},
		{name: "reverse lower-bound join", sig: "<E:Ljava/lang/Throwable;>(TE;TE;)V^TE;", desc: "(Ljava/lang/Throwable;Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/FileNotFoundException;", "Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/io/IOException"}},
		{name: "incomparable lower bounds need separate LUB proof", sig: "<E:Ljava/lang/Throwable;>(TE;TE;)V^TE;", desc: "(Ljava/lang/Throwable;Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;", "Ljava/sql/SQLException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "complete intersection", sig: "<E:Ljava/lang/Exception;:Lp/Marker;>(TE;)V^TE;", desc: "(Ljava/lang/Exception;)V", actuals: []string{"Lp/Marked;"}, erased: []string{"java/lang/Exception"}, want: []string{"p/Marked"}},
		{name: "unsatisfied later bound", sig: "<E:Ljava/lang/Exception;:Lp/Marker;>(TE;)V^TE;", desc: "(Ljava/lang/Exception;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Exception"}},
		{name: "unconstrained intersection is not RuntimeException", sig: "<E:Ljava/lang/Exception;:Lp/Marker;>()V^TE;", desc: "()V", erased: []string{"java/lang/Exception"}},
		{name: "later bound must be interface", sig: "<E:Ljava/lang/Exception;:Lp/NotInterface;>(TE;)V^TE;", desc: "(Ljava/lang/Exception;)V", actuals: []string{"Lp/Marked;"}, erased: []string{"java/lang/Exception"}},
		{name: "checked upper bound does not acquire unchecked preference", sig: "<E:Ljava/io/IOException;>()V^TE;", desc: "()V", erased: []string{"java/io/IOException"}},
		{name: "checked upper bound constrains actual", sig: "<E:Ljava/io/IOException;>(TE;)V^TE;", desc: "(Ljava/io/IOException;)V", actuals: []string{"Ljava/io/FileNotFoundException;"}, erased: []string{"java/io/IOException"}, want: []string{"java/io/FileNotFoundException"}},
		{name: "method formal shadows same-named class formal", class: "<E:Ljava/io/IOException;>Ljava/lang/Object;", sig: loose, desc: "(ZLjava/lang/Throwable;)V", actuals: []string{"Z", "Ljava/lang/Throwable;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "class formal does not acquire method inference", class: "<E:Ljava/lang/Throwable;>Ljava/lang/Object;", sig: "(TE;)V^TE;", desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "nominal E is not a formal E input", sig: "<E:Ljava/lang/Throwable;>(LE;)V^TE;", desc: "(LE;)V", actuals: []string{"LE;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}},
		{name: "fixed throws retained beside inferred throws", sig: "<E:Ljava/lang/Throwable;>(TE;)V^TE;^Ljava/sql/SQLException;", desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable", "java/sql/SQLException"}, want: []string{"java/io/IOException", "java/sql/SQLException"}},
		{name: "missing actual ancestry", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Lp/Missing;"}, erased: []string{"java/lang/Throwable"}},
		{name: "nested variable input", sig: "<E:Ljava/lang/Throwable;>([TE;)V^TE;", desc: "([Ljava/lang/Throwable;)V", actuals: []string{"[Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "dependent bound", sig: "<A:Ljava/lang/Throwable;E:TA;>(TE;)V^TE;", desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "invalid throwable first bound", sig: "<E:Ljava/lang/Object;>(TE;)V^TE;", desc: "(Ljava/lang/Object;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Object"}},
		{name: "raw Exceptions disagree with Signature", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/io/IOException"}},
		{name: "wrong erased descriptor", sig: argument, desc: "(Ljava/lang/Exception;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "unknown Exceptions", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods[0].ExceptionsKnown = false
			m["p/Base"] = c
		}},
		{name: "incomplete method table", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) { c := m["p/Base"]; c.MembersComplete = false; m["p/Base"] = c }},
		{name: "wrong owner identity", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) { c := m["p/Base"]; c.Name = "p/Other"; m["p/Base"] = c }},
		{name: "duplicate exact declaration", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods = append(c.Methods, c.Methods[0])
			m["p/Base"] = c
		}},
		{name: "same-arity rival not excluded", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "(Ljava/io/IOException;)V", ExceptionsKnown: true})
			m["p/Base"] = c
		}},
		{name: "varargs rival", sig: argument, desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "([Ljava/lang/Object;)V", Varargs: true, ExceptionsKnown: true})
			m["p/Base"] = c
		}},
		{name: "boolean/numeric rival excluded", sig: loose, desc: "(ZLjava/lang/Throwable;)V", actuals: []string{"Z", "Ljava/lang/Throwable;"}, erased: []string{"java/lang/Throwable"}, want: []string{"java/lang/RuntimeException"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "(ILjava/lang/Throwable;)V", ExceptionsKnown: true})
			m["p/Base"] = c
		}},
		{name: "excluded rival Signature must retain descriptor", sig: loose, desc: "(ZLjava/lang/Throwable;)V", actuals: []string{"Z", "Ljava/lang/Throwable;"}, erased: []string{"java/lang/Throwable"}, mutate: func(m map[string]callbinding.Class) {
			c := m["p/Base"]
			c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "(ILjava/lang/Throwable;)V", Generic: true, Signature: "<T:Ljava/lang/Object;>(ZLjava/lang/Throwable;)V", ExceptionsKnown: true})
			m["p/Base"] = c
		}},
		{name: "unknown intersection hierarchy", sig: "<E:Ljava/lang/Exception;:Lp/Marker;>(TE;)V^TE;", desc: "(Ljava/lang/Exception;)V", actuals: []string{"Lp/Marked;"}, erased: []string{"java/lang/Exception"}, mutate: func(m map[string]callbinding.Class) { c := m["p/Marked"]; c.ParentsComplete = false; m["p/Marked"] = c }},
		{name: "truncated Signature", sig: "<E:Ljava/lang/Throwable;>(TE;", desc: "(Ljava/lang/Throwable;)V", actuals: []string{"Ljava/io/IOException;"}, erased: []string{"java/lang/Throwable"}},
		{name: "oversized Signature", sig: strings.Repeat("x", 8193), desc: "()V"},
	} {
		t.Run(test.name, func(t *testing.T) {
			metadata := constructorThrowsBindingMetadata()
			metadata["p/Base"] = callbinding.Class{Name: "p/Base", Signature: test.class, Parents: []string{"java/lang/Object"}, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: test.desc, Signature: test.sig, Generic: true, Public: true, ExceptionsKnown: true, Exceptions: append([]string(nil), test.erased...)}}}
			if test.mutate != nil {
				test.mutate(metadata)
			}
			before := append([]string(nil), metadata["p/Base"].Methods[0].Exceptions...)
			provider := func(name string) (callbinding.Class, bool) { c, known := metadata[name]; return c, known }
			got, known := constructorGenericInvocationThrows(provider, "p/Base", test.desc, test.actuals, nil)
			if known != (test.want != nil) || known && !reflect.DeepEqual(got, test.want) {
				t.Fatalf("bound throws %v known=%v want=%v", got, known, test.want)
			}
			if !reflect.DeepEqual(before, metadata["p/Base"].Methods[0].Exceptions) {
				t.Fatal("original Exceptions were mutated")
			}
		})
	}
}

func TestAdversarialConstructorGenericInvocationThrowsWorkAndMissingMetadata(t *testing.T) {
	for _, mode := range []string{"missing provider", "missing owner", "work", "memory", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			metadata := constructorThrowsBindingMetadata()
			metadata["p/Base"] = callbinding.Class{Name: "p/Base", ParentsComplete: true, MembersComplete: true, Methods: []callbinding.Method{{Name: "<init>", Desc: "()V", Signature: "<E:Ljava/lang/Throwable;>()V^TE;", Generic: true, ExceptionsKnown: true, Exceptions: []string{"java/lang/Throwable"}}}}
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) { c, known := metadata[name]; return c, known })
			var work *workbudget.Budget
			switch mode {
			case "missing provider":
				provider = nil
			case "missing owner":
				delete(metadata, "p/Base")
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got, known := constructorGenericInvocationThrows(provider, "p/Base", "()V", nil, work); known {
				t.Fatal("unproved metadata/work must not narrow original throws", got)
			}
		})
	}
}

func TestAdversarialConstructorGenericInvocationThrowsExactSourcePlanAndHeaderFallback(t *testing.T) {
	files := nativePrivateEnumCompile(t, `class InferredHeaderBase {<E extends Throwable>InferredHeaderBase(boolean b,Throwable value)throws E{}}
class InferredHeader extends InferredHeaderBase {InferredHeader(boolean b,Throwable value){super(false,value);}}`, "InferredHeader", "none")
	for _, mode := range []string{"exact", "no plan", "wrong PC", "wrong origin", "wrong owner", "wrong descriptor", "wrong receiver", "source seven", "missing source bounds", "missing Signature", "unknown Exceptions", "complete ambiguous overload", "ordinary checked declaration", "wrong method", "duplicate method", "duplicate Code"} {
		t.Run(mode, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["InferredHeader.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options.TargetSourceVersion = 8
			resolve := func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }
			d.foldSiblingResolver = resolve
			provider := d.buildInvocationMetadata()
			d.FuncCtx = &class_context.ClassContext{ClassName: "InferredHeader", FunctionName: "<init>", CurrentMethodDesc: "(ZLjava/lang/Throwable;)V", InvocationMetadata: provider}
			var code *CodeAttribute
			for _, a := range object.Methods[0].Attributes {
				if raw, ok := a.(*CodeAttribute); ok {
					code = raw
				}
			}
			if code == nil {
				t.Fatal("authored original constructor code missing")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			pc := -1
			for _, op := range decoder.Opcodes() {
				if m := constructorMotionMember(object, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "InferredHeaderBase" {
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("original delegation missing")
			}
			flag := constructorThrowsTestParameter(1, types.NewJavaPrimer(types.JavaBoolean), "flag")
			argument := constructorThrowsTestParameter(2, types.NewJavaClass("java.lang.Throwable"), "value")
			call := &values.FunctionCallExpression{ClassName: "InferredHeaderBase", FunctionName: "<init>", Descriptor: "(ZLjava/lang/Throwable;)V", Kind: values.InvokeSpecial, IsSpecialInvoke: true, Object: constructorThrowsTestReceiver("InferredHeader"), HasOriginPC: true, OriginPC: pc, Arguments: []values.JavaValue{values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), argument}}
			plan := &constructorSourceBoundary{pc: pc, delegate: call, params: []values.JavaValue{flag, argument}}
			switch mode {
			case "no plan":
				plan = nil
			case "wrong PC":
				plan.pc++
			case "wrong origin":
				call.HasOriginPC = false
			case "wrong owner":
				call.ClassName = "OtherBase"
			case "wrong descriptor":
				call.Descriptor = "(ZLjava/lang/Exception;)V"
			case "wrong receiver":
				call.Object = &values.JavaRef{}
			case "source seven":
				d.options.TargetSourceVersion = 7
			case "missing source bounds":
				call.Arguments[1] = values.NewJavaRef(nil, nil, types.NewJavaTypeVariable("Missing"))
			case "wrong method":
				d.FuncCtx.CurrentMethodDesc = "()V"
			case "duplicate method":
				object.Methods = append(object.Methods, object.Methods[0])
			case "duplicate Code":
				object.Methods[0].Attributes = append(object.Methods[0].Attributes, code)
			case "missing Signature", "unknown Exceptions", "complete ambiguous overload", "ordinary checked declaration":
				d.FuncCtx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
					c, ok := provider(n)
					if n == "InferredHeaderBase" && ok {
						c.Methods = append([]callbinding.Method(nil), c.Methods...)
						switch mode {
						case "missing Signature":
							c.Methods[0].Signature = ""
						case "unknown Exceptions":
							c.Methods[0].ExceptionsKnown = false
						case "complete ambiguous overload":
							c.Methods = append(c.Methods, callbinding.Method{Name: "<init>", Desc: "(ZLjava/lang/Exception;)V", ExceptionsKnown: true})
						case "ordinary checked declaration":
							c.Methods[0].Generic = false
							c.Methods[0].Signature = ""
						}
					}
					return c, ok
				}
			}
			want := mode == "exact"
			var receipt *constructorInvocationThrowsReceipt
			var proved bool
			if plan != nil {
				member := constructorMotionMember(object, decoder.OpcodeByPC(uint16(pc)), core.OP_INVOKESPECIAL)
				receipt, proved = d.proveConstructorInvocationThrows(code, object.Methods[0], member, pc, plan)
				plan.throwsReceipt = receipt
			}
			if proved != want {
				t.Fatalf("exact source receipt=%v want %v", proved, want)
			}
			if legacy, err := d.superCtorCheckedThrows(); err != nil || legacy != "Throwable" {
				t.Fatalf("legacy raw checked declaration disappeared: %q %v", legacy, err)
			}
			if want {
				argument.Id.SetName("settledBeforeHeader")
				if !receipt.fresh(d.FuncCtx) {
					t.Fatal("source names may settle before header projection")
				}
				if receipt.bindSourceParameters(plan, nil, d.FuncCtx) || receipt.parameterBindingCount != 0 {
					t.Fatal("failed parameter binding must be atomic")
				}
				if !receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
					t.Fatal("original typed header binding failed")
				}
				if receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
					t.Fatal("parameter header binding must consume once")
				}
				if receipt.consumed(d.FuncCtx) || receipt.confirmEmission(plan, d.FuncCtx, "\nsuper(false,null);\n", "") {
					t.Fatal("unused receipt claimed by text")
				}
				if _, err := d.superCtorCheckedThrows(plan); err == nil {
					t.Fatal("unused receipt suppressed header fallback")
				}
				sibling := &constructorSourceBoundary{pc: pc, delegate: call, params: plan.params, throwsReceipt: receipt}
				if _, err := sibling.renderDelegation(d.FuncCtx); err == nil || receipt.renderCount != 0 {
					t.Fatal("sibling plan borrowed receipt")
				}
				prior := call.Object
				call.Object = &values.JavaRef{IsThis: true}
				if _, err := plan.renderDelegation(d.FuncCtx); err == nil || receipt.renderCount != 0 || receipt.headerCount != 0 {
					t.Fatal("early render error consumed receipt")
				}
				call.Object = prior
				source, err := plan.renderDelegation(d.FuncCtx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := plan.renderDelegation(d.FuncCtx); err == nil {
					t.Fatal("duplicate receipt render accepted")
				}
				if receipt.claimHeader(sibling, call, pc, source, d.FuncCtx) || receipt.claimHeader(plan, call.Clone(), pc, source, d.FuncCtx) || receipt.claimHeader(plan, call, pc+1, source, d.FuncCtx) || receipt.claimHeader(plan, call, pc, source+" ", d.FuncCtx) {
					t.Fatal("detached/foreign header claim accepted")
				}
				if !receipt.claimHeader(plan, call, pc, source, d.FuncCtx) || receipt.claimHeader(plan, call, pc, source, d.FuncCtx) {
					t.Fatal("header claim must consume exactly once")
				}
				body := "\n" + source + ";\n"
				if receipt.confirmEmission(sibling, d.FuncCtx, body, "") || receipt.confirmEmission(plan, d.FuncCtx, "//"+body, "") || receipt.confirmEmission(plan, d.FuncCtx, "\nother();\n"+body, "") {
					t.Fatal("text/comment/sibling occurrence proved emission")
				}
				if !receipt.confirmEmission(plan, d.FuncCtx, body, "") || receipt.confirmEmission(plan, d.FuncCtx, body, "") || !receipt.consumed(d.FuncCtx) {
					t.Fatal("final body emission must consume exactly once")
				}
				plan.throwsReceipt = nil
				if receipt.consumed(d.FuncCtx) {
					t.Fatal("detached method receipt remained consumed")
				}
				plan.throwsReceipt = receipt
			}
			wantHeader := "Throwable"
			if want {
				wantHeader = ""
			}
			if got, err := d.superCtorCheckedThrows(plan); err != nil || got != wantHeader {
				t.Fatalf("header fallback %q want %q: %v", got, wantHeader, err)
			}
			if want {
				for _, stale := range []string{"receiver", "operand", "source type", "literal", "literal units", "PC", "descriptor", "source version", "method flags", "raw code", "method Signature", "provider", "callee declaration", "carrier", "source identifier", "context identifier", "parameter seed", "MaxStack", "MaxLocals", "handlers", "subtype declaration", "force header type", "standalone source type", "source declaration role"} {
					t.Run("stale "+stale, func(t *testing.T) {
						var undo func()
						switch stale {
						case "receiver":
							prior := call.Object
							call.Object = &values.JavaRef{IsThis: true}
							undo = func() { call.Object = prior }
						case "operand":
							prior := call.Arguments[1]
							call.Arguments[1] = values.NewJavaRef(coreutils.NewRootVariableId(), nil, prior.Type())
							undo = func() { call.Arguments[1] = prior }
						case "source type":
							ref := call.Arguments[1].(*values.JavaRef)
							prior := ref.Type().Copy()
							ref.ResetVarType(types.NewJavaClass("java.lang.Exception"))
							undo = func() { ref.ResetVarType(prior) }
						case "literal":
							literal := call.Arguments[0].(*values.JavaLiteral)
							prior := literal.Data
							literal.Data = true
							undo = func() { literal.Data = prior }
						case "literal units":
							literal := call.Arguments[0].(*values.JavaLiteral)
							literal.Units = []uint16{'x'}
							undo = func() { literal.Units = nil }
						case "PC":
							call.OriginPC++
							undo = func() { call.OriginPC-- }
						case "descriptor":
							prior := call.Descriptor
							call.Descriptor = "()V"
							undo = func() { call.Descriptor = prior }
						case "source version":
							d.options.TargetSourceVersion = 7
							undo = func() { d.options.TargetSourceVersion = 8 }
						case "method flags":
							object.Methods[0].AccessFlags ^= 0x10
							undo = func() { object.Methods[0].AccessFlags ^= 0x10 }
						case "raw code":
							prior := code.Code[len(code.Code)-1]
							code.Code[len(code.Code)-1] = byte(core.OP_NOP)
							undo = func() { code.Code[len(code.Code)-1] = prior }
						case "method Signature":
							d.FuncCtx.CurrentMethodSig = "<X:Ljava/lang/Object;>()V"
							undo = func() { d.FuncCtx.CurrentMethodSig = "" }
						case "provider":
							d.FuncCtx.InvocationMetadata = nil
							undo = func() { d.FuncCtx.InvocationMetadata = provider }
						case "callee declaration":
							d.FuncCtx.InvocationMetadata = func(n string) (callbinding.Class, bool) {
								m, known := provider(n)
								if n == "InferredHeaderBase" && known {
									m.Methods = append([]callbinding.Method(nil), m.Methods...)
									m.Methods[0].Signature = strings.ReplaceAll(m.Methods[0].Signature, "E", "F")
								}
								return m, known
							}
							undo = func() { d.FuncCtx.InvocationMetadata = provider }
						case "source identifier":
							ref := call.Arguments[1].(*values.JavaRef)
							prior := ref.Id.Name
							ref.Id.SetName("renamedAfterHeader")
							undo = func() { ref.Id.SetName(prior) }
						case "context identifier":
							prior := d.FuncCtx.LocalNames
							d.FuncCtx.LocalNames = map[*coreutils.VariableId]string{argument.Id: "renamedAfterHeader"}
							undo = func() { d.FuncCtx.LocalNames = prior }
						case "parameter seed":
							prior := argument.Val
							argument.Val = values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))
							undo = func() { argument.Val = prior }
						case "MaxStack":
							code.MaxStack++
							undo = func() { code.MaxStack-- }
						case "MaxLocals":
							code.MaxLocals++
							undo = func() { code.MaxLocals-- }
						case "handlers":
							prior := code.ExceptionTable
							code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: uint16(pc), HandlerPc: 0})
							undo = func() { code.ExceptionTable = prior }
						case "subtype declaration":
							if _, queried := receipt.metadataHashes["java/lang/RuntimeException"]; !queried {
								t.Fatal("stale metadata test must alter an actually queried declaration")
							}
							d.FuncCtx.InvocationMetadata = func(name string) (callbinding.Class, bool) {
								m, known := provider(name)
								if name == "java/lang/RuntimeException" && known {
									m.Public = !m.Public
								}
								return m, known
							}
							undo = func() { d.FuncCtx.InvocationMetadata = provider }
						case "force header type":
							prior := d.FuncCtx.ForceParamEraseTypeVars
							d.FuncCtx.ForceParamEraseTypeVars = map[string]string{"java.lang.Throwable": "java.lang.Object"}
							undo = func() { d.FuncCtx.ForceParamEraseTypeVars = prior }
						case "standalone source type":
							prior := d.FuncCtx.StandaloneEraseTypeVars
							d.FuncCtx.StandaloneEraseTypeVars = map[string]string{"java.lang.Throwable": "java.lang.Object"}
							undo = func() { d.FuncCtx.StandaloneEraseTypeVars = prior }
						case "source declaration role":
							prior := d.FuncCtx.DeclarationSourceName
							d.FuncCtx.DeclarationSourceName = func(string) (string, bool) { return "java.lang.Object", true }
							undo = func() { d.FuncCtx.DeclarationSourceName = prior }
						case "carrier":
							plan.carrier = "other"
							undo = func() { plan.carrier = "" }
						}
						if receipt.fresh(d.FuncCtx) {
							t.Fatal("stale source receipt accepted")
						}
						if _, err := plan.renderDelegation(d.FuncCtx); err == nil {
							t.Fatal("stale source receipt must refuse the delegation")
						}
						if _, err := d.superCtorCheckedThrows(plan); err == nil {
							t.Fatal("stale source receipt must not fall back and invent throws")
						}
						undo()
						if !receipt.fresh(d.FuncCtx) {
							t.Fatal("restored immutable source receipt rejected")
						}
					})
				}
			}

		})
	}
}

func TestAdversarialConstructorGenericInvocationThrowsReceiptPinsLegacyCastSourceBinding(t *testing.T) {
	files := nativePrivateEnumCompile(t, `class ArgumentViewBase {<E extends Throwable>ArgumentViewBase(boolean b,E value)throws E{}}
class ArgumentView extends ArgumentViewBase {ArgumentView(boolean b,java.io.IOException value)throws java.io.IOException{super(false,value);}}`, "ArgumentView", "none")
	for _, flag := range []string{"normal", "JDEC_NO_ERASED_TYPEVAR_NOCAST", "JDEC_SIBLING_CTOR_SIG_OFF"} {
		t.Run(flag, func(t *testing.T) {
			object, err := Parse(bytes.Clone(files["ArgumentView.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(object)
			d.options.TargetSourceVersion = 8
			d.foldSiblingResolver = func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return bytes.Clone(raw), ok }
			d.FuncCtx = &class_context.ClassContext{ClassName: "ArgumentView", SupperClassName: "ArgumentViewBase", FunctionName: "<init>", CurrentMethodDesc: "(ZLjava/io/IOException;)V", InvocationMetadata: d.buildInvocationMetadata(), Env: func(k string) string {
				if k == flag {
					return "1"
				}
				return ""
			}}
			var code *CodeAttribute
			for _, a := range object.Methods[0].Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					code = c
				}
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(d.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			var member *values.JavaClassMember
			pc := -1
			for _, op := range decoder.Opcodes() {
				if m := constructorMotionMember(object, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "ArgumentViewBase" {
					member = m
					pc = int(op.CurrentOffset)
				}
			}
			ft, err := types.ParseMethodDescriptor(member.Description)
			if err != nil {
				t.Fatal(err)
			}
			arg := constructorThrowsTestParameter(2, types.NewJavaClass("java.io.IOException"), "value")
			flagParam := constructorThrowsTestParameter(1, types.NewJavaPrimer(types.JavaBoolean), "flag")
			call := &values.FunctionCallExpression{ClassName: "ArgumentViewBase", FunctionName: "<init>", Descriptor: member.Description, Kind: values.InvokeSpecial, IsSpecialInvoke: true, HasOriginPC: true, OriginPC: pc, Object: constructorThrowsTestReceiver("ArgumentView"), FuncType: ft.FunctionType(), Arguments: []values.JavaValue{values.NewJavaLiteral(false, types.NewJavaPrimer(types.JavaBoolean)), arg}}
			legacy := call.ArgumentStrings(d.FuncCtx)
			if len(legacy) != 2 || !strings.Contains(legacy[1], "Throwable") {
				t.Fatalf("red legacy cast control did not widen the IOException input: %v", legacy)
			}
			plan := &constructorSourceBoundary{pc: pc, delegate: call, params: []values.JavaValue{flagParam, arg}}
			receipt, known := d.proveConstructorInvocationThrows(code, object.Methods[0], member, pc, plan)
			if !known || !reflect.DeepEqual(receipt.exceptions, []string{"java/io/IOException"}) {
				t.Fatal("actual IOException lower bound lost", known)
			}
			plan.throwsReceipt = receipt
			if !receipt.bindSourceParameters(plan, plan.params, d.FuncCtx) {
				t.Fatal("original typed header binding failed")
			}
			source, err := plan.renderDelegation(d.FuncCtx)
			if err != nil || strings.Contains(source, "Throwable") || !strings.Contains(source, arg.String(d.FuncCtx)) {
				t.Fatalf("receipt source must consume its exact IOException input: %q %v", source, err)
			}
			// An explicit source cast really does broaden the input; it must not
			// borrow the previous receipt or the caller's IOException declaration.
			call.Arguments[1] = &values.CastExpression{Value: arg, TargetType: types.NewJavaClass("java.lang.Throwable")}
			if receipt.fresh(d.FuncCtx) {
				t.Fatal("explicit broad cast reused narrower receipt")
			}
			widened, known := d.constructorDelegationInvocationThrows(member, pc, call)
			if !known || !reflect.DeepEqual(widened, []string{"java/lang/Throwable"}) {
				t.Fatal("explicit source cast did not constrain inference", widened, known)
			}
		})
	}
}

func constructorThrowsTestParameter(slot int, typ types.JavaType, name string) *values.JavaRef {
	id := coreutils.NewRootVariableId()
	id.SetName(name)
	ref := values.NewJavaRef(id, nil, typ)
	ref.IsParam = true
	ref.MarkOriginalParameter(slot)
	return ref
}
func constructorThrowsTestReceiver(owner string) *values.JavaRef {
	ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaClass(owner))
	ref.IsParam = true
	ref.IsThis = true
	ref.MarkOriginalParameter(0)
	ref.MarkOriginalReceiver()
	return ref
}

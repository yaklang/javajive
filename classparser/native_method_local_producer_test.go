package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// All positive witnesses come from a fresh original javac class and the real
// production decoder. Equal descriptor/return type is deliberately insufficient:
// the two category-2 producer arguments have different original slot identities.
func TestNativeMethodLocalProducerRequiresActualInvocationStoreAndPlacement(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class ProducedFactOwner {static long first(long a,long b){return a-b;}Object make(long a,long b){final long made=first(a,b);class Entry{long get(){return made;}}return new Entry();}}`, debug)
			variants := []string{"original", "renamed", "swapped wide arguments", "wrong owner", "wrong method", "wrong descriptor", "wrong invoke kind", "wrong static flag", "wrong producer PC", "unregistered producer PC", "replaced seed", "wrong STORE PC", "duplicate declaration", "nested declaration", "after allocation", "later write", "deep wrapper", "cyclic statement", "missing allocation", "budget", "canceled"}
			for _, variant := range variants {
				t.Run(variant, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["ProducedFactOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					obj, e := Parse(append([]byte(nil), files["ProducedFactOwner$1Entry.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					owner, ok := originalMethodLocalOwner(obj, root, nil)
					if !ok {
						t.Fatal("original enclosing method")
					}
					ctor, ok := originalMethodLocalDefaultConstructor(obj, root, nil)
					if !ok {
						t.Fatal("original default constructor")
					}
					d := NewClassObjectDumper(root)
					mt, e := types.ParseMethodDescriptor(owner.descriptor)
					if e != nil {
						t.Fatal(e)
					}
					d.MethodType = mt.FunctionType()
					d.CurrentMethod = owner.declaration
					d.FuncCtx = &class_context.ClassContext{ClassName: root.GetClassName(), FunctionName: owner.method, CurrentMethodDesc: owner.descriptor, FunctionType: mt.FunctionType()}
					d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
					var code *CodeAttribute
					for _, a := range owner.declaration.Attributes {
						if b, ok := a.(*CodeAttribute); ok {
							code = b
						}
					}
					if code == nil {
						t.Fatal("original code")
					}
					params, body, e := ParseBytesCode(d, code, coreutils.NewRootVariableId())
					if e != nil {
						t.Fatal(e)
					}
					slots := map[int]*values.JavaRef{}
					for _, v := range params {
						if r, ok := v.(*values.JavaRef); ok {
							if r.IsThis {
								slots[0] = r
							} else if s, ok := r.OriginalParameterSlot(); ok {
								slots[s] = r
							}
						}
					}
					sites, ok := d.nativeMethodLocalAllocationFacts(obj, owner, ctor, true)
					if !ok {
						t.Fatal("original typed SSA")
					}
					local := &nativeMethodLocalClass{object: obj, owner: owner, constructor: ctor, allocations: sites}
					var assign *statements.AssignStatement
					var call *values.FunctionCallExpression
					var ref *values.JavaRef
					for _, st := range body {
						if a, ok := st.(*statements.AssignStatement); ok {
							v, ok := nativeMemberEnclosingUnpack(a.JavaValue, nil)
							if !ok {
								continue
							}
							if c, ok := v.(*values.FunctionCallExpression); ok && c.FunctionName == "first" {
								assign = a
								call = c
								left, _ := nativeMemberEnclosingUnpack(a.LeftValue, nil)
								ref, _ = left.(*values.JavaRef)
							}
						}
					}
					if assign == nil || call == nil || ref == nil || len(body) < 2 || len(call.Arguments) != 2 {
						t.Fatalf("actual decoded producer/body: %+v", body)
					}
					switch variant {
					case "renamed":
						ref.Id = coreutils.NewRootVariableId() // Source binding itself still refers to this exact ref.
					case "swapped wide arguments":
						call.Arguments[0], call.Arguments[1] = call.Arguments[1], call.Arguments[0]
					case "wrong owner":
						call.ClassName += "Other"
					case "wrong method":
						call.FunctionName = "second"
					case "wrong descriptor":
						call.Descriptor = "(JJ)D"
					case "wrong invoke kind":
						call.Kind = values.InvokeVirtual
					case "wrong static flag":
						call.IsStatic = false
					case "wrong producer PC":
						call.OriginPC++
					case "unregistered producer PC":
						call.HasOriginPC = false
					case "replaced seed":
						ref.Val = values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong))
					case "wrong STORE PC":
						assign.OriginPC++
					case "duplicate declaration":
						body = append([]statements.Statement{assign}, body...)
					case "nested declaration":
						body[0] = &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{assign}}
					case "after allocation":
						body[0], body[len(body)-1] = body[len(body)-1], body[0]
					case "later write":
						body = append(body, &statements.AssignStatement{LeftValue: ref, JavaValue: values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong))})
					case "deep wrapper":
						for i := 0; i < 40; i++ {
							assign.LeftValue = values.NewSlotValue(assign.LeftValue, ref.Type())
						}
					case "cyclic statement":
						cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
						cycle.IfBody = []statements.Statement{cycle}
						body = append(body, cycle)
					case "missing allocation":
						body = []statements.Statement{assign}
					case "budget":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					bindings, anchor, ok := d.nativeMethodLocalProducerBindings(local, body, slots)
					want := variant == "original" || variant == "renamed"
					if ok != want {
						t.Fatalf("admitted=%v want=%v", ok, want)
					}
					if ok && (bindings["val$made"] != ref || anchor != body[1]) {
						t.Fatal("actual capture/declaration anchor lost")
					}
				})
			}
		})
	}
}

func TestNativeMethodLocalFinalEmissionCannotBorrowPreviewOrSiblingMethod(t *testing.T) {
	for _, variant := range []string{"original", "removed", "duplicated", "wrong method", "wrong descriptor", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj := &ClassObject{}
			owner := &nativeMethodLocalOwner{method: "make", descriptor: "()V"}
			local := &nativeMethodLocalClass{owner: owner, source: "class Entry { long get(){return 3;} }", allocations: map[int]nativeMethodLocalAllocation{}, calls: map[int]bool{}}
			d := NewClassObjectDumper(obj)
			d.nativeMemberRoot = &nativeMemberFamily{owner: obj.GetClassName(), methodLocals: map[string]*nativeMethodLocalClass{"entry": local}}
			body := local.source
			name, desc := "make", "()V"
			switch variant {
			case "removed":
				body = "return;"
			case "duplicated":
				body += body
			case "wrong method":
				name = "other"
			case "wrong descriptor":
				desc = "(J)V"
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if nativeMethodLocalSourceComplete(d.nativeMemberRoot) {
				t.Fatal("preview counted as final emission")
			}
			d.nativeMethodLocalMethodSourceComplete(name, desc, body)
			if got := nativeMethodLocalSourceComplete(d.nativeMemberRoot); got != (variant == "original") {
				t.Fatalf("closed=%v", got)
			}
		})
	}
}

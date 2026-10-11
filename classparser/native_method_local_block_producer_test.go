package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeMethodLocalBlockProducerRequiresDominatingScope(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class BlockFactOwner{Object make(java.util.Iterator<?> iterator){while(iterator.hasNext()){final Object made=iterator.next();if(made==null)continue;class Entry{Object get(){return made;}}return new Entry();}return null;}}`, "none")
	for _, variant := range []string{"original", "wrong STORE", "duplicate declaration", "producer sibling", "allocation outside", "duplicate allocation", "later write", "cycle", "depth", "memory", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, local, params, body := nativeMethodLocalProducerTestBody(t, files, "BlockFactOwner")
			var assign *statements.AssignStatement
			var block []statements.Statement
			index := -1
			var allocation statements.Statement
			var allocationBlock []statements.Statement
			allocationIndex := -1
			var find func([]statements.Statement)
			find = func(ss []statements.Statement) {
				for i, st := range ss {
					if a, ok := st.(*statements.AssignStatement); ok && a.HasOriginPC {
						if v, _ := nativeMemberEnclosingUnpack(a.JavaValue, nil); v != nil {
							if call, ok := v.(*values.FunctionCallExpression); ok && call.FunctionName == "next" {
								assign = a
								block = ss
								index = i
							}
						}
					}
					if ret, ok := st.(*statements.ReturnStatement); ok {
						if n, ok := ret.JavaValue.(*values.NewExpression); ok && n.ConstructorCall != nil && n.ConstructorCall.ClassName == local.object.GetClassName() {
							allocation = st
							allocationBlock = ss
							allocationIndex = i
						}
					}
					_, children, _ := nativeSourceNameChildren(st)
					for _, child := range children {
						find(child)
					}
				}
			}
			find(body)
			if assign == nil || allocation == nil || index < 0 || allocationIndex < 0 {
				t.Fatal("original produced capture and NEW")
			}
			left, _ := nativeMemberEnclosingUnpack(assign.LeftValue, nil)
			ref, ok := left.(*values.JavaRef)
			if !ok {
				t.Fatal("original declaration")
			}
			switch variant {
			case "wrong STORE":
				assign.OriginPC++
			case "duplicate declaration":
				body = append(body, assign)
			case "producer sibling":
				block[index] = &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{assign}}
			case "allocation outside":
				allocationBlock[allocationIndex] = &statements.CustomStatement{Name: "continue"}
				body = append(body, allocation)
			case "duplicate allocation":
				ret := allocation.(*statements.ReturnStatement)
				allocationBlock[allocationIndex] = &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: []statements.Statement{&statements.ReturnStatement{JavaValue: ret.JavaValue}}, ElseBody: []statements.Statement{&statements.ReturnStatement{JavaValue: ret.JavaValue}}}
			case "later write":
				body = append(body, &statements.AssignStatement{LeftValue: ref, JavaValue: values.NewJavaLiteral(nil, types.NewJavaClass("java.lang.Object"))})
			case "cycle":
				cycle := &statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))}
				cycle.IfBody = []statements.Statement{cycle}
				body = append(body, cycle)
			case "depth":
				for i := 0; i < 65; i++ {
					body = []statements.Statement{&statements.IfStatement{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), IfBody: body}}
				}
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got, anchor, known := d.nativeMethodLocalProducerBindings(local, body, params)
			if known != (variant == "original") {
				t.Fatalf("admitted=%v", known)
			}
			if known && (got["val$made"] != ref || anchor != block[index+1]) {
				t.Fatal("capture identity or immediate lexical anchor changed")
			}
		})
	}
}

func TestNativeMethodLocalProducerFieldReceiverRequiresSealedDefUse(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class FieldFactOwner{java.util.Iterator<?> iterator;Object make(){final Object made=iterator.next();class Entry{Object get(){return made;}}return new Entry();}}`, "none")
	for _, variant := range []string{"original", "changed field PC", "missing field PC", "changed field name", "changed field erasure", "different receiver", "receiver seed changed", "cyclic receiver", "wrong producer", "memory", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, local, params, body := nativeMethodLocalProducerTestBody(t, files, "FieldFactOwner")
			var field *values.RefMember
			for _, st := range body {
				if a, ok := st.(*statements.AssignStatement); ok {
					v, _ := nativeMemberEnclosingUnpack(a.JavaValue, nil)
					if call, ok := v.(*values.FunctionCallExpression); ok && call.FunctionName == "next" {
						receiver, _ := nativeMemberEnclosingUnpack(call.Object, nil)
						field, _ = receiver.(*values.RefMember)
						if variant == "wrong producer" {
							call.OriginPC++
						}
					}
				}
			}
			if field == nil {
				t.Fatal("original field receiver")
			}
			switch variant {
			case "changed field PC":
				field.OriginPC++
			case "missing field PC":
				field.HasOriginPC = false
			case "changed field name":
				field.Member = "other"
			case "changed field erasure":
				field.JavaType = types.NewJavaClass("java.lang.Object")
			case "different receiver":
				field.Object = values.NewJavaRef(nil, values.JavaNull, field.Object.Type())
			case "receiver seed changed":
				receiver, _ := nativeMemberEnclosingUnpack(field.Object, nil)
				ref, _ := receiver.(*values.JavaRef)
				if ref == nil {
					t.Fatal("THIS receiver")
				}
				ref.Val = values.NewJavaLiteral("changed-seed", ref.Type())
			case "cyclic receiver":
				field.Object = field
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, _, known := d.nativeMethodLocalProducerBindings(local, body, params)
			if known != (variant == "original") {
				t.Fatalf("admitted=%v", known)
			}
		})
	}
}

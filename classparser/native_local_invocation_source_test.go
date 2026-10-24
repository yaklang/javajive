package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeLocalInvocationRequiresOriginalStoreWebAndReceiver(t *testing.T) {
	for _, static := range []bool{false, true} {
		kind := "instance"
		getter := "java.util.Comparator<N> ordering(){return null;}"
		if static {
			kind, getter = "static", "static java.util.Comparator<String> ordering(){return null;}"
		}
		t.Run(kind, func(t *testing.T) {
			fixture := "class LocalProducerOwner<N>{" + getter + "Object make(){final java.util.Comparator captured=ordering();class Entry{Object get(){return captured;}}return new Entry();}}"
			files := nativeCompileDebugClasses(t, fixture, "none")
			for _, variant := range []string{"original", "renamed", "source declaration clone", "missing STORE", "wrong STORE PC", "replaced seed", "same ID foreign copy", "changed UID", "parameter", "later write", "duplicate declaration", "wrong scope", "wrong descriptor", "duplicate method", "missing Code", "duplicate Code", "wrong producer PC", "wrong producer owner", "wrong producer method", "wrong producer descriptor", "wrong invoke kind", "wrong static flag", "extra argument", "foreign receiver", "unsealed this", "missing original parameter", "foreign class Signature", "foreign method Signature", "original shadowing method", "work", "memory", "canceled"} {
				t.Run(variant, func(t *testing.T) {
					d, local, slots, body := nativeMethodLocalProducerTestBody(t, files, "LocalProducerOwner")
					d.FuncCtx.ClassSig, _ = nativeMethodLocalOriginalSignature(d.obj, d.obj.Attributes, nil)
					d.FuncCtx.ClassTypeParams, d.FuncCtx.TypeParams = []string{"N"}, []string{"N"}
					method := local.owner.declaration
					var code *CodeAttribute
					for _, attr := range method.Attributes {
						if v, ok := attr.(*CodeAttribute); ok {
							code = v
						}
					}
					var declaration *statements.AssignStatement
					var ref *values.JavaRef
					var call *values.FunctionCallExpression
					for _, st := range body {
						if a, ok := st.(*statements.AssignStatement); ok {
							producer, _ := nativeMemberEnclosingUnpack(a.JavaValue, nil)
							if f, ok := producer.(*values.FunctionCallExpression); ok && f.FunctionName == "ordering" {
								left, _ := nativeMemberEnclosingUnpack(a.LeftValue, nil)
								ref, _ = left.(*values.JavaRef)
								declaration, call = a, f
							}
						}
					}
					if ref == nil || declaration == nil || call == nil || code == nil {
						t.Fatal("missing actual decoded local invocation")
					}
					params := []values.JavaValue{}
					for _, p := range slots {
						params = append(params, p)
					}
					switch variant {
					case "renamed":
						ref.Id.SetName("renamedProducer")
					case "source declaration clone":
						ref = ref.CloneForSourceDeclaration()
					case "missing STORE":
						declaration.HasOriginPC = false
					case "wrong STORE PC":
						declaration.OriginPC++
					case "replaced seed":
						ref.Val = values.JavaNull
					case "same ID foreign copy":
						copy := *ref
						ref = &copy
					case "changed UID":
						copy := *ref
						copy.VarUid = "foreign"
						ref = &copy
					case "parameter":
						ref.IsParam = true
					case "later write":
						body = append(body, statements.NewAssignStatement(ref, values.JavaNull, false))
					case "duplicate declaration":
						body = append(body, declaration)
					case "wrong scope":
						d.FuncCtx.FunctionName = "other"
					case "wrong descriptor":
						d.FuncCtx.CurrentMethodDesc = "(J)Ljava/lang/Object;"
					case "duplicate method":
						d.obj.Methods = append(d.obj.Methods, method)
					case "missing Code":
						method.Attributes = nil
					case "duplicate Code":
						method.Attributes = append(method.Attributes, code)
					case "wrong producer PC":
						call.OriginPC++
					case "wrong producer owner":
						call.ClassName += "Foreign"
					case "wrong producer method":
						call.FunctionName = "other"
					case "wrong producer descriptor":
						call.Descriptor = "()Ljava/util/List;"
					case "wrong invoke kind":
						call.Kind = values.InvokeSpecial
					case "wrong static flag":
						call.IsStatic = !call.IsStatic
					case "extra argument":
						call.Arguments = []values.JavaValue{values.JavaNull}
					case "foreign receiver":
						call.Object = values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("LocalProducerOwner"))
					case "unsealed this":
						r := values.NewJavaRef(utils.NewRootVariableId(), nil, types.NewJavaClass("LocalProducerOwner"))
						r.IsParam, r.IsThis = true, true
						call.Object = r
					case "missing original parameter":
						params = nil
					case "foreign class Signature":
						d.FuncCtx.ClassSig = "<N:Ljava/io/Serializable;>Ljava/lang/Object;"
					case "foreign method Signature":
						d.FuncCtx.CurrentMethodSig = "<N:Ljava/lang/Object;>()Ljava/lang/Object;"
					case "original shadowing method":
						sig := "<N:Ljava/lang/Object;>()Ljava/lang/Object;"
						method.Attributes = append(method.Attributes, &SignatureAttribute{SignatureIndex: sourceBridgePoolString(t, d.obj, sig)})
						d.FuncCtx.CurrentMethodSig = sig
					case "work":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "memory":
						d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						d.Work = workbudget.New(ctx, workbudget.Limits{})
					}
					object, originalType := call.Object, ref.Type()
					proof, known := d.nativeSourceLocalInvocation(d.FuncCtx, method, code, body, params, ref)
					positive := variant == "original" || variant == "renamed" || variant == "source declaration clone" || static && (variant == "foreign receiver" || variant == "unsealed this" || variant == "missing original parameter" || variant == "foreign class Signature" || variant == "foreign method Signature" || variant == "original shadowing method")
					if known != positive || known && (proof == nil || proof == call || proof.FunctionName != call.FunctionName || proof.OriginPC != call.OriginPC) {
						t.Fatalf("%s original certificate=%v, want=%v", variant, known, positive)
					}
					if call.Object != object || ref.Type() != originalType || d.nativeCaptureFailed {
						t.Fatal("read-only producer query mutated original value or family state")
					}
					if positive && !static && proof.Object.Type().String(d.FuncCtx) != "LocalProducerOwner<N>" {
						t.Fatal("original receiver class binding lost")
					}
				})
			}
		})
	}
}

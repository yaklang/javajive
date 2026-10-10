package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

// AST refusal models complement the JVM banks; they are not additional
// executable Java programs. The child's ownership and PCs come from an actual
// original classfile, never a class-name pattern.
func TestNativeAnonymousOwnerInitializerRejectsScopeAndAllocationCounterexamples(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ScopeOwner.java": `class ScopeOwner{Object value=new Object(){};ScopeOwner(){}}`}, "none", "8")
	for _, variant := range []string{"original", "no plan", "ordinary method", "missing allocation", "second constructor", "wrong new PC", "wrong call PC", "no new origin", "no call origin", "wrong descriptor", "wrong receiver", "ordinary invocation", "wrong invocation name", "wrong arity", "parameter", "local", "opaque", "cyclic value", "control flow", "foreign delegation", "budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			obj, err := Parse(files["ScopeOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := archive.nativeMemberReader(obj)
			plan := d.planNativeAnonymousFamily()
			if plan == nil || plan.children["ScopeOwner$1"] == nil {
				t.Fatal("original initializer role")
			}
			child := plan.children["ScopeOwner$1"]
			d.nativeAnonymousRoot = plan
			d.FuncCtx = &class_context.ClassContext{FunctionName: "<init>"}
			this := &values.JavaRef{IsThis: true}
			allocation := values.NewNewExpression(types.NewJavaClass("ScopeOwner$1"))
			allocation.OriginPC, allocation.HasOriginPC = child.newPC, true
			call := &values.FunctionCallExpression{ClassName: "ScopeOwner$1", FunctionName: "<init>", Descriptor: child.descriptor, Object: allocation, OriginPC: child.invokePC, HasOriginPC: true, Kind: values.InvokeSpecial, IsSpecialInvoke: true, Arguments: []values.JavaValue{this}}
			allocation.ConstructorCall = call
			assignment := &statements.AssignStatement{LeftValue: values.NewRefMember(this, "value", types.NewJavaClass("java.lang.Object")), JavaValue: allocation}
			body := []statements.Statement{assignment}
			switch variant {
			case "no plan":
				d.nativeAnonymousRoot = nil
			case "ordinary method":
				d.FuncCtx.FunctionName = "make"
			case "missing allocation":
				assignment.JavaValue = values.JavaNull
			case "second constructor":
				obj.Methods = append(obj.Methods, obj.Methods[0])
			case "wrong new PC":
				allocation.OriginPC++
			case "wrong call PC":
				call.OriginPC++
			case "no new origin":
				allocation.HasOriginPC = false
			case "no call origin":
				call.HasOriginPC = false
			case "wrong descriptor":
				call.Descriptor = "()V"
			case "wrong receiver":
				call.Object = this
			case "ordinary invocation":
				call.Kind, call.IsSpecialInvoke = values.InvokeVirtual, false
			case "wrong invocation name":
				call.FunctionName = "make"
			case "wrong arity":
				call.Arguments = nil
			case "parameter":
				call.Arguments[0] = &values.JavaRef{IsParam: true}
			case "local":
				call.Arguments[0] = &values.JavaRef{}
			case "opaque":
				call.Arguments[0] = &values.CustomValue{}
			case "cyclic value":
				expr := &values.JavaExpression{}
				expr.Values = []values.JavaValue{expr}
				call.Arguments[0] = expr
			case "control flow":
				body = append([]statements.Statement{&statements.ReturnStatement{}}, body...)
			case "foreign delegation":
				body = append([]statements.Statement{&statements.ExpressionStatement{Expression: &values.FunctionCallExpression{ClassName: "Foreign", FunctionName: "<init>", IsSpecialInvoke: true}}}, body...)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			count, err := d.nativeAnonymousOwnerInitializerPrefix(body)
			if variant == "no plan" || variant == "ordinary method" {
				if count != 0 || err != nil {
					t.Fatal("unrelated scope changed")
				}
				return
			}
			if variant == "original" {
				if err != nil || count != 1 {
					t.Fatal("original prefix refused", count, err)
				}
			} else if err == nil || count != 0 || !plan.failed {
				t.Fatal("invalid scope accepted", count, err)
			}
		})
	}
}

func TestAdversarialAnonymousInitializerKeepsEmptyConstructorAndFieldOrder(t *testing.T) {
	fixture := `public class EmptyInitOwner{final int before=EmptyInitEffects.touch(17);final Object value=new Object(){public String toString(){return "original";}};final int after=EmptyInitEffects.touch(31);}
class EmptyInitEffects{static String trace="";static int touch(int n){trace+=n+":";return n;}}
class EmptyInitDriver{public static void main(String[]args){EmptyInitOwner root=new EmptyInitOwner();if(root.before!=17||root.after!=31||!EmptyInitEffects.trace.equals("17:31:")||!root.value.toString().equals("original")||root.value.getClass().getEnclosingConstructor()!=null||root.value.getClass().getEnclosingMethod()!=null||!root.value.getClass().isAnonymousClass())throw new AssertionError("initializer order/owner/body loss");System.out.println("initializer:empty-constructor:ordered-fields");}}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"EmptyInitOwner.java": fixture}, "EmptyInitOwner", "EmptyInitDriver", "initializer:empty-constructor:ordered-fields\n")
}

func TestNativeAnonymousInitializerContextNeedsOriginalUniqueAllocation(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"ContextOwner.java": `class ContextOwner{static final Object shared=new Object(){};final Object own=new Object(){};}`}, "none", "8")
	for _, childName := range []string{"ContextOwner$1", "ContextOwner$2"} {
		for _, variant := range []string{"original", "wrong lexical method", "wrong owner", "missing bodies", "duplicate bodies", "wrong staticness", "missing child constructor", "duplicate child constructor", "wrong descriptor", "malformed method", "budget", "cancelled"} {
			t.Run(childName+"/"+variant, func(t *testing.T) {
				owner, err := Parse(files["ContextOwner.class"])
				if err != nil {
					t.Fatal(err)
				}
				child, err := Parse(files[childName+".class"])
				if err != nil {
					t.Fatal(err)
				}
				lexical := ""
				var work *workbudget.Budget
				switch variant {
				case "wrong lexical method":
					lexical = "make()Ljava/lang/Object;"
				case "wrong owner":
					owner = child
				case "missing bodies":
					for _, method := range owner.Methods {
						method.Attributes = nil
					}
				case "duplicate bodies":
					owner.Methods = append(owner.Methods, owner.Methods...)
				case "wrong staticness":
					for _, method := range owner.Methods {
						method.AccessFlags ^= StaticFlag
					}
				case "missing child constructor", "duplicate child constructor", "wrong descriptor":
					for i, method := range child.Methods {
						if name, _ := sourceBridgeUTF8(child, method.NameIndex); name == "<init>" {
							if variant == "missing child constructor" {
								child.Methods = append(child.Methods[:i], child.Methods[i+1:]...)
							} else if variant == "duplicate child constructor" {
								child.Methods = append(child.Methods, method)
							} else {
								method.DescriptorIndex = method.NameIndex
							}
							break
						}
					}
				case "malformed method":
					owner.Methods[0] = nil
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "cancelled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				static, known := nativeAnonymousOriginalContext(owner, child, lexical, work)
				if known != (variant == "original") || known && static != (childName == "ContextOwner$1") {
					t.Fatal("allocation evidence replaced by metadata/name guess", static, known)
				}
			})
		}
	}
}

func TestAdversarialAnonymousInitializerPreservesDeclaredCheckedConstructionFailure(t *testing.T) {
	fixture := `public class CheckedInitOwner{final CheckedInitParent value=new CheckedInitParent(){long get(){return word^0xCAFEBABEL;}};public CheckedInitOwner()throws java.io.IOException{CheckedInitEffects.trace+="C";}}
 class CheckedInitEffects{static String trace="";static long input;static boolean fail;static final java.io.IOException error=new java.io.IOException("same");}
 class CheckedInitParent{final long word;CheckedInitParent()throws java.io.IOException{CheckedInitEffects.trace+="P";word=CheckedInitEffects.input;if(CheckedInitEffects.fail)throw CheckedInitEffects.error;}long get(){return word;}}
 class CheckedInitDriver{public static void main(String[]args)throws Exception{int rows=0;for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){CheckedInitEffects.input=n;CheckedInitEffects.fail=fail;CheckedInitEffects.trace="";try{CheckedInitOwner root=new CheckedInitOwner();if(fail||root.value.get()!=(n^0xCAFEBABEL)||!CheckedInitEffects.trace.equals("PC"))throw new AssertionError("checked success/order");if(root.value.getClass().getEnclosingConstructor()!=null||root.value.getClass().getEnclosingMethod()!=null)throw new AssertionError("initializer ownership");}catch(java.io.IOException e){if(!fail||e!=CheckedInitEffects.error||!CheckedInitEffects.trace.equals("P"))throw new AssertionError("checked failure identity/order",e);}rows++;}System.out.println(rows+":checked:initializer:identity:order");}}`
	testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{"CheckedInitOwner.java": fixture}, debug, "8")
	}, []string{"CheckedInitOwner"}, "CheckedInitDriver", "8:checked:initializer:identity:order\n", nil, nativeLexicalExactSignatures)
}

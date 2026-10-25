package javaclassparser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const protectedParentVisibilityBridgeFixture = `
class VisibilityEffects {static int calls;static boolean fail;static final java.io.IOException error=new java.io.IOException("identity");}
class VisibilityBase {
 protected static class Parent {
 public Object read(long n,double d,Object token)throws java.io.IOException {VisibilityEffects.calls++;if(n!=Long.MIN_VALUE||Double.doubleToRawLongBits(d)!=Double.doubleToRawLongBits(-0.0))throw new AssertionError("slots");if(VisibilityEffects.fail)throw VisibilityEffects.error;return token;}}}
public class VisibilityOwner extends VisibilityBase {
 protected static class Child extends Parent {public Child(){}}
 static Child make(){return new Child();}}
class BridgeDriver {public static void main(String[]args)throws Exception {
 if(VisibilityOwner.Child.class.getDeclaredMethods().length!=0)throw new AssertionError("original child must inherit, not redeclare");
 if(VisibilityBase.Parent.class.getModifiers()!=12)throw new AssertionError("original protected static parent scope");
 int rows=0;VisibilityOwner.Child child=VisibilityOwner.make();
 for(Object token:new Object[]{null,new Object(),"value"})for(boolean fail:new boolean[]{false,true}){
 VisibilityEffects.calls=0;VisibilityEffects.fail=fail;boolean caught=false;
 try{if(child.read(Long.MIN_VALUE,-0.0,token)!=token)throw new AssertionError("identity");}catch(java.io.IOException error){if(error!=VisibilityEffects.error)throw error;caught=true;}
 if(caught!=fail||VisibilityEffects.calls!=1)throw new AssertionError("dispatch/exception");rows++;
 }System.out.println(rows+":protected:parent:visibility:declaration:identity");}}
`

func TestAdversarialProtectedMemberParentKeepsInheritedMethodUniverse(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, root := range []string{"VisibilityOwner", "RenamedVisibilityScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, member := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/member%v", root, debug, member), func(t *testing.T) {
					fixture := strings.ReplaceAll(protectedParentVisibilityBridgeFixture, "VisibilityOwner", root)
					if member {
						fixture = strings.ReplaceAll(fixture, "protected static class", "protected class")
						fixture = strings.ReplaceAll(fixture, "static Child make()", "Child make()")
						fixture = strings.ReplaceAll(fixture, root+".make()", "new "+root+"().make()")
						fixture = strings.ReplaceAll(fixture, "getModifiers()!=12", "getModifiers()!=4")
						// A flat nonstatic unit must not be enrolled twice by the
						// compiler-only dependency row. This helper removes only
						// that row during javac and restores the unchanged original
						// dependencies before the executable oracle.
						fixture = strings.ReplaceAll(fixture, "public class "+root, "class "+root)
						fixture = strings.ReplaceAll(fixture, "BridgeDriver", "SuffixDriver")
						constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{root + "$Child"}, "6:protected:parent:visibility:declaration:identity\n")
						return
					}
					testIndependentInheritedBridgeOwner(t, modern, java, native, fixture, root, root+"$Child", debug, true, "6:protected:parent:visibility:declaration:identity\n", nil)
				})
			}
		}
	}
}

func TestAdversarialProtectedParentVisibilityRequiresReciprocalOriginalScope(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, protectedParentVisibilityBridgeFixture, "VisibilityOwner", "none", false)
	for _, variant := range []string{"control", "missing child row", "duplicate child row", "missing owner row", "duplicate owner row", "mismatched visibility", "mismatched simple name", "mismatched outer name", "public member", "private member", "conflicting access", "unknown flags", "kind mismatch", "missing owner", "foreign owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			child := mustParseLowering(t, files["VisibilityOwner$Child.class"])
			parent := mustParseLowering(t, files["VisibilityBase$Parent.class"])
			outer := mustParseLowering(t, files["VisibilityBase.class"])
			row := func(obj *ClassObject) (*InnerClassesAttribute, *InnerClassInfo) {
				for _, attr := range obj.Attributes {
					if table, ok := attr.(*InnerClassesAttribute); ok {
						for _, r := range table.Classes {
							if name, ok := sourceBridgeClassName(obj, r.InnerClassInfoIndex); ok && name == parent.GetClassName() {
								return table, r
							}
						}
					}
				}
				t.Fatal("missing original reciprocal parent declaration")
				return nil, nil
			}
			pt, pr := row(parent)
			ot, or := row(outer)
			d := NewClassObjectDumper(child)
			switch variant {
			case "missing child row":
				pt.Classes = nil
			case "duplicate child row":
				pt.Classes = append(pt.Classes, pr)
			case "missing owner row":
				ot.Classes = nil
			case "duplicate owner row":
				ot.Classes = append(ot.Classes, or)
			case "mismatched visibility":
				or.InnerClassAccessFlags = 9
			case "mismatched simple name":
				or.InnerNameIndex = uint16(outer.ConstantPoolManager.AddUtf8Info("Other"))
			case "mismatched outer name":
				or.OuterClassInfoIndex = uint16(outer.ConstantPoolManager.AddNewClassInfo("DifferentOuter"))
			case "public member":
				pr.InnerClassAccessFlags, or.InnerClassAccessFlags = 9, 9
			case "private member":
				pr.InnerClassAccessFlags, or.InnerClassAccessFlags = 10, 10
			case "conflicting access":
				pr.InnerClassAccessFlags |= 1
			case "unknown flags":
				pr.InnerClassAccessFlags |= 0x80
			case "kind mismatch":
				pr.InnerClassAccessFlags |= 0x400
			case "foreign owner":
				outer.ThisClass = uint16(outer.ConstantPoolManager.AddNewClassInfo("Foreign"))
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			for _, table := range []*InnerClassesAttribute{pt, ot} {
				table.NumberOfClasses = uint16(len(table.Classes))
				table.AttrLen = uint32(2 + len(table.Classes)*8)
			}
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "VisibilityBase$Parent" {
					return parent.Bytes(), true
				}
				if name == "VisibilityBase" && variant != "missing owner" {
					return outer.Bytes(), true
				}
				return nil, false
			}
			if accepted := d.inheritedVisibilityDomainParent() != nil; accepted != (variant == "control") {
				t.Fatalf("original lexical visibility accepted=%v", accepted)
			}
		})
	}
}

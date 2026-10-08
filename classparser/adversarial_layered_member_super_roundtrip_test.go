package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

// Two separate declarations supply inherited member constructors. The two
// anonymous source scopes add captures that the parent observes before its
// constructor returns (or throws). Inheritance must not invent lexical owners.
const layeredMemberSuperFixture = `class ConstructionEffects{static Object seen;static boolean fail;static final RuntimeException failure=new RuntimeException("original");}
abstract class OriginNamespace<T>{final T value;OriginNamespace(T value){this.value=value;}
 abstract class Leaf{final Object token;Leaf(Object token){this.token=token;ConstructionEffects.seen=origin();if(ConstructionEffects.fail)throw ConstructionEffects.failure;}abstract Object origin();Object outer(){return OriginNamespace.this;}}
 abstract Leaf make(Object token);
}
abstract class TypedNamespace<T> extends OriginNamespace<T>{TypedNamespace(T value){super(value);}
 abstract class TypedLeaf extends Leaf{TypedLeaf(Object token){super(token);}}
}
class LayeredFactory{OriginNamespace<Object> create(Object value){return new TypedNamespace<Object>(value){Leaf make(Object token){return new TypedLeaf(token){Object origin(){return LayeredFactory.this;}};}};}}
class LayeredSuperDriver{public static void main(String[]args){Object marker=new Object();int rows=0;
 for(Object value:new Object[]{null,marker,"value"}){LayeredFactory factory=new LayeredFactory();OriginNamespace<Object> namespace=factory.create(value);
  for(Object token:new Object[]{null,marker,"token"})for(boolean fail:new boolean[]{false,true}){ConstructionEffects.fail=fail;ConstructionEffects.seen=null;
   try{OriginNamespace<Object>.Leaf leaf=namespace.make(token);if(fail||leaf.token!=token||leaf.outer()!=namespace||leaf.origin()!=factory||namespace.value!=value)throw new AssertionError("layered member values");
    if(leaf.getClass().getEnclosingClass()!=namespace.getClass()||!leaf.getClass().isAnonymousClass()||leaf.getClass().getSuperclass()!=TypedNamespace.TypedLeaf.class||TypedNamespace.TypedLeaf.class.getDeclaringClass()!=TypedNamespace.class||OriginNamespace.Leaf.class.getDeclaringClass()!=OriginNamespace.class)throw new AssertionError("layered declaration identity");
   }catch(RuntimeException ex){if(!fail||ex!=ConstructionEffects.failure)throw new AssertionError("layered exception identity",ex);}
   if(ConstructionEffects.seen!=factory)throw new AssertionError("layered parent observed capture");rows++;
  }
 }
 System.out.println(rows+":layered:super:enclosing:effects");}}
`

func TestAdversarialLayeredMemberSuperPreservesOriginalEnclosingAndEarlyObservation(t *testing.T) {
	testNativeIndependentFamilyFixture(t, layeredMemberSuperFixture, []string{"OriginNamespace", "TypedNamespace", "LayeredFactory", "ConstructionEffects"}, "LayeredSuperDriver", "18:layered:super:enclosing:effects\n", nativeLexicalExactSignatures)
}

func TestAdversarialLayeredMemberSuperPreservesRenamedDeclarations(t *testing.T) {
	f := strings.NewReplacer("OriginNamespace", "BaseOwner", "TypedNamespace", "SpecializedOwner", "LayeredFactory", "UsingRoot", "TypedLeaf", "DerivedItem", "Leaf", "Item", "token", "argument").Replace(layeredMemberSuperFixture)
	testNativeIndependentFamilyFixture(t, f, []string{"BaseOwner", "SpecializedOwner", "UsingRoot", "ConstructionEffects"}, "LayeredSuperDriver", "18:layered:super:enclosing:effects\n", nativeLexicalExactSignatures)
}

func TestAdversarialLayeredMemberSuperIsIndependentOfUnrelatedNamedDeclarations(t *testing.T) {
	f := strings.Replace(layeredMemberSuperFixture, "class LayeredFactory{", "class LayeredFactory{static class Unrelated{}", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"OriginNamespace", "TypedNamespace", "LayeredFactory", "ConstructionEffects"}, "LayeredSuperDriver", "18:layered:super:enclosing:effects\n", nativeLexicalExactSignatures)
}

func TestNativeAnonymousEmptyRootCycleKeepsAllSourceReadsConsistent(t *testing.T) {
	files := nativeCompileClasses(t, layeredMemberSuperFixture)
	for _, first := range []string{"LayeredFactory", "LayeredFactory$1", "LayeredFactory$1$1", "TypedNamespace", "OriginNamespace"} {
		t.Run(first, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			if _, err := archive.ReadFile(first + ".class"); err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 2; round++ {
				root, _ := Parse(files["LayeredFactory.class"])
				owner, known := archive.nativeMemberTransactionOwner(root)
				if !known || owner != root.GetClassName() {
					t.Fatal("original anonymous source root cannot join its declaration cycle")
				}
				entry := archive.nativeMemberEntry(root)
				if entry == nil || entry.family == nil {
					entry = archive.nativeMemberTransactionEntry(root)
				}
				if entry == nil || entry.family == nil || entry.family.owner != owner || len(entry.family.anonymousUnits) != 2 {
					t.Fatal("both original anonymous scopes must commit with their root")
				}
				for _, name := range []string{"LayeredFactory", "LayeredFactory$1", "LayeredFactory$1$1"} {
					source, err := archive.ReadFile(name + ".class")
					if err != nil || strings.Contains(string(source), DecompileStubMarker) {
						t.Fatal("incomplete original source family", name, err, string(source))
					}
					if name == owner {
						if !strings.Contains(string(source), "new TypedNamespace") || strings.Contains(string(source), "new LayeredFactory$1(") {
							t.Fatal("root cannot stay flattened while suppressing its children", string(source))
						}
					} else if !strings.Contains(string(source), "body owned by") {
						t.Fatal("anonymous child must use the same committed source root", name, string(source))
					}
				}
			}
		})
	}
}

func TestNativeAnonymousEmptyRootDiscoveryRequiresOriginalContext(t *testing.T) {
	files := nativeCompileClasses(t, layeredMemberSuperFixture)
	for _, variant := range []string{"original", "nil resolver", "missing class", "different class", "missing enclosing method", "unknown enclosing method", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["LayeredFactory.class"])
			reader := archive.nativeMemberReader(root)
			switch variant {
			case "nil resolver":
				reader.foldSiblingResolver = nil
			case "missing class":
				reader.foldSiblingResolver = func(string) ([]byte, bool) { return nil, false }
			case "different class", "missing enclosing method", "unknown enclosing method":
				child, _ := Parse(files["LayeredFactory$1.class"])
				if variant == "different class" {
					child, _ = Parse(files["OriginNamespace.class"])
				} else {
					for i, attribute := range child.Attributes {
						if enclosing, ok := attribute.(*UnparsedAttribute); ok && enclosing.Name == "EnclosingMethod" {
							if variant == "missing enclosing method" {
								child.Attributes = append(child.Attributes[:i], child.Attributes[i+1:]...)
							} else {
								index := int(enclosing.Info[2])<<8 | int(enclosing.Info[3])
								nameType := child.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
								nameType.NameIndex = sourceBridgePoolString(t, child, "unknownOriginalMethod")
							}
							break
						}
					}
				}
				reader.foldSiblingResolver = func(string) ([]byte, bool) { return child.Bytes(), true }
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				reader.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if admitted := reader.nativeMemberHasAnonymousDeclarations(); admitted != (variant == "original") {
				t.Fatalf("original anonymous discovery admitted=%v", admitted)
			}
		})
	}
}

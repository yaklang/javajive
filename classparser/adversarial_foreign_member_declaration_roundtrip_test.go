package javaclassparser

import (
	"strings"
	"testing"
)

// Unrelated source families exchange a value with its own enclosing identity.
// Restoring the declared type must not import that family's private scope or
// replace its nullable value with the caller's lexical enclosing instance.
func foreignMemberDeclarationFixture() string {
	f := strings.Replace(nativeInheritedDependencyFixture, "class DependencyDerived extends DependencyBase", "class DependencyDerived", 1)
	f = strings.Replace(f, "final Value value;DependencyDerived(Value value)", "final DependencyBase.Value value;DependencyDerived(DependencyBase.Value value)", 1)
	return strings.Replace(f, "Object read(Value value)", "Object read(DependencyBase.Value value)", 1)
}

func TestAdversarialForeignMemberDeclarationKeepsSeparateEnclosingIdentity(t *testing.T) {
	testNativeIndependentFamilyFixture(t, foreignMemberDeclarationFixture(), []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsGenericOwnerBinding(t *testing.T) {
	f := foreignMemberDeclarationFixture()
	f = strings.Replace(f, "class DependencyBase{", "class DependencyBase<T>{", 1)
	f = strings.Replace(f, "final Object token;Value(Object token)", "final T token;Value(T token)", 1)
	f = strings.Replace(f, "Value make(Object token)", "Value make(T token)", 1)
	f = strings.Replace(f, "class DependencyDerived{", "class DependencyDerived<T>{", 1)
	head, driver, found := strings.Cut(f, "class InheritedDependencyDriver")
	if !found {
		t.Fatal("independent driver boundary")
	}
	f = strings.ReplaceAll(head, "DependencyBase.Value value", "DependencyBase<T>.Value value") + "class InheritedDependencyDriver" + driver
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsPrivateConstructorOwnership(t *testing.T) {
	f := strings.Replace(foreignMemberDeclarationFixture(), "Value(Object token)", "private Value(Object token)", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsOriginalNameAgainstLocalDecoy(t *testing.T) {
	f := strings.Replace(foreignMemberDeclarationFixture(), "class DependencyDerived{", "class DependencyDerived{static class Value{final Object token=new Object();}", 1)
	f = strings.Replace(f, "return value.token;", "Value decoy=new Value();if(decoy.token==value.token)throw new AssertionError(\"wrong local binding\");return value.token;", 1)
	f = "package independent.binding;\n" + f
	testNativeIndependentFamilyFixture(t, f, []string{"independent/binding/DependencyBase", "independent/binding/DependencyDerived"}, "independent.binding.InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsDistinctMemberFormals(t *testing.T) {
	f := foreignMemberDeclarationFixture()
	f = strings.Replace(f, "class DependencyBase{class Value{", "class DependencyBase<T>{class Value<U>{", 1)
	f = strings.Replace(f, "final Object token;Value(Object token)", "final U token;Value(U token)", 1)
	f = strings.Replace(f, "Value make(Object token){return new Value(token);}", "Value<T> make(T token){return new Value<T>(token);}", 1)
	f = strings.Replace(f, "class DependencyDerived{", "class DependencyDerived<T>{", 1)
	head, driver, found := strings.Cut(f, "class InheritedDependencyDriver")
	if !found {
		t.Fatal("independent driver boundary")
	}
	f = strings.ReplaceAll(head, "DependencyBase.Value value", "DependencyBase<T>.Value<T> value") + "class InheritedDependencyDriver" + driver
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyBase", "DependencyDerived"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationIsIndependentOfSpelling(t *testing.T) {
	f := strings.ReplaceAll(foreignMemberDeclarationFixture(), "DependencyBase", "OriginalNamespace")
	f = strings.ReplaceAll(f, "DependencyDerived", "UnrelatedNamespace")
	f = strings.ReplaceAll(f, "Value", "Element")
	testNativeIndependentFamilyFixture(t, f, []string{"OriginalNamespace", "UnrelatedNamespace"}, "InheritedDependencyDriver", "2:ancestor:dependency:separate:outer\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsNestedGenericOwnerPath(t *testing.T) {
	const fixture = `class DeclaredNamespace<T>{
 final T seed;DeclaredNamespace(T seed){this.seed=seed;}
 class Context<V>{final V view;Context(V view){this.view=view;}
  class Entry<U>{final U token;Entry(U token){this.token=token;}U get(){return token;}Object owner(){return DeclaredNamespace.this;}Object context(){return Context.this;}}
 }
}

class IndependentConsumer<T>{class Local{
 <U> U value(DeclaredNamespace<T>.Context<String>.Entry<U> entry){return entry.get();}
 <U> DeclaredNamespace<T>.Context<String>.Entry<U> identity(DeclaredNamespace<T>.Context<String>.Entry<U> entry){return entry;}
}}
class NestedNamespaceDriver{public static void main(String[]args){Object token=new Object();int rows=0;
 for(Object seed:new Object[]{null,token,"owner"}){DeclaredNamespace<Object> owner=new DeclaredNamespace<>(seed);DeclaredNamespace<Object>.Context<String> context=owner.new Context<>("context");IndependentConsumer<Object>.Local consumer=new IndependentConsumer<Object>().new Local();
  for(Object value:new Object[]{null,token,"value"}){DeclaredNamespace<Object>.Context<String>.Entry<Object> entry=context.new Entry<>(value);
   if(consumer.identity(entry)!=entry||consumer.value(entry)!=value||entry.owner()!=owner||entry.context()!=context||entry.getClass().getDeclaringClass()!=DeclaredNamespace.Context.class||!context.view.equals("context")||owner.seed!=seed)throw new AssertionError("nested generic namespace/enclosing identity");rows++;
  }
  try{consumer.value(null);throw new AssertionError("missing original null failure");}catch(NullPointerException expected){}
 }
 System.out.println(rows+":nested:generic:independent:namespace");}}
`
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaredNamespace", "IndependentConsumer"}, "NestedNamespaceDriver", "9:nested:generic:independent:namespace\n", nativeLexicalExactSignatures)
}

func TestAdversarialForeignMemberDeclarationKeepsCrossPackagePublicPath(t *testing.T) {
	const owner = `package declaration.provider;
public class Namespace<T> { public class Value<U> { public final U token; public Value(U token){this.token=token;} public Object outer(){return Namespace.this;} } }`
	const consumer = `package declaration.consumer;
public class Reader<T> { public class Local { public <U> U read(declaration.provider.Namespace<T>.Value<U> value){return value.token;} } }`
	const driver = `package declaration.consumer;
public class DeclarationDriver { public static void main(String[] args) { int rows=0; Object marker=new Object();
 for(Object token:new Object[]{null,marker,"value"}){declaration.provider.Namespace<Object> namespace=new declaration.provider.Namespace<>();declaration.provider.Namespace<Object>.Value<Object> value=namespace.new Value<>(token);Reader<Object>.Local local=new Reader<Object>().new Local();
 if(local.read(value)!=token||value.outer()!=namespace||value.getClass().getDeclaringClass()!=declaration.provider.Namespace.class||local.getClass().getDeclaringClass()!=Reader.class)throw new AssertionError("public namespace/independent enclosing instance");
 try{local.read(null);throw new AssertionError("missing original null failure");}catch(NullPointerException expected){} rows++;}
 System.out.println(rows+":public:separate:namespace");} }`
	testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, map[string]string{"declaration/provider/Namespace.java": owner, "declaration/consumer/Reader.java": consumer, "declaration/consumer/DeclarationDriver.java": driver}, debug, "8")
	}, []string{"declaration/provider/Namespace", "declaration/consumer/Reader"}, "declaration.consumer.DeclarationDriver", "3:public:separate:namespace\n", nil, nativeLexicalExactSignatures)
}

func TestAdversarialIndependentDeclarationFromDescriptorAndNestedBoundRoundTrip(t *testing.T) {
	// Keep the exact declarations from the former descriptor-only refusal.
	// Only the original root's redundant foreign InnerClasses row is omitted;
	// its descriptors, Signature and bytecode remain the independent input.
	const fixture = `class NativeArchiveOwner<T extends java.util.List<OtherOwner.Child>>{OtherOwner.Child field;java.util.List<OtherOwner.Child> generic;class Child{Child(){}}Child make(){return new Child();}}class OtherOwner{class Child{Child(){}}Child make(){return new Child();}}
class DescriptorDeclarationDriver{public static void main(String[]args)throws Exception{int rows=0;
 for(int iteration=0;iteration<3;iteration++){OtherOwner declaring=new OtherOwner();OtherOwner.Child foreign=declaring.make();NativeArchiveOwner<java.util.ArrayList<OtherOwner.Child>> reader=new NativeArchiveOwner<>();
  for(OtherOwner.Child value:new OtherOwner.Child[]{null,foreign,declaring.make()}){reader.field=value;reader.generic=java.util.Arrays.asList(value,null,foreign);NativeArchiveOwner<java.util.ArrayList<OtherOwner.Child>>.Child own=reader.make();
   if(reader.field!=value||reader.generic.get(0)!=value||reader.generic.get(2)!=foreign||own.getClass().getDeclaringClass()!=NativeArchiveOwner.class||foreign.getClass().getDeclaringClass()!=OtherOwner.class||!NativeArchiveOwner.class.getTypeParameters()[0].getBounds()[0].getTypeName().equals("java.util.List<OtherOwner$Child>")||!NativeArchiveOwner.class.getDeclaredField("generic").getGenericType().getTypeName().equals("java.util.List<OtherOwner$Child>"))throw new AssertionError("descriptor/bound/independent declaration");rows++;
  }
 }
 java.lang.reflect.Constructor<?> ctor=OtherOwner.Child.class.getDeclaredConstructor(OtherOwner.class);ctor.setAccessible(true);Object nullable=ctor.newInstance(new Object[]{null});java.lang.reflect.Field capture=OtherOwner.Child.class.getDeclaredField("this$0");capture.setAccessible(true);if(!capture.isSynthetic()||capture.get(nullable)!=null)throw new AssertionError("original nullable enclosing constructor");
 System.out.println(rows+":descriptor:bound:independent:declaration");}}
`
	testNativeIndependentMutatedFamilyFixture(t, fixture, []string{"NativeArchiveOwner", "OtherOwner"}, "DescriptorDeclarationDriver", "9:descriptor:bound:independent:declaration\n", func(t *testing.T, files map[string][]byte) {
		root, err := Parse(files["NativeArchiveOwner.class"])
		if err != nil {
			t.Fatal(err)
		}
		removed := 0
		for _, attribute := range root.Attributes {
			if table, ok := attribute.(*InnerClassesAttribute); ok {
				var kept []*InnerClassInfo
				for _, row := range table.Classes {
					name, known := sourceBridgeClassName(root, row.InnerClassInfoIndex)
					if !known {
						t.Fatal("original root declaration row")
					}
					if name == "OtherOwner$Child" {
						removed++
						continue
					}
					kept = append(kept, row)
				}
				table.Classes, table.NumberOfClasses, table.AttrLen = kept, uint16(len(kept)), uint32(2+8*len(kept))
			}
		}
		if removed != 1 {
			t.Fatalf("redundant root declaration row count=%d", removed)
		}
		files["NativeArchiveOwner.class"] = root.Bytes()
	}, nativeLexicalExactSignatures)
}

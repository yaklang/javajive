package javaclassparser

import (
	"strings"
	"testing"
)

// These two receivers belong to independent declarations but are the same
// original enclosing object after an ordinary-class widening. That relation
// does not grant the child source family private ownership of the superclass.
const inheritedAnonymousMemberSuperFixture = `class DeclaringNamespace{
 class Entry{final Object value;final Object observed;protected Entry(Object token){value=token;observed=actor();}Object actor(){return null;}Object outer(){return DeclaringNamespace.this;}}
}
class CurrentNamespace extends DeclaringNamespace{static class Anchor{}Entry create(Object token){return new Entry(token){Object actor(){return CurrentNamespace.this;}};}}
class InheritedAnonymousMemberDriver{public static void main(String[]args)throws Exception{int rows=0;Object marker=new Object();boolean declarations=true;
 for(int instance=0;instance<3;instance++){CurrentNamespace current=new CurrentNamespace();
  for(Object token:new Object[]{null,marker,"value"}){DeclaringNamespace.Entry entry=current.create(token);
   if(entry.value!=token||entry.outer()!=current||entry.actor()!=current||entry.observed!=current)throw new AssertionError("inherited anonymous SUPER enclosing/observation");
   declarations&=entry.getClass().isAnonymousClass()&&entry.getClass().getEnclosingClass()==CurrentNamespace.class&&entry.getClass().getEnclosingMethod()!=null&&entry.getClass().getEnclosingMethod().getName().equals("create")&&entry.getClass().getSuperclass()==DeclaringNamespace.Entry.class;rows++;
  }
 }
 System.out.println(rows+":inherited:anonymous:member:super");if(!declarations)throw new AssertionError("inherited anonymous declaration identity");}}
`

func TestAdversarialInheritedAnonymousMemberSuperPreservesEnclosingObservation(t *testing.T) {
	testNativeIndependentFamilyFixture(t, inheritedAnonymousMemberSuperFixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:super\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperIsIndependentOfSpelling(t *testing.T) {
	f := strings.ReplaceAll(inheritedAnonymousMemberSuperFixture, "DeclaringNamespace", "DeclaredOwner")
	f = strings.ReplaceAll(f, "CurrentNamespace", "UsingOwner")
	f = strings.ReplaceAll(f, "Entry", "Cursor")
	testNativeIndependentFamilyFixture(t, f, []string{"DeclaredOwner", "UsingOwner"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:super\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperPreservesNullableOriginalOuter(t *testing.T) {
	fixture := strings.Replace(inheritedAnonymousMemberSuperFixture,
		`System.out.println(rows+":inherited:anonymous:member:super");`,
		`java.lang.reflect.Constructor<?> ctor=Class.forName("CurrentNamespace$1").getDeclaredConstructor(CurrentNamespace.class,Object.class);ctor.setAccessible(true);
 DeclaringNamespace.Entry detached=(DeclaringNamespace.Entry)ctor.newInstance(null,marker);
 if(detached.value!=marker||detached.outer()!=null||detached.actor()!=null||detached.observed!=null)throw new AssertionError("nullable hidden enclosing operand");rows++;
 System.out.println(rows+":inherited:anonymous:member:super");`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "10:inherited:anonymous:member:super\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperPreservesExactOverload(t *testing.T) {
	fixture := strings.Replace(inheritedAnonymousMemberSuperFixture,
		`Object actor(){return null;}`,
		`protected Entry(String token){throw new AssertionError("wrong SUPER overload");}Object actor(){return null;}`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:super\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperPreservesGenericOwners(t *testing.T) {
	testNativeIndependentFamilyFixture(t, inheritedAnonymousMemberGenericFixture(), []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:generic\n", nativeLexicalExactSignatures)
}

func inheritedAnonymousMemberGenericFixture() string {
	return `class DeclaringNamespace<T>{final T outerValue;DeclaringNamespace(T value){outerValue=value;}
 class Entry<U>{final U value;final Object observed;protected Entry(U token){value=token;observed=actor();}Object actor(){return null;}Object outer(){return DeclaringNamespace.this;}T retained(){return outerValue;}}}
class CurrentNamespace<V> extends DeclaringNamespace<V>{static class Anchor{}CurrentNamespace(V value){super(value);}Entry<String> create(String token){return new Entry<String>(token){Object actor(){return CurrentNamespace.this;}};}}
class InheritedAnonymousMemberDriver{public static void main(String[]args){int rows=0;Object marker=new Object();
 for(Object value:new Object[]{null,marker,"outer"}){CurrentNamespace<Object> current=new CurrentNamespace<Object>(value);
  for(String token:new String[]{null,"","value"}){DeclaringNamespace<Object>.Entry<String> entry=current.create(token);
   if(entry.value!=token||entry.retained()!=value||entry.outer()!=current||entry.actor()!=current||entry.observed!=current)throw new AssertionError("generic declaring/enclosing owner binding");rows++;}}
 System.out.println(rows+":inherited:anonymous:member:generic");}}
`
}

func TestAdversarialInheritedAnonymousMemberSuperBindsOuterAndLeafConstructorFormals(t *testing.T) {
	fixture := strings.Replace(inheritedAnonymousMemberGenericFixture(), `final U value;`, `final T explicitOuter;final U value;`, 1)
	fixture = strings.Replace(fixture, `protected Entry(U token){value=token;`, `protected Entry(T outer,U token){explicitOuter=outer;value=token;`, 1)
	fixture = strings.Replace(fixture, `new Entry<String>(token)`, `new Entry<String>(outerValue,token)`, 1)
	fixture = strings.Replace(fixture, `entry.value!=token`, `entry.explicitOuter!=value||entry.value!=token`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:generic\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperRetainsIntermediateAncestry(t *testing.T) {
	fixture := strings.Replace(inheritedAnonymousMemberSuperFixture, `class CurrentNamespace extends DeclaringNamespace`, `class MiddleNamespace extends DeclaringNamespace{}class CurrentNamespace extends MiddleNamespace`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "MiddleNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:inherited:anonymous:member:super\n", nativeLexicalExactSignatures)
}

const inheritedAnonymousMemberNestedFixture = `class DeclaringNamespace{class Entry{final Object value;final Object observed;protected Entry(Object token){value=token;observed=actor();}Object actor(){return null;}Object outer(){return DeclaringNamespace.this;}}Entry create(Object token){return null;}}
class CurrentNamespace{static class Anchor{}DeclaringNamespace create(){return new DeclaringNamespace(){Entry create(Object token){return new Entry(token){Object actor(){return this.outer();}};}};}}
class InheritedAnonymousMemberDriver{public static void main(String[]args){Object marker=new Object();int rows=0;
 for(int instance=0;instance<3;instance++){DeclaringNamespace current=new CurrentNamespace().create();for(Object token:new Object[]{null,marker,"value"}){DeclaringNamespace.Entry entry=current.create(token);if(entry.value!=token||entry.outer()!=current||entry.actor()!=current||entry.observed!=current)throw new AssertionError("nested anonymous declaring/enclosing identity");
  if(!entry.getClass().isAnonymousClass()||entry.getClass().getEnclosingClass()!=current.getClass())throw new AssertionError("nested anonymous declaration");rows++;}}
 System.out.println(rows+":nested:inherited:anonymous:super");}}
`

func TestAdversarialInheritedAnonymousMemberSuperInsideAnonymousSubclass(t *testing.T) {
	fixture := inheritedAnonymousMemberNestedFixture
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:nested:inherited:anonymous:super\n", nativeLexicalExactSignatures)
}

func inheritedAnonymousMemberNamedContextFixture() string {
	fixture := strings.Replace(inheritedAnonymousMemberNestedFixture,
		`class CurrentNamespace{static class Anchor{}DeclaringNamespace create(){`,
		`class CurrentNamespace{static class Container{DeclaringNamespace create(){`, 1)
	fixture = strings.Replace(fixture, "}};}}\nclass InheritedAnonymousMemberDriver", "}};}}}\nclass InheritedAnonymousMemberDriver", 1)
	return strings.Replace(fixture, `new CurrentNamespace().create()`, `new CurrentNamespace.Container().create()`, 1)
}

func TestAdversarialInheritedAnonymousMemberSuperInsideNamedAndAnonymousScopes(t *testing.T) {
	testNativeIndependentFamilyFixture(t, inheritedAnonymousMemberNamedContextFixture(), []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "9:nested:inherited:anonymous:super\n", nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperPreservesForeignProtectedScope(t *testing.T) {
	sources := map[string]string{
		"source/provider/Declared.java": `package source.provider;public class Declared{protected class Entry{final Object value;final Object observed;protected Entry(Object token){value=token;observed=actor();}public Object actor(){return null;}public Object outer(){return Declared.this;}}public Entry make(Object token){return null;}}`,
		"source/client/UsingRoot.java":  `package source.client;import source.provider.Declared;public class UsingRoot{static class Anchor{}public Declared create(){return new Declared(){public Entry make(Object token){return new Entry(token){public Object actor(){return this.outer();}};}};}}`,
		"source/provider/Driver.java":   `package source.provider;import source.client.UsingRoot;public class Driver{public static void main(String[]args){Object marker=new Object();int rows=0;for(int instance=0;instance<3;instance++){Declared current=new UsingRoot().create();for(Object token:new Object[]{null,marker,"value"}){Declared.Entry entry=current.make(token);if(entry.value!=token||entry.outer()!=current||entry.actor()!=current||entry.observed!=current)throw new AssertionError("protected anonymous enclosing/observation");rows++;}}System.out.println(rows+":protected:anonymous:super");}}`,
	}
	testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
	}, []string{"source/provider/Declared", "source/client/UsingRoot"}, "source.provider.Driver", "9:protected:anonymous:super\n", nil, nativeLexicalExactSignatures)
}

func TestAdversarialInheritedAnonymousMemberSuperPreservesCallbackBeforeThrow(t *testing.T) {
	fixture := `class DeclaringNamespace{static String trace="";static Object seen;
 class Entry{protected Entry(boolean fail){trace+="super>";seen=actor();if(fail){trace+="throw>";throw new IllegalArgumentException("original");}trace+="end>";}Object actor(){return null;}}}
class CurrentNamespace extends DeclaringNamespace{static class Anchor{}Entry create(boolean fail){trace+="allocate>";return new Entry(fail){Object actor(){trace+="callback>";return CurrentNamespace.this;}};}}
class InheritedAnonymousMemberDriver{public static void main(String[]args){int rows=0;
 for(boolean fail:new boolean[]{false,true}){CurrentNamespace current=new CurrentNamespace();DeclaringNamespace.trace="";DeclaringNamespace.seen=null;
  try{current.create(fail);if(fail)throw new AssertionError("missing exception");}catch(IllegalArgumentException ex){if(!fail||!"original".equals(ex.getMessage()))throw new AssertionError("exception identity");}
  String expected=fail?"allocate>super>callback>throw>":"allocate>super>callback>end>";
  if(!expected.equals(DeclaringNamespace.trace)||DeclaringNamespace.seen!=current)throw new AssertionError("capture/callback/exception order");rows++;}
 System.out.println(rows+":inherited:anonymous:member:effects");}}
`
	testNativeIndependentFamilyFixture(t, fixture, []string{"DeclaringNamespace", "CurrentNamespace"}, "InheritedAnonymousMemberDriver", "2:inherited:anonymous:member:effects\n", nativeLexicalExactSignatures)
}

package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

const nativeConstructorEnumLocalFixture = `
interface EnumCtorRead {long read(); Object owner(); Object token();}
enum EnumCtorScope {
 FIRST(new Object(),Long.MIN_VALUE,-1), SECOND(null,Long.MAX_VALUE,1);
 long live; final long wide; final int delta; final Object input; EnumCtorRead first,view;
 EnumCtorScope(final Object token,final long wide,final int delta){this(token,wide,delta,false);
  class View implements EnumCtorRead { public long read(){return live+wide+delta;} public Object owner(){return EnumCtorScope.this;} public Object token(){return token;} }
  view=new View();
 }
 EnumCtorScope(final Object token,final long wide,final int delta,boolean unused){this.input=token;this.wide=wide;this.delta=delta;live=17;
  class View implements EnumCtorRead { public long read(){return live+wide+delta;} public Object owner(){return EnumCtorScope.this;} public Object token(){return token;} }
  first=new View();
 }
}
class EnumCtorDriver {public static void main(String[]args){int rows=0;
 for(EnumCtorScope root:EnumCtorScope.values())for(long state:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
 root.live=state;
 if(root.first.read()!=state+root.wide+root.delta||root.view.read()!=state+root.wide+root.delta||root.first.owner()!=root||root.view.owner()!=root||root.first.token()!=root.input||root.view.token()!=root.input)throw new AssertionError("constructor-local capture effects");
 if(!root.first.getClass().isLocalClass()||!root.view.getClass().isLocalClass()||root.first.getClass().getEnclosingConstructor().getParameterTypes().length!=6||root.view.getClass().getEnclosingConstructor().getParameterTypes().length!=5||root.first.getClass().getEnclosingConstructor().getDeclaringClass()!=EnumCtorScope.class||root.first.getClass()==root.view.getClass())throw new AssertionError("original overloaded enum constructor identity"); rows++;
 }System.out.println(rows+":enum:implicit:ports:constructor:locals");}}
`

func TestAdversarialEnumConstructorLocalsKeepProjectedDelegation(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeConstructorEnumLocalFixture, []string{"EnumCtorScope"}, "EnumCtorDriver", "10:enum:implicit:ports:constructor:locals\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}

func TestAdversarialMemberConstructorLocalKeepsOriginalMethodFormalScope(t *testing.T) {
	source := nativeConstructorLocalNamedFixture(nativeConstructorLocalNullPortFixture())
	source = strings.Replace(source, "CtorScopeRoot(final Object captured,", "<T> CtorScopeRoot(final T captured,", 1)
	source = strings.Replace(source, "getParameterTypes().length!=4", "getParameterTypes().length!=4||local.getEnclosingConstructor().getTypeParameters().length!=1", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeContainer"}, "CtorScopeDriver", "30:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}
func TestAdversarialMemberConstructorLocalMethodFormalShadowsOwnerFormal(t *testing.T) {
	source := nativeConstructorLocalNamedFixture(nativeConstructorLocalNullPortFixture())
	source = strings.Replace(source, "class CtorScopeContainer{", "class CtorScopeContainer<T extends CharSequence>{", 1)
	source = strings.Replace(source, "CtorScopeRoot(final Object captured,", "<T extends Number> CtorScopeRoot(final T captured,", 1)
	source = strings.Replace(source, "new CtorScopeRoot(new Object(),3,991)", "new CtorScopeRoot(Integer.valueOf(3),3,991)", 1)
	source = strings.Replace(source, "for(Object captured:new Object[]{null,new Object()})", "for(Number captured:new Number[]{null,Integer.valueOf(7),Long.valueOf(-3)})", 1)
	source = strings.Replace(source, "getParameterTypes().length!=4", "getParameterTypes().length!=4||local.getEnclosingConstructor().getTypeParameters().length!=1||local.getEnclosingConstructor().getTypeParameters()[0].getBounds()[0]!=Number.class", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeContainer"}, "CtorScopeDriver", "45:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}

const portsReviewArrayPrefixSource = `
class PortArrayBase {}
class PortArrayChild extends PortArrayBase {}
class PortArraySuper {final PortArrayBase[] seen;PortArraySuper(PortArrayBase[] value){seen=value;}}
interface PortArrayRead {Object read();}
class PortArrayOuter {
 class Inner extends PortArraySuper {
  final PortArrayRead view;
  <U> Inner(final U token){super(new PortArrayBase[]{new PortArrayChild()});
   class View implements PortArrayRead {public U read(){return token;}}
   view=new View();
  }
 }
}
class PortArrayDriver {public static void main(String[]args){Object token=new Object();PortArrayOuter.Inner value=new PortArrayOuter().new Inner(token);
 if(value.view.read()!=token||value.seen.length!=1||value.seen[0].getClass()!=PortArrayChild.class)throw new AssertionError("prefix binding independent of source array subtype");
 System.out.println("array:prefix:binding");}}
`

func TestAdversarialConstructorLocalBindingUsesOriginalArrayStoreIdentity(t *testing.T) {
	testNativeIndependentFamilyFixture(t, portsReviewArrayPrefixSource, []string{"PortArrayOuter"}, "PortArrayDriver", "array:prefix:binding\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}

func TestAdversarialConstructorLocalArrayStoreConcreteControl(t *testing.T) {
	source := strings.Replace(portsReviewArrayPrefixSource, "<U> Inner(final U token)", "Inner(final Object token)", 1)
	source = strings.Replace(source, "public U read()", "public Object read()", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"PortArrayOuter"}, "PortArrayDriver", "array:prefix:binding\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}

func nativeHiddenConstructorParameterMetadata(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	inspect := func(raw []byte) []string {
		object, e := Parse(raw)
		if e != nil {
			t.Fatal(e)
		}
		var out []string
		for _, method := range object.Methods {
			name, e := sourceBridgeUTF8(object, method.NameIndex)
			if !e {
				t.Fatal("method name")
			}
			desc, e := sourceBridgeUTF8(object, method.DescriptorIndex)
			if !e {
				t.Fatal("method descriptor")
			}
			for _, attribute := range method.Attributes {
				table, ok := attribute.(*UnparsedAttribute)
				if !ok || table == nil || table.Name != "MethodParameters" {
					continue
				}
				if len(table.Info) < 1 || len(table.Info) != 1+4*int(table.Info[0]) {
					t.Fatal("original parameter table")
				}
				row := name + desc + ":"
				for i := 0; i < int(table.Info[0]); i++ {
					data := table.Info[1+4*i : 5+4*i]
					index := binary.BigEndian.Uint16(data[:2])
					param := "<unnamed>"
					if index != 0 {
						param, e = sourceBridgeUTF8(object, index)
						if !e {
							t.Fatal("parameter name")
						}
					}
					row += fmt.Sprintf("%d=%s/%04x;", i, param, binary.BigEndian.Uint16(data[2:]))
				}
				out = append(out, row)
			}
		}
		slices.Sort(out)
		return out
	}
	if before, after := inspect(original), inspect(rebuilt); !slices.Equal(before, after) {
		t.Fatalf("physical parameter metadata %s\noriginal %v\nrebuilt %v", name, before, after)
	}
}

func nativeHiddenMemberMethodFormalSource(shadow bool) string {
	source := nativeConstructorLocalNamedFixture(nativeConstructorLocalNullPortFixture())
	formal := "<T> CtorScopeRoot(final T captured,"
	if shadow {
		source = strings.Replace(source, "class CtorScopeContainer{", "class CtorScopeContainer<T extends CharSequence>{", 1)
		formal = "<T extends Number> CtorScopeRoot(final T captured,"
		source = strings.Replace(source, "new CtorScopeRoot(new Object(),3,991)", "new CtorScopeRoot(Integer.valueOf(3),3,991)", 1)
		source = strings.Replace(source, "for(Object captured:new Object[]{null,new Object()})", "for(Number captured:new Number[]{null,Integer.valueOf(7),Long.valueOf(-3)})", 1)
	}
	source = strings.Replace(source, "CtorScopeRoot(final Object captured,", formal, 1)
	extra := "getParameterTypes().length!=4||local.getEnclosingConstructor().getTypeParameters().length!=1"
	if shadow {
		extra += "||local.getEnclosingConstructor().getTypeParameters()[0].getBounds()[0]!=Number.class"
	}
	return strings.Replace(source, "getParameterTypes().length!=4", extra, 1)
}
func TestAdversarialConstructorHiddenPortsMatchSameTypedPayload(t *testing.T) {
	source := strings.ReplaceAll(nativeConstructorEnumLocalFixture, "final Object token", "final String token")
	source = strings.Replace(source, "FIRST(new Object(),", "FIRST(\"same-typed-payload\",", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"EnumCtorScope"}, "EnumCtorDriver", "10:enum:implicit:ports:constructor:locals\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}
func TestAdversarialConstructorHiddenPortsKeepEffectivelyFinalMetadata(t *testing.T) {
	source := strings.NewReplacer("final Object token", "Object token", "final long wide", "long wide", "final int delta", "int delta").Replace(nativeConstructorEnumLocalFixture)
	testNativeIndependentFamilyFixture(t, source, []string{"EnumCtorScope"}, "EnumCtorDriver", "10:enum:implicit:ports:constructor:locals\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}
func TestAdversarialConstructorHiddenPortsPackagedRenamedScope(t *testing.T) {
	source := "package ports.scoped;\n" + strings.NewReplacer("EnumCtorScope", "ReboundPorts", "EnumCtorRead", "ReboundRead", "View", "LocalRead").Replace(nativeConstructorEnumLocalFixture)
	testNativeIndependentFamilyFixture(t, source, []string{"ports/scoped/ReboundPorts"}, "ports.scoped.EnumCtorDriver", "10:enum:implicit:ports:constructor:locals\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}
func TestAdversarialConstructorHiddenPortsPreserveDependentOwnerBound(t *testing.T) {
	source := nativeHiddenMemberMethodFormalSource(true)
	source = strings.Replace(source, "class CtorScopeContainer<T extends CharSequence>", "class CtorScopeContainer<T extends Number>", 1)
	source = strings.Replace(source, "<T extends Number> CtorScopeRoot(final T captured,", "<U extends T> CtorScopeRoot(final U captured,", 1)
	source = strings.Replace(source, "||local.getEnclosingConstructor().getTypeParameters()[0].getBounds()[0]!=Number.class", "||!local.getEnclosingConstructor().getTypeParameters()[0].getBounds()[0].equals(CtorScopeContainer.class.getTypeParameters()[0])", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"CtorScopeContainer"}, "CtorScopeDriver", "45:ctor:local:live:wide:scope:failure\n", nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
}
func TestAdversarialConstructorHiddenPortsOriginalCompiler(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	for _, kind := range []string{"enum", "member", "shadow"} {
		t.Run(kind, func(t *testing.T) {
			source, owner, driver, want := nativeConstructorEnumLocalFixture, "EnumCtorScope", "EnumCtorDriver", "10:enum:implicit:ports:constructor:locals\n"
			if kind != "enum" {
				source = nativeHiddenMemberMethodFormalSource(kind == "shadow")
				owner = "CtorScopeContainer"
				driver = "CtorScopeDriver"
				want = "30:ctor:local:live:wide:scope:failure\n"
				if kind == "shadow" {
					want = "45:ctor:local:live:wide:scope:failure\n"
				}
			}
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, source, owner, debug) }, NativeJavac8, javac, []string{owner}, driver, want, nil, nativeLexicalExactSignatures, nativeHiddenConstructorParameterMetadata)
		})
	}
}

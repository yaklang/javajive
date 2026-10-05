package javaclassparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// Both ordinary and constant-specific enums must retain their original member
// declaration owner. A utility family's other member types cannot be flattened
// solely because it also owns an enum.
const nativeEnumScopeFixture = `class EnumScopeOwner{static class Token{final Object value;Token(Object value){this.value=value;}}enum Mode{LEFT,RIGHT;Object token(Token value){return value.value;}}Token make(Object value){return new Token(value);}}
class EnumScopeDriver{public static void main(String[]args)throws Exception{int rows=0;for(Object value:new Object[]{null,new Object()}){EnumScopeOwner.Token token=new EnumScopeOwner().make(value);for(EnumScopeOwner.Mode mode:EnumScopeOwner.Mode.values()){if(mode.token(token)!=value||mode.getDeclaringClass()!=EnumScopeOwner.Mode.class||mode.getClass().getEnclosingClass()!=EnumScopeOwner.class||EnumScopeOwner.Mode.class.getDeclaringClass()!=EnumScopeOwner.class||token.getClass().getDeclaringClass()!=EnumScopeOwner.class||EnumScopeOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("enum and sibling declaration owners");rows++;}EnumScopeOwner.Mode[] copy=EnumScopeOwner.Mode.values();copy[0]=null;if(EnumScopeOwner.Mode.values()[0]!=EnumScopeOwner.Mode.LEFT||EnumScopeOwner.Mode.valueOf("RIGHT")!=EnumScopeOwner.Mode.RIGHT)throw new AssertionError("enum synthesized factories");try{EnumScopeOwner.Mode.valueOf(null);throw new AssertionError("lost null failure");}catch(NullPointerException expected){}try{EnumScopeOwner.Mode.valueOf("UNKNOWN");throw new AssertionError("lost unknown failure");}catch(IllegalArgumentException expected){}}System.out.println(rows+":enum:member:sibling:identity");}}`

func TestNativeMemberEnumSiblingDeclarationOwnersRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeEnumScopeFixture, []string{"EnumScopeOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

// Earlier compilers emitted the values-array packet directly in <clinit>
// instead of the private synthetic $values helper. Only that proven packet is
// moved; original constant allocations, array order and user tail remain intact.
func TestNativeMemberEnumInlineValuesArrayCompilerProtocolRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "EnumScopeOwner", "EnumScopeDriver", "4:enum:member:sibling:identity\n", func(t *testing.T, debug string) map[string][]byte {
		files := nativeCompileDebugClasses(t, nativeEnumScopeFixture, debug)
		obj, err := Parse(files["EnumScopeOwner$Mode.class"])
		if err != nil {
			t.Fatal(err)
		}
		var helper *MemberInfo
		var initializer, array *CodeAttribute
		for _, method := range obj.Methods {
			name, _ := sourceBridgeUTF8(obj, method.NameIndex)
			for _, attribute := range method.Attributes {
				if code, ok := attribute.(*CodeAttribute); ok {
					if name == "$values" {
						helper, array = method, code
					}
					if name == "<clinit>" {
						initializer = code
					}
				}
			}
		}
		if helper == nil || initializer == nil || array == nil {
			t.Fatal("original generated array protocol missing")
		}
		if len(array.Code) == 0 || array.Code[len(array.Code)-1] != byte(core.OP_ARETURN) {
			t.Fatal("array packet terminator")
		}
		call := -1
		for i := 0; i+2 < len(initializer.Code); i++ {
			if initializer.Code[i] == byte(core.OP_INVOKESTATIC) {
				index := uint16(initializer.Code[i+1])<<8 | uint16(initializer.Code[i+2])
				member, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
				if !ok {
					continue
				}
				nt, ok := obj.ConstantPool[member.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok {
					continue
				}
				name, _ := sourceBridgeUTF8(obj, nt.NameIndex)
				if name == "$values" {
					call = i
					break
				}
			}
		}
		if call < 0 {
			t.Fatal("original generated array invocation missing")
		}
		code := append([]byte(nil), initializer.Code[:call]...)
		code = append(code, array.Code[:len(array.Code)-1]...)
		code = append(code, initializer.Code[call+3:]...)
		initializer.Code, initializer.Attributes = code, nil
		initializer.AttrLen = uint32(12 + len(code))
		if initializer.MaxStack < array.MaxStack {
			initializer.MaxStack = array.MaxStack
		}
		methods := obj.Methods[:0]
		for _, method := range obj.Methods {
			if method != helper {
				methods = append(methods, method)
			}
		}
		obj.Methods = methods
		files["EnumScopeOwner$Mode.class"] = obj.Bytes()
		return files
	}, nativeLexicalExactSignatures)
}
func TestNativeMemberEnumSiblingDeclarationOwnersRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeEnumScopeFixture, "EnumScopeOwner", "StrategyOwner")
	f = strings.ReplaceAll(f, "Token", "Element")
	f = strings.ReplaceAll(f, "Mode", "Choice")
	testNativeIndependentFamilyFixture(t, f, []string{"StrategyOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

func TestNativeMemberEnumOrdinarySelfAliasIsNotAConstantRoundTrip(t *testing.T) {
	f := strings.Replace(nativeEnumScopeFixture, "LEFT,RIGHT;", "LEFT,RIGHT;public static final Mode ALIAS=LEFT;", 1)
	f = strings.Replace(f, "EnumScopeOwner.Mode[] copy=", "if(EnumScopeOwner.Mode.ALIAS!=EnumScopeOwner.Mode.LEFT||EnumScopeOwner.Mode.values().length!=2)throw new AssertionError(\"ordinary self-typed alias became a constant\");EnumScopeOwner.Mode[] copy=", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"EnumScopeOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

// The same declaration-identity distinction applies without a lexical alias.
// A public static final self-typed field is not a constant unless ACC_ENUM is
// present in its original field_info; treating ALIAS as a third declaration
// changes values(), name(), ordinal() and reference identity.
func TestNativeEnumTopLevelOrdinarySelfAliasRoundTrip(t *testing.T) {
	const fixture = `enum AliasChoice{LEFT,RIGHT;public static final AliasChoice ALIAS=LEFT;}class AliasDriver{public static void main(String[]args){AliasChoice[] copy=AliasChoice.values();if(copy.length!=2||copy[0]!=AliasChoice.LEFT||copy[1]!=AliasChoice.RIGHT||AliasChoice.ALIAS!=AliasChoice.LEFT||AliasChoice.ALIAS.ordinal()!=0||!AliasChoice.ALIAS.name().equals("LEFT"))throw new AssertionError("constant versus ordinary declaration");copy[0]=null;if(AliasChoice.values()[0]!=AliasChoice.LEFT)throw new AssertionError("clone");System.out.println("2:alias:identity:ordinal:clone");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"AliasChoice"}, "AliasDriver", "2:alias:identity:ordinal:clone\n", nativeLexicalExactSignatures)
}

func TestNativeEnumOrdinarySelfAliasWithoutLegacyShapeRepairsRoundTrip(t *testing.T) {
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	TestNativeEnumTopLevelOrdinarySelfAliasRoundTrip(t)
	TestNativeMemberEnumOrdinarySelfAliasIsNotAConstantRoundTrip(t)
}

func TestNativeMemberEmptyEnumAndSiblingDeclarationOwnersRoundTrip(t *testing.T) {
	const fixture = `class EmptyEnumOwner{enum State{}static class Token{}}class EmptyEnumDriver{public static void main(String[]args){if(EmptyEnumOwner.State.values().length!=0||EmptyEnumOwner.State.class.getDeclaringClass()!=EmptyEnumOwner.class||new EmptyEnumOwner.Token().getClass().getDeclaringClass()!=EmptyEnumOwner.class||EmptyEnumOwner.class.getDeclaredClasses().length!=2)throw new AssertionError("empty enum ownership");try{EmptyEnumOwner.State.valueOf("missing");throw new AssertionError("missing failure");}catch(IllegalArgumentException expected){}System.out.println("empty:enum:member:sibling");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"EmptyEnumOwner"}, "EmptyEnumDriver", "empty:enum:member:sibling\n", nativeLexicalExactSignatures)
}

func TestNativeMemberEnumConstructorEffectsRemainInOriginalOrderRoundTrip(t *testing.T) {
	f := strings.Replace(nativeEnumScopeFixture, "class EnumScopeOwner", `class EnumScopeEffects{static String trace="";}class EnumScopeOwner`, 1)
	f = strings.Replace(f, "LEFT,RIGHT;", `LEFT,RIGHT;private Mode(){EnumScopeEffects.trace+="C";}`, 1)
	f = strings.Replace(f, "System.out.println(rows+", `if(!EnumScopeEffects.trace.equals("CC"))throw new AssertionError("enum constructor effects:"+EnumScopeEffects.trace);System.out.println(rows+`, 1)
	testNativeIndependentFamilyFixture(t, f, []string{"EnumScopeOwner"}, "EnumScopeDriver", "4:enum:member:sibling:identity\n", nativeLexicalExactSignatures)
}

// Factory replacement is independent of source constructor arity. Likewise,
// $values is not a reserved source method name: ordinary overloads must survive.
func TestNativeEnumFactoriesWithSourceConstructorAndUserHelperRoundTrip(t *testing.T) {
	const fixture = `enum ArityChoice{LEFT(7),RIGHT(11);final int n;private ArityChoice(int n){this.n=n;}static int $values(int x){return x+31;}}class ArityDriver{public static void main(String[]args){ArityChoice[] copy=ArityChoice.values();copy[0]=null;if(ArityChoice.values()[0]!=ArityChoice.LEFT||ArityChoice.LEFT.n!=7||ArityChoice.RIGHT.n!=11||ArityChoice.$values(4)!=35||ArityChoice.valueOf("RIGHT")!=ArityChoice.RIGHT)throw new AssertionError("factories/constructor/ordinary helper");try{ArityChoice.valueOf(null);throw new AssertionError("null priority");}catch(NullPointerException expected){}System.out.println("arity:helper:clone:identity");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"ArityChoice"}, "ArityDriver", "arity:helper:clone:identity\n", nativeLexicalExactSignatures)
}

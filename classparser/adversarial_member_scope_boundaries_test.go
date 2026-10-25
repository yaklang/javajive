package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// A static declaration cuts off outer class formals even when several physical
// '$' name components separate it from the independently rebuilt member.
func TestAdversarialMemberScopeArityStopsAtStaticDeclaration(t *testing.T) {
	fixture := `class SuffixOwner<T extends Number> {
 static class Cut {class Middle {class Member {int value(int n){return n*31+7;}} Member make(){return new Member();}}}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 SuffixOwner.Cut cut=new SuffixOwner.Cut();SuffixOwner.Cut.Middle middle=cut.new Middle();
 Class<?> cls=SuffixOwner.Cut.Middle.Member.class;
 if(cls.getTypeParameters().length!=0||cls.getEnclosingClass()!=SuffixOwner.Cut.Middle.class)throw new AssertionError("static cut binder");
 int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}) {
  SuffixOwner.Cut.Middle.Member member=middle.make();
  if(member.value(n)!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).intValue())throw new AssertionError("arithmetic"); rows++;
 }System.out.println(rows+":static:scope:arity:overflow");}}
`
	for _, root := range []string{"SuffixOwner", "DifferentScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", root, debug, profile), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Cut$Middle$Member"}, "5:static:scope:arity:overflow\n", 8, 8, profile)
				})
			}
		}
	}
}

func TestAdversarialGenericMemberScopeRetainsPrivateConstructorPorts(t *testing.T) {
	for _, bound := range []string{"Number", "CharSequence"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", bound, debug, profile), func(t *testing.T) {
					source := strings.Replace(externalOwnerBoundFixture(bound), "Member(){}", "private Member(){}", 1)
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, source, debug, []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n", 8, 8, profile)
				})
			}
		}
	}
}

func TestAdversarialGenericScopeComposesWithParameterTableRestoration(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, externalOwnerBoundFixture("Number"), "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n")
	obj := mustParseLowering(t, fresh)
	for _, m := range obj.Methods {
		if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name != "<init>" {
			continue
		}
		obj.ConstantPoolManager.AddUtf8Info("MethodParameters")
		name := uint16(obj.ConstantPoolManager.AddUtf8Info("enclosingParameter"))
		m.Attributes = append(m.Attributes, &UnparsedAttribute{Name: "MethodParameters", Length: 5, Info: []byte{1, byte(name >> 8), byte(name), 0, 0}})
	}
	input := obj.Bytes()
	out, err := artifact.FinishClass("SuffixOwner$Member.class", input)
	if err != nil {
		t.Fatal("compose complete parameter and scope proofs", err)
	}
	if err := sourceMethodMetadataClosed(mustParseLowering(t, original), out, nil); err != nil {
		t.Fatal("original method contract", err)
	}
	before, after := mustParseLowering(t, input), mustParseLowering(t, out)
	for i, m := range before.Methods {
		left, right := constructorLoweringCode(m), constructorLoweringCode(after.Methods[i])
		if (left == nil) != (right == nil) || left != nil && !bytes.Equal(left.Code, right.Code) {
			t.Fatal("parameter/scope composition changed executable code")
		}
	}
}

func TestAdversarialMemberScopeRetainsParameterizedSuperclassOwner(t *testing.T) {
	fixture := `class SuffixOwner<T extends Number> {
 final T seed;SuffixOwner(T seed){this.seed=seed;}
 class Base<U> {Base(){} U value(){return null;}}
 class Member extends Base<T> {Member(){} T value(){return SuffixOwner.this.seed;}}
 Member make(){return new Member();}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 java.lang.reflect.ParameterizedType parent=(java.lang.reflect.ParameterizedType)SuffixOwner.Member.class.getGenericSuperclass();
 java.lang.reflect.ParameterizedType enclosing=(java.lang.reflect.ParameterizedType)parent.getOwnerType();
 java.lang.reflect.Type variable=SuffixOwner.class.getTypeParameters()[0];
 if(parent.getRawType()!=SuffixOwner.Base.class||enclosing.getRawType()!=SuffixOwner.class||!parent.getActualTypeArguments()[0].equals(variable)||!enclosing.getActualTypeArguments()[0].equals(variable)||SuffixOwner.Member.class.getTypeParameters().length!=0)throw new AssertionError("superclass owner/binder identity");
 int rows=0;for(Number n:new Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")}) {
  SuffixOwner<Number> owner=new SuffixOwner<Number>(n);SuffixOwner<Number>.Member result=owner.make();
  if(result.value()!=n||((SuffixOwner<Number>.Base<Number>)result).value()!=n)throw new AssertionError("generic dispatch/identity"); rows++;
 }System.out.println(rows+":nested:generic:super:owner:identity");}}
`
	for _, root := range []string{"SuffixOwner", "NestedScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", root, debug, profile), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Member", root + "$Base"}, "4:nested:generic:super:owner:identity\n", 8, 8, profile)
				})
			}
		}
	}
}

func TestAdversarialMemberScopeCaptureUsesDeclaredReferenceWidening(t *testing.T) {
	fixture := `interface ScopeView<T>{T value();}
class ScopeBase<T>{final ScopeView<T> view;ScopeBase(ScopeView<T> view){this.view=view;}T get(){return view.value();}}
class SuffixOwner<T extends Number> implements ScopeView<T> {
 final T seed;SuffixOwner(T seed){this.seed=seed;}public T value(){return seed;}
 class Member extends ScopeBase<T>{Member(){super(SuffixOwner.this);}}
 Member make(){return new Member();}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 java.lang.reflect.ParameterizedType parent=(java.lang.reflect.ParameterizedType)SuffixOwner.Member.class.getGenericSuperclass();
 if(parent.getRawType()!=ScopeBase.class||!parent.getActualTypeArguments()[0].equals(SuffixOwner.class.getTypeParameters()[0]))throw new AssertionError("super binder");
 int rows=0;for(Number n:new Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")}){
  SuffixOwner<Number> owner=new SuffixOwner<Number>(n);SuffixOwner<Number>.Member value=owner.make();if(value.get()!=n||value.view!=owner)throw new AssertionError("widened capture identity");rows++;
 }System.out.println(rows+":capture:subtype:dispatch:identity");}}
`
	for _, root := range []string{"SuffixOwner", "WidenedScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(fmt.Sprintf("%s/%s/%s", root, debug, profile), func(t *testing.T) {
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(fixture, "SuffixOwner", root), debug, []string{root + "$Member"}, "4:capture:subtype:dispatch:identity\n", 8, 8, profile)
				})
			}
		}
	}
}

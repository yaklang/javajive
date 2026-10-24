package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The source-level constructor omits its physical enclosing-instance port.
// Its payload still resolves T in the original outer declaration, not in a
// newly invented member binder. The unchanged factory exercises the real ABI.
const enclosingConstructorPayloadFixture = `class SuffixOwner<T extends Number> {
 class Member {final T item;Member(T item){this.item=item;}T get(){return item;}}
 Member make(T item){return new Member(item);}
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 Class<?> member=SuffixOwner.Member.class;
 java.lang.reflect.Constructor<?> ctor=member.getDeclaredConstructor(SuffixOwner.class,Number.class);
 java.lang.reflect.Type[] params=ctor.getGenericParameterTypes();
 if(params.length!=1||!params[0].equals(SuffixOwner.class.getTypeParameters()[0])||member.getTypeParameters().length!=0)throw new AssertionError("constructor lexical binder/hidden port");
 int rows=0;for(Number n:new Number[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")}){
 SuffixOwner<Number> owner=new SuffixOwner<Number>();SuffixOwner<Number>.Member a=owner.make(n),b=owner.make(n);
 if(a==b||a.get()!=n||b.get()!=n||ctor.newInstance(owner,n).getClass()!=member)throw new AssertionError("original factory/binding/identity");rows++;
 }System.out.println(rows+":constructor:outer:payload:binding");}}
`

func TestAdversarialMemberConstructorRetainsEnclosingPayloadBinder(t *testing.T) {
	fixture := enclosingConstructorPayloadFixture
	for _, bound := range []string{"Number", "CharSequence"} {
		source := fixture
		if bound == "CharSequence" {
			source = strings.ReplaceAll(source, "Number", "CharSequence")
			source = strings.Replace(source, `Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")`, `"immutable",new StringBuilder("mutable"),new StringBuffer("separate")`, 1)
		}
		for _, root := range []string{"SuffixOwner", "PayloadScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
					t.Run(fmt.Sprintf("%s/%s/%s/%s", bound, root, debug, profile), func(t *testing.T) {
						constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, strings.ReplaceAll(source, "SuffixOwner", root), debug, []string{root + "$Member"}, "4:constructor:outer:payload:binding\n", 8, 8, profile)
					})
				}
			}
		}
	}
}

// A constructor and its physical class header publish independent receipts.
// Their composition must prove both obligations, including on repeated Finish.
func TestAdversarialEnclosingConstructorProjectionRejectsPartialProofs(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, enclosingConstructorPayloadFixture, "none", []string{"SuffixOwner$Member"}, "4:constructor:outer:payload:binding\n")
	base := artifact.source.jar.sourceConstructorLowering
	owner := "SuffixOwner$Member"
	for _, variant := range []string{"control", "already restored", "class bound", "class formal identity", "method class role", "missing binding receipt", "missing scope receipt", "pending scope only", "changed original"} {
		t.Run(variant, func(t *testing.T) {
			authority := &sourceConstructorLoweringAuthority{jar: base.jar, bindingActive: map[string]constructorNativeSourceOrigin{owner: base.bindingActive[owner]}, genericActive: map[string]constructorNativeSourceOrigin{owner: base.genericActive[owner]}}
			input := bytes.Clone(fresh)
			old := bytes.Clone(original)
			obj := mustParseLowering(t, input)
			if variant == "class bound" || variant == "class formal identity" {
				for _, attr := range obj.Attributes {
					if sig, ok := attr.(*SignatureAttribute); ok {
						value := "<T:Ljava/lang/Object;>Ljava/lang/Object;"
						if variant == "class formal identity" {
							value = "<U:Ljava/lang/Number;>Ljava/lang/Object;"
						}
						sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(value))
					}
				}
			}
			if variant == "method class role" {
				for _, m := range obj.Methods {
					if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
						for _, attr := range m.Attributes {
							if sig, ok := attr.(*SignatureAttribute); ok {
								sig.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(LSuffixOwner;LT;)V"))
							}
						}
					}
				}
			}
			input = obj.Bytes()
			switch variant {
			case "missing binding receipt":
				authority.bindingActive = nil
			case "missing scope receipt":
				authority.genericActive = nil
			case "pending scope only":
				authority.genericPending = authority.genericActive
				authority.genericActive = nil
			case "changed original":
				old = append(old, 0)
			case "already restored":
				var err error
				input, err = authority.restoreConstructorBindingSignatures(old, input, nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			before := bytes.Clone(input)
			result, err := authority.restoreConstructorBindingSignatures(old, input, nil)
			if !bytes.Equal(input, before) {
				t.Fatal("mutated caller bytes")
			}
			positive := variant == "control" || variant == "already restored"
			if !positive {
				if err == nil && sourceMethodMetadataClosed(mustParseLowering(t, original), result, nil) == nil {
					t.Fatal("partial proof accepted metadata")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := sourceMethodMetadataClosed(mustParseLowering(t, original), result, nil); err != nil {
				t.Fatal(err)
			}
			left, right := mustParseLowering(t, input), mustParseLowering(t, result)
			for i, m := range left.Methods {
				a, b := constructorLoweringCode(m), constructorLoweringCode(right.Methods[i])
				if (a == nil) != (b == nil) || a != nil && !bytes.Equal(a.Code, b.Code) {
					t.Fatal("signature transaction changed executable instructions")
				}
			}
			again, err := authority.restoreConstructorBindingSignatures(old, result, nil)
			if err != nil || !bytes.Equal(result, again) {
				t.Fatal("non-idempotent binding", err)
			}
		})
	}
}

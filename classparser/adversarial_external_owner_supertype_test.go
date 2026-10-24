package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialCompletedMemberScopeStillChecksPhysicalSupertype(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, externalOwnerSupertypeFixture("Number"), "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n")
	base := artifact.source.jar.sourceConstructorLowering
	completed, err := base.restoreMemberGenericScope(original, fresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A ClassFile may carry a syntactically valid Signature that disagrees
	// with its physical superclass. Equal original/fresh strings and an
	// already present membership row do not certify that declaration.
	change := func(raw []byte) []byte {
		o := mustParseLowering(t, bytes.Clone(raw))
		for _, attr := range o.Attributes {
			if sig, ok := attr.(*SignatureAttribute); ok {
				sig.SignatureIndex = uint16(o.ConstantPoolManager.AddUtf8Info("Ljava/lang/Object;"))
			}
		}
		return o.Bytes()
	}
	old, next := change(original), change(completed)
	owner := "SuffixOwner$Member"
	origin := base.genericActive[owner]
	origin.originalHash = sha256.Sum256(old)
	a := &sourceConstructorLoweringAuthority{jar: base.jar, genericActive: map[string]constructorNativeSourceOrigin{owner: origin}}
	before := bytes.Clone(next)
	if result, err := a.restoreMemberGenericScope(old, next, nil); err == nil || result != nil {
		t.Fatal("completed state admitted a signature unrelated to its physical superclass", err)
	}
	if !bytes.Equal(next, before) {
		t.Fatal("failed completion mutated input")
	}
}

func TestAdversarialMemberGenericSupertypeCompletionRejectsChangedTail(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyRoundTrip(t, externalOwnerSupertypeFixture("Number"), "none", []string{"SuffixOwner$Member"}, "4:external:bound:erasure:identity\n")
	authority := artifact.source.jar.sourceConstructorLowering
	for _, variant := range []string{"control", "concrete argument", "unknown binder", "wrong bound", "wrong physical superclass", "missing signature", "duplicate signature"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var signature *SignatureAttribute
			for _, attr := range obj.Attributes {
				if s, ok := attr.(*SignatureAttribute); ok {
					signature = s
				}
			}
			if signature == nil {
				t.Fatal("fresh generic declaration missing")
			}
			text, known := sourceBridgeUTF8(obj, signature.SignatureIndex)
			if !known || !strings.Contains(text, "LScopeBase<TT;>;") {
				t.Fatal("fresh generic superclass", text)
			}
			switch variant {
			case "concrete argument":
				text = strings.Replace(text, "LScopeBase<TT;>;", "LScopeBase<Ljava/lang/Number;>;", 1)
			case "unknown binder":
				text = strings.Replace(text, "LScopeBase<TT;>;", "LScopeBase<TMissing;>;", 1)
			case "wrong bound":
				text = strings.Replace(text, "T:Ljava/lang/Number;", "T:Ljava/lang/Object;", 1)
			case "wrong physical superclass":
				obj.SuperClass = uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/Object"))
			case "missing signature":
				var kept []AttributeInfo
				for _, attr := range obj.Attributes {
					if attr != signature {
						kept = append(kept, attr)
					}
				}
				obj.Attributes = kept
			case "duplicate signature":
				obj.Attributes = append(obj.Attributes, signature)
			}
			signature.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(text))
			input := obj.Bytes()
			saved := bytes.Clone(input)
			out, err := authority.restoreMemberGenericScope(original, input, nil)
			if !bytes.Equal(input, saved) {
				t.Fatal("caller bytes mutated")
			}
			if variant != "control" {
				if err == nil || out != nil {
					t.Fatal("changed supertype declaration admitted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before := mustParseLowering(t, original)
			after := mustParseLowering(t, out)
			want, valid := nativeMethodLocalOriginalSignature(before, before.Attributes, nil)
			got, complete := nativeMethodLocalOriginalSignature(after, after.Attributes, nil)
			if !valid || !complete || want != got {
				t.Fatal("original supertype signature changed", want, got)
			}
			if again, err := authority.restoreMemberGenericScope(original, out, nil); err != nil || !bytes.Equal(again, out) {
				t.Fatal("scope completion not idempotent", err)
			}
		})
	}
}

// A member's generic superclass can refer to an enclosing binder without
// introducing a member formal. Reconstruct only the member; keep the owner,
// superclass and factory original so an invented fresh binder is observable.
func externalOwnerSupertypeFixture(bound string) string {
	source := "class ScopeBase<X> {ScopeBase(){}}\n" + externalOwnerBoundFixture(bound)
	source = strings.ReplaceAll(source, "class Member {", "class Member extends ScopeBase<T> {")
	source = strings.ReplaceAll(source, "int rows=0;", `
 java.lang.reflect.Type generic=member.getGenericSuperclass();
 if(!(generic instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("missing generic superclass");
 java.lang.reflect.ParameterizedType parent=(java.lang.reflect.ParameterizedType)generic;
 if(parent.getRawType()!=ScopeBase.class||!parent.getActualTypeArguments()[0].equals(SuffixOwner.class.getTypeParameters()[0]))throw new AssertionError("superclass binder identity");
 int rows=0;`)
	return source
}

func TestAdversarialIndependentMemberRetainsGenericSupertypeBinder(t *testing.T) {
	for _, bound := range []string{"Number", "CharSequence"} {
		for _, root := range []string{"SuffixOwner", "AlternateSupertypeScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", bound, root, debug), func(t *testing.T) {
					constructorSuffixFamilyRoundTrip(t, strings.ReplaceAll(externalOwnerSupertypeFixture(bound), "SuffixOwner", root), debug, []string{root + "$Member"}, "4:external:bound:erasure:identity\n")
				})
			}
		}
	}
}

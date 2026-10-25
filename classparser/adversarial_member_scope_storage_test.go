package javaclassparser

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const memberScopeStorageFixture = `class SuffixEffects {static String trace="";static boolean check(long n){trace+="C";return n>=0;}static Object detail(Object v){trace+="M";return v;}}
class SuffixOwner<T> {
 final T token;SuffixOwner(T token){this.token=token;}
 class Member<U> {
  final U value;final java.util.List<T> items;final long seed;
  Member(U value,java.util.List<T> items,long seed){this.value=value;this.items=items;this.seed=seed;}
  U value(){return value;}java.util.List<T> items(){return items;}Object owner(){return SuffixOwner.this.token;}
  long fold(long n){assert SuffixEffects.check(n):SuffixEffects.detail(value);return n*31L+seed;}
 }
}

class SuffixDriver {public static void main(String[]args)throws Exception{
 boolean enabled=SuffixOwner.class.desiredAssertionStatus();int rows=0;
 for(Object token:new Object[]{null,"detail",new IllegalArgumentException("cause")})for(long seed:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){
  SuffixOwner<Object> outer=new SuffixOwner<Object>(token);java.util.List<Object> list=new java.util.ArrayList<Object>();list.add(token);
  SuffixOwner<Object>.Member<Object> member=outer.new Member<Object>(token,list,seed);
  if(member.value()!=token||member.items()!=list||member.owner()!=token)throw new AssertionError("binder / capture identity");
  for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   SuffixEffects.trace="";Throwable error=null;long result=0;try{result=member.fold(n);}catch(Throwable e){error=e;}
   boolean fail=enabled&&n<0;if((error instanceof AssertionError)!=fail||error!=null&&!(error instanceof AssertionError)||!SuffixEffects.trace.equals(enabled?(fail?"CM":"C"):""))throw new AssertionError("assertion owner / evaluation order",error);
   if(fail){if(!String.valueOf(token).equals(error.getMessage())||error.getCause()!=(token instanceof Throwable?token:null))throw new AssertionError("message / cause");}
   else if(result!=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(seed)).longValue())throw new AssertionError("modular arithmetic");rows++;
  }
 }
 Class<?> member=Class.forName("SuffixOwner$Member");java.lang.reflect.Field flag=member.getDeclaredField("$assertionsDisabled");if(flag.getModifiers()!=0x1018||!flag.isSynthetic()||member.getDeclaringClass()!=SuffixOwner.class)throw new AssertionError("original storage / lexical identity");
 System.out.println(rows+":member:scope:storage:effects:identity");
}}
`

func TestAdversarialMemberScopeComposesWithAssertionStorageIdentity(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, own := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			name := "outer-only/" + debug
			fixture := memberScopeStorageFixture
			if own {
				name = "own-and-outer/" + debug
			} else {
				fixture = strings.NewReplacer("Member<U>", "Member", " U ", " T ", "(U ", "(T ", "U value()", "T value()", ".Member<Object>", ".Member", "new Member<Object>", "new Member").Replace(fixture)
			}
			t.Run(name, func(t *testing.T) {
				_, _, artifact := constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{"SuffixOwner$Member"}, "45:member:scope:storage:effects:identity\n")
				if !artifact.source.jar.sourceFieldMetadata.hasAssertionStorage("SuffixOwner$Member") {
					t.Fatal("fixture did not require a published field identity projection")
				}
				if _, ok := artifact.source.jar.sourceConstructorLowering.genericActive["SuffixOwner$Member"]; !ok {
					t.Fatal("fixture did not require a published lexical scope projection")
				}
			})
		}
	}
}

func TestAdversarialConstructorHiddenScopeLiftRequiresBothPublications(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	original, raw, artifact := constructorSuffixFamilyRoundTrip(t, memberScopeStorageFixture, "none", []string{"SuffixOwner$Member"}, "45:member:scope:storage:effects:identity\n")
	base, owner := artifact.source.jar.sourceConstructorLowering, "SuffixOwner$Member"
	for _, variant := range []string{"control", "idempotent", "unreturned scope", "pending scope", "stale scope", "unreturned constructor", "changed own binder", "invented outer argument", "wrong reference argument", "unknown binder", "missing signature", "changed flags", "budget", "canceled", "output budget"} {
		t.Run(variant, func(t *testing.T) {
			a := &sourceConstructorLoweringAuthority{jar: base.jar, genericActive: map[string]constructorNativeSourceOrigin{owner: base.genericActive[owner]}, bindingActive: map[string]constructorNativeSourceOrigin{owner: base.bindingActive[owner]}}
			obj := mustParseLowering(t, raw)
			var ctor *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			if ctor == nil {
				t.Fatal("missing constructor")
			}
			sig, valid := nativeMethodLocalOriginalSignature(obj, ctor.Attributes, nil)
			if !valid || sig != "(LSuffixOwner;TU;Ljava/util/List;J)V" {
				t.Fatal("fixture did not project hidden enclosing binder", sig)
			}
			var work *workbudget.Budget
			switch variant {
			case "unreturned scope":
				a.genericActive = nil
			case "pending scope":
				a.genericPending, a.genericActive = a.genericActive, nil
			case "stale scope":
				o := a.genericActive[owner]
				o.originalHash[0] ^= 1
				a.genericActive[owner] = o
			case "unreturned constructor":
				a.bindingActive = nil
			case "changed own binder":
				sig = "(LSuffixOwner;Ljava/lang/Object;Ljava/util/List;J)V"
			case "invented outer argument":
				sig = "(LSuffixOwner;TU;Ljava/util/List<TU;>;J)V"
			case "wrong reference argument":
				sig = "(LSuffixOwner;TU;Ljava/util/Collection;J)V"
			case "unknown binder":
				sig = "(LSuffixOwner;TUnknown;Ljava/util/List;J)V"
			case "changed flags":
				ctor.AccessFlags |= 1
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "output budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			attrs := ctor.Attributes[:0]
			for _, attr := range ctor.Attributes {
				if s, ok := attr.(*SignatureAttribute); ok {
					if variant == "missing signature" {
						continue
					}
					s.SignatureIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(sig))
				}
				attrs = append(attrs, attr)
			}
			ctor.Attributes = attrs
			input := obj.Bytes()
			if variant == "idempotent" {
				var e error
				input, e = a.restoreConstructorBindingSignatures(original, input, nil)
				if e != nil {
					t.Fatal(e)
				}
			}
			saved := bytes.Clone(input)
			out, err := a.restoreConstructorBindingSignatures(original, input, work)
			if !bytes.Equal(input, saved) {
				t.Fatal("mutated input")
			}
			if variant == "unreturned constructor" {
				if err != nil || !bytes.Equal(out, input) {
					t.Fatal("unpublished edit", err)
				}
				return
			}
			if variant != "control" && variant != "idempotent" {
				if err == nil || out != nil {
					t.Fatal("unproved constructor signature accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before, after := mustParseLowering(t, input), mustParseLowering(t, out)
			for i, m := range before.Methods {
				if !reflect.DeepEqual(constructorLoweringCode(m), constructorLoweringCode(after.Methods[i])) {
					t.Fatal("fresh Code changed")
				}
			}
			if !reflect.DeepEqual(before.ConstantPool, after.ConstantPool[:len(before.ConstantPool)]) {
				t.Fatal("existing constant changed")
			}
		})
	}
}

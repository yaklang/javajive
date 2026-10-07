package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

const modernDefaultContractFixture = `class ContractEffects {static Object published,seenToken;static long seenWord;static boolean fail;static final RuntimeException error=new RuntimeException("identity");}
class ModernContractParent {
 interface Contract {long word(long n);Object token();default long add(long n){return ModernContractChild.Payload.add(word(n),n);}static long mix(long a,long b){return ModernContractChild.Payload.mix(a,b);}}
 static abstract class Base implements Contract {final long supplied;Base(long supplied){ContractEffects.published=this;ContractEffects.seenToken=token();ContractEffects.seenWord=word(0);this.supplied=supplied;if(ContractEffects.fail)throw ContractEffects.error;}}
}
class ModernContractChild {
 private final long offset;ModernContractChild(long offset){this.offset=offset;}
 static final class Payload {static long add(long a,long b){return a+b;}static long mix(long a,long b){return a^b;}}
 ModernContractParent.Base make(final long seed,final Object token){return new ModernContractParent.Base(seed){public long word(long n){return seed+offset+n;}public Object token(){return token;}};}
}
class ContractDriver {public static void main(String[]args)throws Exception{Object identity=new Object();int rows=0;for(long offset:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(boolean fail:new boolean[]{false,true}){ContractEffects.published=null;ContractEffects.fail=fail;ModernContractParent.Base value;try{value=new ModernContractChild(offset).make(seed,token);if(fail)throw new AssertionError("missing failure");}catch(RuntimeException e){if(!fail||e!=ContractEffects.error)throw new AssertionError("error identity",e);value=(ModernContractParent.Base)ContractEffects.published;}long observed=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(offset)).longValue();long expected=java.math.BigInteger.valueOf(observed).add(java.math.BigInteger.valueOf(n)).longValue();long sum=java.math.BigInteger.valueOf(expected).add(java.math.BigInteger.valueOf(n)).longValue();if(value==null||value!=ContractEffects.published||value.supplied!=seed||value.token()!=token||ContractEffects.seenToken!=token||ContractEffects.seenWord!=observed||value.word(n)!=expected||value.add(n)!=sum||ModernContractParent.Contract.mix(seed,n)!=(seed^n))throw new AssertionError("default/static interface, original nest capture, overflow");rows++;}if(!ModernContractParent.Contract.class.getDeclaredMethod("add",long.class).isDefault()||!java.lang.reflect.Modifier.isStatic(ModernContractParent.Contract.class.getModifiers())||ModernContractParent.Contract.class.getDeclaringClass()!=ModernContractParent.class)throw new AssertionError("original declaration flags");System.out.println(rows+":default:static:cyclic:pre-super:identity:word");}}
`

// The two source owners form a dependency cycle through a default/static
// interface and an anonymous constructor. Each original is run first; the
// rebuilt runtime contains no original owner classes. The driver checks both
// returned words and observable pre-super captures/publication/failure identity.
func TestAdversarialModernInterfaceCyclicFamilyRoundTrip(t *testing.T) {
	for _, release := range []string{"8", "9", "10", "11"} {
		t.Run("input"+release, func(t *testing.T) {
			targets := []int{11}
			if release == "8" {
				targets = []int{8}
			}
			if release == "11" {
				targets = []int{11, 16}
			}
			testSourceTargetReleaseFamilyFixture(t, modernDefaultContractFixture, "ModernContract", "ContractDriver", "300:default:static:cyclic:pre-super:identity:word\n", release, targets)
		})
	}
}

func TestNativeMemberInterfaceMethodsRequireSupportedOriginalVersion(t *testing.T) {
	const fixture = `class VersionContractOwner {interface Contract {default long sum(long a,long b){return a+b;}static long mix(long a,long b){return a^b;}}}`
	for _, release := range []string{"8", "9", "10", "11"} {
		files := nativeCompileReleaseClasses(t, fixture, "none", release)
		for _, variant := range []string{"original", "pre-default version", "future version", "preview", "unknown constant", "private default", "missing code", "duplicate code", "budget", "canceled"} {
			t.Run("input"+release+"/"+variant, func(t *testing.T) {
				obj, e := Parse(append([]byte(nil), files["VersionContractOwner$Contract.class"]...))
				if e != nil {
					t.Fatal(e)
				}
				var method *MemberInfo
				for _, m := range obj.Methods {
					name, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if name == "sum" {
						method = m
					}
				}
				if method == nil {
					t.Fatal("original default method")
				}
				var work *workbudget.Budget
				switch variant {
				case "pre-default version":
					obj.MajorVersion = 51
				case "future version":
					obj.MajorVersion = 56
				case "preview":
					obj.MinorVersion = 65535
				case "unknown constant":
					obj.ConstantPool = append(obj.ConstantPool, &ConstantModuleInfo{})
				case "private default":
					method.AccessFlags = 2
				case "missing code":
					method.Attributes = nil
				case "duplicate code":
					method.Attributes = append(method.Attributes, &CodeAttribute{})
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				got := nativeMemberDeclarationKindRepresentable(obj, 0x608, work)
				if got != (variant == "original") {
					t.Fatalf("interface declaration admitted=%v", got)
				}
			})
		}
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Older valid classfile profiles have no MethodParameters attribute and can
// omit FINAL from an anonymous enum self row while the actual class is final.
// The original JVM runs first: source representation cannot stand in for behavior.
func TestAdversarialLegacyEnumConstantMetadataKeepsExecutableOwnershipRoundTrip(t *testing.T) {
	for _, version := range []uint16{49, 50, 51} {
		for _, owner := range []string{"LegacyEnumOwner", "RenamedLegacyEnumOwner"} {
			for _, selfFlags := range []uint16{0x4000, 0x4008} {
				t.Run(fmt.Sprintf("%d/%s/self-%x", version, owner, selfFlags), func(t *testing.T) {
					fixture := `class LegacyEnumEffects{static String trace="";static final RuntimeException error=new RuntimeException("parent identity");static boolean fail;}
class LegacyEnumOwner{int seed;LegacyEnumOwner(int n){seed=n;}enum Mode{FIRST(7,0.25){long apply(long n){return n+step;}},LAST(11,-0.0){long apply(long n){return -n+step;}};final long step;final double bias;private Mode(long n,double d){LegacyEnumEffects.trace+="E";step=n;bias=d;}abstract long apply(long n);}final class View extends LegacyEnumBase{View(long n,Object t){super(n,t);LegacyEnumEffects.trace+="C";}int value(){return LegacyEnumOwner.this.seed;}}View make(long n,Object t){return new View(n,t);}}
class LegacyEnumBase{final long wide;final Object token;final int observed;LegacyEnumBase(long n,Object t){LegacyEnumEffects.trace+="P";wide=n;token=t;observed=value();if(LegacyEnumEffects.fail)throw LegacyEnumEffects.error;}int value(){return 0;}}
class LegacyEnumDriver{public static void main(String[]args){LegacyEnumOwner.Mode a=LegacyEnumOwner.Mode.FIRST,b=LegacyEnumOwner.Mode.LAST;if(!LegacyEnumEffects.trace.equals("EE")||a.ordinal()!=0||b.ordinal()!=1||a.getDeclaringClass()!=LegacyEnumOwner.Mode.class||a.step!=7||b.step!=11||Double.doubleToRawLongBits(b.bias)!=0x8000000000000000L)throw new AssertionError("enum words/order/raw bits");Object token=new Object();int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object t:new Object[]{null,token})for(boolean fail:new boolean[]{false,true}){long wide=((long)n)*0x100000001L;long first=java.math.BigInteger.valueOf(wide).add(java.math.BigInteger.valueOf(7)).longValue(),last=java.math.BigInteger.valueOf(wide).negate().add(java.math.BigInteger.valueOf(11)).longValue();if(a.apply(wide)!=first||b.apply(wide)!=last)throw new AssertionError("independent enum dispatch/overflow");LegacyEnumOwner.Mode[]copy=LegacyEnumOwner.Mode.values();copy[0]=null;if(LegacyEnumOwner.Mode.values()[0]!=a||LegacyEnumOwner.Mode.valueOf("LAST")!=b)throw new AssertionError("enum factories/identity");LegacyEnumOwner root=new LegacyEnumOwner(n);LegacyEnumEffects.trace="";LegacyEnumEffects.fail=fail;try{LegacyEnumOwner.View view=root.make(wide,t);if(fail||view.observed!=n||view.wide!=wide||view.token!=t||!LegacyEnumEffects.trace.equals("PC"))throw new AssertionError("complete sibling constructor callback");root.seed+=31;if(view.value()!=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(31)).intValue())throw new AssertionError("live capture");LegacyEnumEffects.trace="";LegacyEnumOwner.View second=root.make(wide,t);if(second==view||second.observed!=root.seed||!LegacyEnumEffects.trace.equals("PC"))throw new AssertionError("allocation identity/callback");}catch(RuntimeException error){if(!fail||error!=LegacyEnumEffects.error||!LegacyEnumEffects.trace.equals("P"))throw new AssertionError("parent failure identity/effects",error);}rows++;}System.out.println(rows+":legacy-enum:dispatch:owned-sibling:callback:identity:effects");}}`
					fixture = strings.ReplaceAll(fixture, "LegacyEnumOwner", owner)
					testNativePrivateSetterCompiledFixture(t, owner, "LegacyEnumDriver", "20:legacy-enum:dispatch:owned-sibling:callback:identity:effects\n", func(t *testing.T, debug string) map[string][]byte {
						files := nativeCompileDebugClasses(t, fixture, debug)
						for name, raw := range files {
							obj, err := Parse(append([]byte(nil), raw...))
							if err != nil {
								t.Fatal(err)
							}
							obj.MajorVersion = version
							for _, m := range obj.Methods {
								attrs := []AttributeInfo{}
								for _, attr := range m.Attributes {
									if a, ok := attr.(*UnparsedAttribute); ok && a.Name == "MethodParameters" {
										continue
									}
									attrs = append(attrs, attr)
								}
								m.Attributes = attrs
							}
							for _, attr := range obj.Attributes {
								if table, ok := attr.(*InnerClassesAttribute); ok {
									for _, row := range table.Classes {
										if row.InnerNameIndex == 0 && row.InnerClassAccessFlags == 0x4010 {
											row.InnerClassAccessFlags = selfFlags
										}
									}
								}
							}
							files[name] = obj.Bytes()
						}
						return files
					})
				})
			}
		}
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

const legacyLocalMetadataFixture = `interface LegacyLocalValue{long number(long delta);double fraction();Object token();}
class LegacyLocalOwner{public long bias;LegacyLocalOwner(long n){bias=n;}LegacyLocalValue make(final long seed,final double fraction,final Object token){class Entry implements LegacyLocalValue{public long number(long delta){return bias+seed+delta;}public double fraction(){return fraction;}public Object token(){return token;}}return new Entry();}static LegacyLocalValue stat(final long seed,final double fraction,final Object token){class Node implements LegacyLocalValue{public long number(long delta){return seed-delta;}public double fraction(){return fraction;}public Object token(){return token;}}return new Node();}}
class LegacyLocalDriver{public static void main(String[]args){int rows=0;Object identity=new Object();for(long seed:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE})for(double fraction:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY})for(Object token:new Object[]{null,identity}){LegacyLocalOwner owner=new LegacyLocalOwner(31);LegacyLocalValue a=owner.make(seed,fraction,token),b=LegacyLocalOwner.stat(seed,fraction,token);owner.bias=-77;long delta=seed^0x12345678L;if(a.number(delta)!=java.math.BigInteger.valueOf(-77).add(java.math.BigInteger.valueOf(seed)).add(java.math.BigInteger.valueOf(delta)).longValue()||b.number(delta)!=java.math.BigInteger.valueOf(seed).subtract(java.math.BigInteger.valueOf(delta)).longValue()||Double.doubleToRawLongBits(a.fraction())!=Double.doubleToRawLongBits(fraction)||Double.doubleToRawLongBits(b.fraction())!=Double.doubleToRawLongBits(fraction)||a.token()!=token||b.token()!=token||a==b)throw new AssertionError("capture/binding/live owner/raw words/identity");for(LegacyLocalValue v:new LegacyLocalValue[]{a,b}){Class<?>c=v.getClass();if(!c.isLocalClass()||c.isAnonymousClass()||c.getEnclosingClass()!=LegacyLocalOwner.class||c.getDeclaringClass()!=null||!c.getEnclosingMethod().getName().equals(v==a?"make":"stat")||!c.getSimpleName().equals(v==a?"Entry":"Node"))throw new AssertionError("method ownership");}rows++;}System.out.println(rows+":legacy-local:binding:live-owner:raw-words:identity");}}
`

func TestAdversarialLegacyLocalMetadataKeepsExecutableCaptureBindingsRoundTrip(t *testing.T) {
	for _, version := range []uint16{49, 50, 51} {
		for _, owner := range []string{"LegacyLocalOwner", "RenamedLocalScope"} {
			for _, keepSignature := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/signature=%v", version, owner, keepSignature), func(t *testing.T) {
					source := strings.ReplaceAll(legacyLocalMetadataFixture, "LegacyLocalOwner", owner)
					testNativePrivateSetterCompiledFixture(t, owner, "LegacyLocalDriver", "24:legacy-local:binding:live-owner:raw-words:identity\n", func(t *testing.T, debug string) map[string][]byte {
						files := nativeCompileDebugClasses(t, source, debug)
						for name, raw := range files {
							obj, err := Parse(append([]byte(nil), raw...))
							if err != nil {
								t.Fatal(err)
							}
							obj.MajorVersion = version
							for _, m := range obj.Methods {
								keep := []AttributeInfo{}
								methodName, _ := sourceBridgeUTF8(obj, m.NameIndex)
								for _, a := range m.Attributes {
									if _, ok := a.(*SignatureAttribute); ok && !keepSignature && methodName == "<init>" {
										continue
									}
									if p, ok := a.(*UnparsedAttribute); ok && p.Name == "MethodParameters" {
										continue
									}
									keep = append(keep, a)
								}
								m.Attributes = keep
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

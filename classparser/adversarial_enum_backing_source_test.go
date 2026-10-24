package javaclassparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// This is a source-generation obligation in each compiler's own domain. A
// native-to-modern artifact has the separate, still strict helper-method ABI
// obligation; these controls never authorize adding or discarding a method.
func testEnumBackingSourcePacket(t *testing.T, fixture, want string) {
	t.Helper()
	for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
		t.Run(policy, func(t *testing.T) {
			if policy == "no-source-rewrites" {
				t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
			}
			if policy == "no-core-cleanups" {
				t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
			}
			testArtifactLambdaMetadataCompilerDomains(t, fixture, []bool{false, true}, want, false, func(t *testing.T, original, rebuilt []byte) {
				if a, b := enumBackingAllocationEvents(t, original), enumBackingAllocationEvents(t, rebuilt); a != 1 || b != a {
					t.Fatalf("own enum backing allocation events: original=%d rebuilt=%d", a, b)
				}
			})
		})
	}
}

// Counting actual fresh ANEWARRAY sites catches a leaked private DUP temp even
// when values(), its clone, and all ordinary numerical results still agree.
func enumBackingAllocationEvents(t *testing.T, raw []byte) int {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, method := range obj.Methods {
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			for _, op := range constructorMotionOps(decoder) {
				if nativeEnumOpcode(op, core.OP_ANEWARRAY) && len(op.Data) == 2 {
					owner, known := sourceBridgeClassName(obj, core.Convert2bytesToInt(op.Data))
					if known && owner == obj.GetClassName() {
						count++
					}
				}
			}
		}
	}
	return count
}

const enumBackingSourceFixture = `class EnumBackingEffects{static String trace="";}
public enum ArtifactLambdaOwner {
 LEFT(3),RIGHT(7);public final int tag;
 private ArtifactLambdaOwner(int value){tag=value;EnumBackingEffects.trace+="C"+value;}
 static{EnumBackingEffects.trace+="S";}
 public long apply(long value,int width){long mask=(1L<<width)-1L;return (this==LEFT?(value&mask):(value^mask))+tag;}
}
class ArtifactLambdaDriver {
 public static void main(String[]args){EnumBackingEffects.trace="";ArtifactLambdaOwner[] original=ArtifactLambdaOwner.values();
  if(!EnumBackingEffects.trace.equals("C3C7S")||original.length!=2||original[0]!=ArtifactLambdaOwner.LEFT||original[1]!=ArtifactLambdaOwner.RIGHT||original[0].ordinal()!=0||original[1].ordinal()!=1)throw new AssertionError("constant/backing initialization/order");
  ArtifactLambdaOwner[] fresh=ArtifactLambdaOwner.values();fresh[0]=null;if(ArtifactLambdaOwner.values()[0]!=original[0]||fresh==original)throw new AssertionError("backing copy ownership");int rows=0;
  for(ArtifactLambdaOwner mode:original)for(long value:new long[]{Long.MIN_VALUE,-1L,0L,1L,Long.MAX_VALUE})for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,7,31,32,63,64,65,70,Integer.MAX_VALUE}){
   java.math.BigInteger mask=java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE);java.math.BigInteger number=java.math.BigInteger.valueOf(value);
   long expected=(mode==original[0]?number.and(mask):number.xor(mask)).add(java.math.BigInteger.valueOf(mode==original[0]?3L:7L)).longValue();if(mode.apply(value,width)!=expected||ArtifactLambdaOwner.valueOf(mode.name())!=mode)throw new AssertionError("modular enum algorithm/binding");rows++;
  }
  Throwable error=null;try{ArtifactLambdaOwner.valueOf("absent");}catch(Throwable e){error=e;}if(!(error instanceof IllegalArgumentException))throw new AssertionError("enum lookup failure");
  System.out.println(rows+":enum:backing:order:identity:algorithm");
 }
}`

func TestAdversarialEnumBackingSourcePacketRetainsAllocationAndAlgorithm(t *testing.T) {
	testEnumBackingSourcePacket(t, enumBackingSourceFixture, "120:enum:backing:order:identity:algorithm\n")
}

func TestAdversarialEnumBackingEmptyAndLargeSourcePackets(t *testing.T) {
	for _, count := range []int{0, 1, 129} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			names := make([]string, count)
			for i := range names {
				names[i] = fmt.Sprintf("C%d", i)
			}
			fixture := `class EnumArityEffects{static String after;}
public enum ArtifactLambdaOwner {` + strings.Join(names, ",") + `;
 static {EnumArityEffects.after="S";}
 public int code(){return ordinal();}
}
class ArtifactLambdaDriver {public static void main(String[]args){
 ArtifactLambdaOwner[] values=ArtifactLambdaOwner.values();
 if(values.length!=COUNT||!EnumArityEffects.after.equals("S"))throw new AssertionError("enum empty/large initialization");
 for(int i=0;i<values.length;i++){if(values[i].ordinal()!=i||values[i].code()!=i||!values[i].name().equals("C"+i)||ArtifactLambdaOwner.valueOf("C"+i)!=values[i])throw new AssertionError("enum exact declaration order/lookup/identity");}
 if(values.length>0){ArtifactLambdaOwner first=values[0];values[0]=null;if(ArtifactLambdaOwner.values()[0]!=first)throw new AssertionError("values clone ownership");}
 System.out.println("COUNT:enum:arity:identity:order");
}}`
			testEnumBackingSourcePacket(t, strings.ReplaceAll(fixture, "COUNT", fmt.Sprint(count)), fmt.Sprintf("%d:enum:arity:identity:order\n", count))
		})
	}
}

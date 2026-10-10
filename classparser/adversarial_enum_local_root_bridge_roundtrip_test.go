package javaclassparser

import (
	"strings"
	"testing"
)

// A newly admitted enum local must not hide a missing constructor projection
// in another method of the same family. The anonymous factory is also the
// compiler's private-constructor marker; there are no named member classes.
const enumLocalRootBridgeFixture = `enum FactoryChoice{FIRST,SECOND,THIRD}
interface RootFactory{FactoryRoot create(int value);}
class FactoryRoot{static int made;private final int value;private FactoryRoot(int value){made++;this.value=value;}
 static RootFactory factory(final int offset){return new RootFactory(){public FactoryRoot create(int value){return new FactoryRoot(value+offset);}};}
 int pick(FactoryChoice input){FactoryChoice local=input;switch(local){case SECOND:return value-1;case FIRST:return value+1;default:return value;}}}
class FactoryRootDriver{public static void main(String[]args){int rows=0;RootFactory factory=FactoryRoot.factory(17);for(int value:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){int before=FactoryRoot.made;FactoryRoot root=factory.create(value);if(FactoryRoot.made!=before+1)throw new AssertionError("constructor count");for(FactoryChoice choice:FactoryChoice.values()){java.math.BigInteger want=java.math.BigInteger.valueOf(value).add(java.math.BigInteger.valueOf(17));want=java.math.BigInteger.valueOf(want.intValue());if(choice==FactoryChoice.FIRST)want=want.add(java.math.BigInteger.ONE);else if(choice==FactoryChoice.SECOND)want=want.subtract(java.math.BigInteger.ONE);if(root.pick(choice)!=want.intValue())throw new AssertionError("factory/enum binding/overflow");rows++;}try{root.pick(null);throw new AssertionError("null");}catch(NullPointerException expected){}}System.out.println(rows+":enum:local:private:root:anonymous:factory");}}`

func TestAdversarialEnumLocalRootBridgeRetainsPrivateAnonymousFactory(t *testing.T) {
	testNativeIndependentFamilyFixture(t, enumLocalRootBridgeFixture, []string{"FactoryRoot"}, "FactoryRootDriver", "15:enum:local:private:root:anonymous:factory\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalRootBridgeRetainsNestedAllocationIdentity(t *testing.T) {
	fixture := strings.Replace(enumLocalRootBridgeFixture, `new FactoryRoot(value+offset)`, `new FactoryRoot(new FactoryRoot(value+offset).value)`, 1)
	fixture = strings.Replace(fixture, `made!=before+1`, `made!=before+2`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"FactoryRoot"}, "FactoryRootDriver", "15:enum:local:private:root:anonymous:factory\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalRootBridgePreservesWideDescriptor(t *testing.T) {
	fixture := strings.Replace(enumLocalRootBridgeFixture, `private FactoryRoot(int value){made++;this.value=value;}`, `private FactoryRoot(long value){made++;this.value=(int)value;}private FactoryRoot(String decoy){throw new AssertionError("wrong overload");}`, 1)
	fixture = strings.Replace(fixture, `new FactoryRoot(value+offset)`, `new FactoryRoot((long)value+offset)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"FactoryRoot"}, "FactoryRootDriver", "15:enum:local:private:root:anonymous:factory\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalRootBridgeEvaluatesArgumentsOnce(t *testing.T) {
	fixture := strings.Replace(enumLocalRootBridgeFixture, `static int made;`, `static int made;static int evaluated;static int argument(int value){evaluated++;return value;}`, 1)
	fixture = strings.Replace(fixture, `new FactoryRoot(value+offset)`, `new FactoryRoot(FactoryRoot.argument(value+offset))`, 1)
	fixture = strings.Replace(fixture, `int before=FactoryRoot.made;`, `int before=FactoryRoot.made;int effects=FactoryRoot.evaluated;`, 1)
	fixture = strings.Replace(fixture, `if(FactoryRoot.made!=before+1)`, `if(FactoryRoot.evaluated!=effects+1)throw new AssertionError("argument count/order");if(FactoryRoot.made!=before+1)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"FactoryRoot"}, "FactoryRootDriver", "15:enum:local:private:root:anonymous:factory\n", nativeLexicalExactSignatures)
}

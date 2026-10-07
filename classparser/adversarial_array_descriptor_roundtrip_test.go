package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Array dimensions are a runtime descriptor bound, not generic nesting. The
// unchanged driver observes the return type, value and real source ownership;
// merely producing compilable flat classes cannot satisfy this oracle.
func TestAdversarialRuntimeArrayDescriptorsKeepTypesAndOwnershipRoundTrip(t *testing.T) {
	for _, dimensions := range []int{1, 128, 129, 254, 255} {
		t.Run(fmt.Sprint(dimensions), func(t *testing.T) {
			array := "ForeignArrayScope.Value" + strings.Repeat("[]", dimensions)
			fixture := fmt.Sprintf(`class ArrayDescriptorOwner{static class Anchor{}static %s echo(%s value){return value;}}
class ForeignArrayScope{static class Value{}}
class ArrayDescriptorDriver{public static void main(String[]args)throws Exception{if(ArrayDescriptorOwner.echo(null)!=null)throw new AssertionError("null array value");Class<?> type=null;for(java.lang.reflect.Method m:ArrayDescriptorOwner.class.getDeclaredMethods())if(m.getName().equals("echo")){if(type!=null||m.getParameterTypes().length!=1||m.getParameterTypes()[0]!=m.getReturnType())throw new AssertionError("descriptor identity");type=m.getReturnType();}int dims=0;while(type!=null&&type.isArray()){dims++;type=type.getComponentType();}if(dims!=%d||type!=ForeignArrayScope.Value.class)throw new AssertionError("dimensions and component type");if(ArrayDescriptorOwner.Anchor.class.getDeclaringClass()!=ArrayDescriptorOwner.class||ArrayDescriptorOwner.class.getDeclaredClasses().length!=1)throw new AssertionError("original declaring owner");System.out.println("array:descriptor:%d:ownership");}}`, array, array, dimensions, dimensions)
			testSourceTargetReleaseFamilyFixture(t, fixture, "ArrayDescriptorOwner", "ArrayDescriptorDriver", fmt.Sprintf("array:descriptor:%d:ownership\n", dimensions), "8", []int{8})
		})
	}
}

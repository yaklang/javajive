package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialStaticOwnersKeepTypeAndPackageNamespaces(t *testing.T) {
	for _, spelling := range []string{"ApiProducer", "NumericOwner", "Catalog"} {
		for _, scope := range []string{"method formals", "class formals", "package root type"} {
			for _, kind := range []string{"class", "interface"} {
				t.Run(spelling+"/"+scope+"/"+kind, func(t *testing.T) {
					api := fmt.Sprintf(`package owners;public %s ApiProducer{public static int compute(int x){OwnerState.calls++;if(x<0)throw OwnerState.failure;return x*17+5;}public static long compute(long x){throw new AssertionError("wrong overload");}}`, kind)
					source := `import static owners.ApiProducer.compute;class TypeShadowProbe{static <ApiProducer,owners> int m(int x){return compute(x);}}
public class TypeShadowDriver{public static void main(String[]args){int rows=0;for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,17,Integer.MAX_VALUE}){owners.OwnerState.calls=0;try{int got=TypeShadowProbe.m(x);if(x<0||got!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("value/overload");}catch(RuntimeException e){if(x>=0||e!=owners.OwnerState.failure)throw new AssertionError("failure identity");}if(owners.OwnerState.calls!=1)throw new AssertionError("effects/once");rows++;}System.out.println(rows+":type-namespace:values:effects:failure");}}`
					if scope == "class formals" {
						source = strings.Replace(source, "class TypeShadowProbe{static <ApiProducer,owners> int m", "class TypeShadowProbe<ApiProducer,owners>{int m", 1)
						source = strings.Replace(source, "TypeShadowProbe.m(x)", "new TypeShadowProbe<Object,Object>().m(x)", 1)
					}
					if scope == "package root type" {
						source = strings.Replace(source, "class TypeShadowProbe{static <ApiProducer,owners>", "class owners{static class ApiProducer{static int compute(int x){throw new AssertionError(\"wrong package-root type\");}}}class TypeShadowProbe{static <ApiProducer>", 1)
						source = strings.ReplaceAll(source, "owners.OwnerState", "OwnerState")
						source = "import owners.OwnerState;" + source
					}
					source = strings.ReplaceAll(source, "ApiProducer", spelling)
					api = strings.ReplaceAll(api, "ApiProducer", spelling)
					roundTripGenericFlowSources(t, "TypeShadowDriver", source, map[string]string{"owners/" + spelling + ".java": api, "owners/OwnerState.java": `package owners;public class OwnerState{public static int calls;public static final RuntimeException failure=new RuntimeException("same");}`}, nil, []string{"TypeShadowProbe"}, true, Precision, Compatibility, "legacy")
				})
			}
		}
	}
}

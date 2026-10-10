package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerMultidimensionalArrayRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", "int[][]")
	f = strings.Replace(f, `new byte[ArrayInitTrace.dimension(input)]`, `new int[ArrayInitTrace.dimension(input)][ArrayInitTrace.dimension(input+1)]`, 1)
	f = strings.Replace(f, `java.lang.reflect.Array.getByte(p.view(),i)!=0`, `!(java.lang.reflect.Array.get(p.view(),i) instanceof int[])||((int[])java.lang.reflect.Array.get(p.view(),i)).length!=n+1`, 1)
	f = strings.Replace(f, `!ArrayInitTrace.trace.equals("PD")`, `!ArrayInitTrace.trace.equals("PDD")`, 1)
	f = strings.Replace(f, `!ArrayInitTrace.trace.equals("PD")`, `!ArrayInitTrace.trace.equals(n==-1?"PDD":"PD")`, 1)
	testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}
func TestNativeAnonymousInitializerZeroOuterStillChecksNegativeInnerDimension(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", "int[][]")
	f = strings.Replace(f, `new byte[ArrayInitTrace.dimension(input)]`, `new int[0][ArrayInitTrace.dimension(input)]`, 1)
	f = strings.Replace(f, `p.size()!=n||java.lang.reflect.Array.getLength(p.view())!=n`, `p.size()!=0||java.lang.reflect.Array.getLength(p.view())!=0`, 1)
	f = strings.Replace(f, `i<n`, `i<java.lang.reflect.Array.getLength(p.view())`, 1)
	f = strings.Replace(f, `java.lang.reflect.Array.getByte(p.view(),i)!=0`, `java.lang.reflect.Array.get(p.view(),i)!=null`, 1)
	testNativePrivateSetterFixture(t, f, "ArrayInitOwner", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}
func TestNativeAnonymousInitializerPartialMultidimensionalRankRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousArrayInitializerFixture, "byte[]", "int[][][]")
	f = strings.Replace(f, `new byte[ArrayInitTrace.dimension(input)]`, `new int[2][ArrayInitTrace.dimension(input)][]`, 1)
	f = strings.Replace(f, `p.size()!=n||java.lang.reflect.Array.getLength(p.view())!=n`, `p.size()!=2||java.lang.reflect.Array.getLength(p.view())!=2`, 1)
	f = strings.Replace(f, `i<n`, `i<java.lang.reflect.Array.getLength(p.view())`, 1)
	f = strings.Replace(f, `java.lang.reflect.Array.getByte(p.view(),i)!=0`, `!(java.lang.reflect.Array.get(p.view(),i) instanceof int[][])||((int[][])java.lang.reflect.Array.get(p.view(),i)).length!=n`, 1)
	f = strings.ReplaceAll(f, "ArrayInitOwner", "IndependentPartialRankScope")
	testNativePrivateSetterFixture(t, f, "IndependentPartialRankScope", "ArrayInitDriver", "5:array:dimension:identity:default:partial:order\n")
}

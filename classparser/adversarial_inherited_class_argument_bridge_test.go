package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// Class binders inside parameterized arguments disappear at erasure; direct
// type-variable ports do not. Both caller-instantiated and declaration-owned
// types must keep the original array identity and exact special invocation.
func TestAdversarialInheritedBridgeClassArgumentsKeepErasureInvariantPorts(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, shape := range []string{"list", "map"} {
		for _, root := range []string{"BridgeOuter", "DistinctBoundOwner"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("%s/%s/%s", shape, root, debug), func(t *testing.T) {
					fixture := inheritedClassArgumentBridgeFixture(shape, "C", root)
					testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, false, "36:inherited:covariant:bridge:dispatch:wide:exception\n")
				})
			}
		}
	}
}

func inheritedClassArgumentBridgeFixture(shape, variable, root string) string {
	applied, bound, erased, value := "java.util.List<String>", "java.util.List<"+variable+">", "java.util.List", `new java.util.List[]{java.util.Arrays.asList("token")}`
	if shape == "map" {
		applied, bound, erased, value = "java.util.Map<String,String>", "java.util.Map<String,"+variable+">", "java.util.Map", `new java.util.Map[]{java.util.Collections.singletonMap("key","token")}`
	}
	fixture := inheritedVarargsBridgeFixture(applied, erased, value, "BridgeOuter")
	formal := "<" + variable + " extends Comparable<" + variable + ">>"
	fixture = strings.Replace(fixture, "class BridgeBase {", "class BridgeBase"+formal+" {", 1)
	fixture = strings.Replace(fixture, "public class BridgeOuter extends BridgeBase {", "public class BridgeOuter"+formal+" extends BridgeBase<"+variable+"> {", 1)
	fixture = strings.Replace(fixture, "static class Leaf extends BridgeOuter", "static class Leaf extends BridgeOuter<String>", 1)
	fixture = strings.ReplaceAll(fixture, applied+"[] token", bound+"[] token")
	fixture = strings.ReplaceAll(fixture, applied+"... token", bound+"... token")
	return strings.ReplaceAll(fixture, "BridgeOuter", root)
}

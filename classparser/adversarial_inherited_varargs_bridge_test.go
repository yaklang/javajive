package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// A varargs declaration changes source invocation applicability, not the
// physical array argument passed by an existing typed super bridge. Preserve
// its original array identity, wide slots, special dispatch and failure object.
func TestAdversarialInheritedVarargsBridgeKeepsArrayAndSuperBinding(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, sample := range []struct{ element, erased, value string }{
		{"Object", "Object", "new Object[]{new Object(),null}"},
		{"long", "long", "new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}"},
		{"Object[]", "Object[]", "new Object[][]{new Object[]{new Object()},null}"},
		{"java.lang.Class<?>", "java.lang.Class", "new java.lang.Class<?>[]{String.class,int.class}"},
		{"java.util.List<String>", "java.util.List", "new java.util.List[]{java.util.Arrays.asList(\"token\")}"},
	} {
		element := sample.element
		for _, root := range []string{"BridgeOuter", "RenamedArrayBridge"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				for _, parameters := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/parameters%v", element, root, debug, parameters), func(t *testing.T) {
						fixture := inheritedVarargsBridgeFixture(element, sample.erased, sample.value, root)
						testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, parameters, "36:inherited:covariant:bridge:dispatch:wide:exception\n")
					})
				}
			}
		}
	}
}

func inheritedVarargsBridgeFixture(element, erased, value, root string) string {
	array := element + "[]"
	fixture := strings.Replace(inheritedCovariantBridgeFixture, `for(Object v:new Object[]{null,"token"})`, "for("+array+" v:new "+erased+"[][]{null,"+value+"})", 1)
	fixture = strings.Replace(fixture, "static int calls;", "static int calls;static Object received;", 1)
	fixture = strings.Replace(fixture, "BridgeEffects.calls++;", "BridgeEffects.calls++;BridgeEffects.received=token;", 1)
	fixture = strings.Replace(fixture, "if(caught!=fail||BridgeEffects.calls!=1)", "if(BridgeEffects.received!=v||caught!=fail||BridgeEffects.calls!=1)", 1)
	fixture = strings.ReplaceAll(fixture, "Object token", array+" token")
	fixture = strings.ReplaceAll(fixture, "Object v)", array+" v)")
	fixture = strings.ReplaceAll(fixture, "double.class,Object.class", "double.class,"+erased+"[].class")
	fixture = strings.Replace(fixture, "public String read(long n,double real,"+array+" token)", "public String read(long n,double real,"+element+"... token)", 1)
	fixture = strings.ReplaceAll(fixture, "BridgeOuter", root)
	return fixture
}

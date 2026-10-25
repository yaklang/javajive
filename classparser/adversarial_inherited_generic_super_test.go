package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The check performed by an inherited bridge can be narrower than the erased
// superclass parameter. A Comparable is not necessarily a Marker: preserving
// only the invocation descriptor loses the original ClassCastException edge.
func inheritedGenericSuperFixture(t *testing.T) string {
	t.Helper()
	s := strings.NewReplacer(
		"interface BridgePort<T>{T exchange", "interface BridgePort<T>{String exchange",
		"public class BridgeOuter implements BridgePort<String>{", "interface BridgeMarker<T> extends Comparable<T>{}\nclass BridgeValue implements BridgeMarker<BridgeValue>{public int compareTo(BridgeValue x){return 0;}public String toString(){return \"token\";}}\npublic class BridgeOuter<X extends Comparable<X>> implements BridgePort<X>{",
		"static class Leaf extends BridgeOuter{", "static class Leaf<X extends BridgeMarker<X>> extends BridgeOuter<X>{",
		"new Leaf()", "new Leaf<BridgeValue>()",
		"String value,String[] array", "X value,X[] array",
		"extends BridgeOuter.Leaf{", "extends BridgeOuter.Leaf<BridgeValue>{",
		"m.getReturnType()!=Object.class", "m.getReturnType()!=String.class",
		"methodType(Object.class,", "methodType(String.class,",
		"return (Object)h.bindTo(this).invokeExact", "return (String)h.bindTo(this).invokeExact",
		`new Object[]{null,"",new String("word"),new Object()}`, `new Object[]{null,new BridgeValue(),"comparable but not marker",new Object()}`,
		`new Object[][]{null,new String[]{null,"item"},new Object[]{"item"},new Integer[]{1}}`, `new Object[][]{null,new BridgeValue[]{null,new BridgeValue()},new Comparable[]{"item"},new Integer[]{1}}`,
		"v instanceof String", "v instanceof BridgeMarker",
		"a instanceof String[]", "a instanceof BridgeMarker[]",
		":inherited:argument:casts:wide:dispatch:exception", ":inherited:generic:super:checks:dispatch",
	).Replace(inheritedArgumentBridgeFixture)
	// The hostile override is more specialized than Leaf's physical bridge.
	// findSpecial must still reach the original superclass exactly once.
	at := strings.Index(s, "class BridgeDerived")
	if at < 0 {
		t.Fatal("fixture derived declaration")
	}
	s = s[:at] + strings.Replace(s[at:], "X value,X[] array", "BridgeValue value,BridgeValue[] array", 1)
	return s
}

func TestAdversarialInheritedGenericSuperBridgeKeepsNarrowChecks(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, root := range []string{"BridgeOuter", "RenamedGenericOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(inheritedGenericSuperFixture(t), "BridgeOuter", root)
				testIndependentInheritedBridgeWithOriginal(t, modern, java, native, fixture, root, debug, false, "288:inherited:generic:super:checks:dispatch\n", nil, testInheritedGenericBridgeFreshClosure)
			})
		}
	}
}

// Static method formals have a real lexical declaration even though an
// anonymous object captures no enclosing instance. Type binding must not depend
// on the unrelated constructor protocol that restores enclosing-instance ports.
func inheritedAnonymousGenericSuperFixture(t *testing.T) string {
	s := inheritedGenericSuperFixture(t)
	s = strings.Replace(s, " static class Leaf<X extends BridgeMarker<X>> extends BridgeOuter<X>{public Leaf(){}}", "", 1)
	s = strings.Replace(s, "static Object leaf(){return new Leaf<BridgeValue>();}", "static <X extends BridgeMarker<X>> Object leaf(){return new BridgeOuter<X>(){};}", 1)
	at, end := strings.Index(s, "class BridgeDerived"), strings.Index(s, "class BridgeDriver")
	if at < 0 || end <= at {
		t.Fatal("fixture declaration boundaries")
	}
	s = s[:at] + s[end:]
	at = strings.Index(s, ` if(args.length>0&&args[0].equals("special"))`)
	end = strings.Index(s, ` bridge.setAccessible(true)`)
	if at < 0 || end <= at {
		t.Fatal("fixture driver boundaries")
	}
	s = s[:at] + s[end:]
	return strings.ReplaceAll(s, "BridgeOuter$Leaf", "BridgeOuter$1")
}

func TestAdversarialInheritedGenericSuperBridgeUsesStaticMethodScope(t *testing.T) {
	// Exercise the independent flat declaration, also used when native
	// anonymous reconstruction cannot represent another ordinary method.
	t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, root := range []string{"BridgeOuter", "RenamedStaticFactory"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(inheritedAnonymousGenericSuperFixture(t), "BridgeOuter", root)
				testIndependentInheritedBridgeOwner(t, modern, java, native, fixture, root, root+"$1", debug, false, "288:inherited:generic:super:checks:dispatch\n", nil)
			})
		}
	}
}

// Two independently bound parent formals distinguish a narrowed key port from
// unchanged Object ports and an erased generic result. A substitution by name
// or parameter position alone must not accidentally narrow the second formal.
func TestAdversarialInheritedGenericSuperBridgeKeepsIndependentResultAndArrayPorts(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	fixture := inheritedGenericSuperFixture(t)
	fixture = strings.NewReplacer(
		"interface BridgePort<T>{String exchange", "interface BridgePort<T,R>{R exchange",
		"T[] array", "R[] array",
		"implements BridgePort<X>", "implements BridgePort<X,Y>",
		"class BridgeOuter<X extends Comparable<X>>", "class BridgeOuter<X extends Comparable<X>,Y>",
		"extends BridgeOuter<X>{", "extends BridgeOuter<X,Object>{",
		"X[] array", "Y[] array",
		"BridgeValue[] array", "Object[] array",
		"m.getReturnType()!=String.class", "m.getReturnType()!=Object.class",
		"methodType(String.class,", "methodType(Object.class,",
		"return (String)h.bindTo(this).invokeExact", "return (Object)h.bindTo(this).invokeExact",
		"public String special", "public Object special",
		"a instanceof BridgeMarker[]", "a instanceof Object[]",
	).Replace(fixture)
	fixture = strings.Replace(fixture, "public String exchange", "public Y exchange", 1)
	expr := `n+":"+Long.toHexString(Double.doubleToRawLongBits(d))+":"+value+":"+(array==null?"null":java.util.Arrays.toString(array))`
	if !strings.Contains(fixture, "return "+expr+";") {
		t.Fatal("fixture generic result")
	}
	fixture = strings.Replace(fixture, "return "+expr+";", "return (Y)(Object)("+expr+");", 1)
	for _, root := range []string{"BridgeOuter", "RenamedTwoParameterOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				source := strings.ReplaceAll(fixture, "BridgeOuter", root)
				testIndependentInheritedBridge(t, modern, java, native, source, root, debug, false, "288:inherited:generic:super:checks:dispatch\n")
			})
		}
	}
}

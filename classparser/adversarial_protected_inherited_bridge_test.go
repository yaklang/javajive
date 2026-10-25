package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialProtectedInheritedCovariantBridgeKeepsVisibilityAndSpecialDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original javac8 required")
	}
	for _, root := range []string{"BridgeOuter", "RenamedProtectedOwner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(inheritedCovariantBridgeFixture, "BridgeOuter", root)
				fixture = strings.ReplaceAll(fixture, "public Object read(", "protected Object read(")
				fixture = strings.ReplaceAll(fixture, "public String read(", "protected String read(")
				fixture = strings.ReplaceAll(fixture, "!m.isSynthetic()", "!java.lang.reflect.Modifier.isProtected(m.getModifiers())||!m.isSynthetic()")
				testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, false, "36:inherited:covariant:bridge:dispatch:wide:exception\n")
			})
		}
	}
}

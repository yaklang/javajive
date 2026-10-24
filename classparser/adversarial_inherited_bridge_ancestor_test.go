package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestAdversarialInheritedBridgeResolvesDeclarationThroughSuperclassChain(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, generic := range []bool{false, true} {
		for _, depth := range []int{1, 2} {
			for _, root := range []string{"BridgeOuter", "DifferentAncestorScope"} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(fmt.Sprintf("generic%v/depth%d/%s/%s", generic, depth, root, debug), func(t *testing.T) {
						parent, declarations := "BridgeOuter", ""
						for i := 0; i < depth; i++ {
							name := fmt.Sprintf("Layer%d", i)
							declarations += "static class " + name + " extends " + parent + " {}\n"
							parent = name
						}
						base := inheritedCovariantBridgeFixture
						if generic {
							base = inheritedVarargsBridgeFixture("java.lang.Class<?>", "java.lang.Class", "new java.lang.Class<?>[]{String.class,int.class}", "BridgeOuter")
						}
						fixture := strings.Replace(base, "static class Leaf extends BridgeOuter", declarations+"static class Leaf extends BridgeOuter", 1)
						fixture = strings.ReplaceAll(fixture, "BridgeOuter", root)
						// javac8 can omit the leaf bridge when the intermediate source
						// already inherits it. Keep an authored bridge and insert only
						// the valid superclass edge and its CP owners. The unchanged
						// original-first JVM oracle verifies this classfile protocol
						// before reconstruction, including invokespecial on a subclass.
						testIndependentInheritedBridgeWithOriginal(t, modern, java, native, fixture, root, debug, false, "36:inherited:covariant:bridge:dispatch:wide:exception\n", func(files map[string][]byte) {
							insertAuthoredBridgeSuperclass(t, files, root, parent)
						})
					})
				}
			}
		}
	}
}

func insertAuthoredBridgeSuperclass(t *testing.T, files map[string][]byte, root, parent string) {
	t.Helper()
	name := root + "$Leaf.class"
	object := mustParseLowering(t, files[name])
	index := uint16(object.ConstantPoolManager.AddNewClassInfo(root + "$" + parent))
	object.SuperClass = index
	changed := 0
	for _, constant := range object.ConstantPool {
		if ref, ok := constant.(*ConstantMethodrefInfo); ok {
			if owner, known := sourceBridgeClassName(object, ref.ClassIndex); known && owner == root {
				ref.ClassIndex = index
				changed++
			}
		}
	}
	if changed != 2 {
		t.Fatal("authored constructor and bridge invocation owners", changed)
	}
	files[name] = object.Bytes()
}

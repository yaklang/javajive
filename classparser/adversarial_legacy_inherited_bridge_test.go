package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// This independently authored leaf uses ordinary receiver/argument loads,
// invokespecial of its direct superclass, and areturn. Its class has no
// version-specific execution feature. Change only the original class header;
// the existing first JVM run checks validity and super-dispatch before any
// source reconstruction. The original binary callers still demand the declared
// bridge (including category-two slots, failure identity and a special call on
// a further overriding subclass), not merely a same-named inherited method.
func TestAdversarialLegacyInheritedBridgeRetainsOriginalSuperDispatch(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, major := range []uint16{49, 50, 51, 52} {
		for _, root := range []string{"BridgeOuter", "DifferentBridgeScope"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("major%d/%s/%s", major, root, debug), func(t *testing.T) {
					fixture := strings.ReplaceAll(inheritedCovariantBridgeFixture, "BridgeOuter", root)
					testIndependentInheritedBridgeWithOriginal(t, modern, java, native, fixture, root, debug, false, "36:inherited:covariant:bridge:dispatch:wide:exception\n", func(files map[string][]byte) {
						name := root + "$Leaf.class"
						raw := bytes.Clone(files[name])
						object := mustParseLowering(t, raw)
						if object.MajorVersion != 52 || object.MinorVersion != 0 {
							t.Fatal("authored original class version")
						}
						for _, constant := range object.ConstantPool {
							switch constant.(type) {
							case *ConstantMethodHandleInfo, *ConstantMethodTypeInfo, *ConstantInvokeDynamicInfo:
								t.Fatal("fixture requires a newer constant-pool protocol")
							}
						}
						raw[6], raw[7] = byte(major>>8), byte(major)
						files[name] = raw
					})
				})
			}
		}
	}
}

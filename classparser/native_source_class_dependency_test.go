package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// An unused CONSTANT_Class may remain in the InnerClasses catalog after a
// method disappears. It must not turn an acyclic source graph into a component
// too large to prove. The JVM oracle runs on the mutated original bytes first;
// all owned classes are then rebuilt without original target-class fallback.
func TestNativeUnusedClassConstantDoesNotCreateSourceCycleRoundTrip(t *testing.T) {
	const nodes = 66
	fixture, owners := wideSourceDependencyFixture(nodes, false)
	last := fmt.Sprintf("static final int initialized=GraphDriver.mark(%d);", nodes-1)
	fixture = strings.Replace(fixture, last, last+"static GraphScope0.Marker unused(){return null;}", 1)
	fixture = strings.Replace(fixture, last, last+"Object choose(boolean use){return use?token:null;}", 1)
	fixture = strings.Replace(fixture, "int rows=0;", `Object lastValue=values[nodes-1];java.lang.reflect.Method choose=lastValue.getClass().getDeclaredMethod("choose",boolean.class);choose.setAccessible(true);if(choose.invoke(lastValue,true)!=token||choose.invoke(lastValue,false)!=null)throw new AssertionError("branch frame binding");int rows=0;`, 1)
	for _, prefix := range []string{"GraphScope", "IndependentDeclaration"} {
		t.Run(prefix, func(t *testing.T) {
			f := strings.ReplaceAll(fixture, "GraphScope", prefix)
			names := append([]string(nil), owners...)
			for i := range names {
				names[i] = strings.ReplaceAll(names[i], "GraphScope", prefix)
			}
			mutate := func(t *testing.T, files map[string][]byte) {
				name := fmt.Sprintf("%s%d$Marker.class", prefix, nodes-1)
				object, err := Parse(files[name])
				if err != nil {
					t.Fatal(err)
				}
				removed := false
				for i, method := range object.Methods {
					methodName, known := sourceBridgeUTF8(object, method.NameIndex)
					if known && methodName == "unused" {
						object.Methods = append(object.Methods[:i], object.Methods[i+1:]...)
						removed = true
						break
					}
				}
				if !removed {
					t.Fatal("fixture did not contain the removable method")
				}
				originalNames, known := nativeMemberDependencyNames(object, nil)
				sourceNames, sourceKnown := nativeMemberSourceBindingNames(object, nil)
				target := prefix + "0$Marker"
				contains := func(names []string) bool {
					for _, n := range names {
						if n == target {
							return true
						}
					}
					return false
				}
				if !known || !contains(originalNames) || !sourceKnown || contains(sourceNames) {
					t.Fatal("unused class catalog did not preserve separate original/source closures")
				}
				files[name] = object.Bytes()
			}
			testNativeIndependentMutatedFamilyFixture(t, f, names, "GraphDriver", "66:4:wide-graph:owners:identity:init:callback:failure\n", mutate, nativeLexicalExactSignatures)
		})
	}
}

package javaclassparser

import (
	"strconv"
	"strings"
	"testing"
)

func TestAdversarialEnumSwitchComputedSelectorPreservesEffectsRoundTrip(t *testing.T) {
	for _, root := range []string{"ComputedSwitchOwner", "RenamedComputedOwner"} {
		for _, nesting := range []int{1, 2} {
			t.Run(root+strings.Repeat("Nested", nesting-1), func(t *testing.T) {
				sources := nativeEnumSwitchFamilySources(root)
				source := sources[root+".java"]
				selector := "m"
				if nesting == 2 {
					selector = "SelectorEffects.first(" + selector + ")"
				}
				selector = "SelectorEffects.choose(" + selector + ")"
				source = strings.ReplaceAll(source, "switch(m)", "switch("+selector+")")
				calls := strconv.Itoa(nesting)
				order := "2"
				if nesting == 2 {
					order = "12"
				}
				source = strings.ReplaceAll(source, "if(task.run(m)!=expected.intValue())", "SelectorEffects.calls=0;SelectorEffects.order=0;if(task.run(m)!=expected.intValue()||SelectorEffects.calls!="+calls+"||SelectorEffects.order!="+order+")")
				source = strings.ReplaceAll(source, "try{task.run(null)", "SelectorEffects.calls=0;SelectorEffects.order=0;try{task.run(null)")
				source = strings.ReplaceAll(source, "catch(NullPointerException expected){}", "catch(NullPointerException expected){if(SelectorEffects.calls!="+calls+"||SelectorEffects.order!="+order+")throw new AssertionError(\"null evaluation count\");}")
				source += `class SelectorEffects{static int calls,order;static ` + root + `Mode first(` + root + `Mode value){calls++;order=order*10+1;return value;}static ` + root + `Mode choose(` + root + `Mode value){calls++;order=order*10+2;return value;}}`
				sources[root+".java"] = source
				testNativeEnumSwitchSourceFixture(t, sources, root, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
			})
		}
	}
}

func TestAdversarialEnumSwitchInstanceSelectorKeepsDispatchAndOperandIdentityRoundTrip(t *testing.T) {
	for _, root := range []string{"DispatchSwitchOwner", "RenamedDispatchSwitchOwner"} {
		for _, dispatch := range []string{"virtual", "interface"} {
			t.Run(root+dispatch, func(t *testing.T) {
				sources := nativeEnumSwitchFamilySources(root)
				source := sources[root+".java"]
				source = strings.ReplaceAll(source, "run("+root+"Mode m)", "run(SelectorProvider provider,"+root+"Mode m)")
				source = strings.Replace(source, "switch(m)", "switch(provider.choose(m))", 1)
				source = strings.ReplaceAll(source, "task.run(m)", "task.run(SelectorProvider.instance,m)")
				source = strings.ReplaceAll(source, "task.run(null)", "task.run(SelectorProvider.instance,null)")
				source = strings.ReplaceAll(source, "if(task.run(SelectorProvider.instance,m)!=expected.intValue())", "SelectorEffects.calls=0;if(task.run(SelectorProvider.instance,m)!=expected.intValue()||SelectorEffects.calls!=1)")
				source = strings.ReplaceAll(source, "try{task.run(SelectorProvider.instance,null)", "SelectorEffects.calls=0;try{task.run(SelectorProvider.instance,null)")
				source = strings.ReplaceAll(source, "catch(NullPointerException expected){}", "catch(NullPointerException expected){if(SelectorEffects.calls!=1)throw new AssertionError(\"dispatch/evaluation\");}")
				source += `class SelectorEffects{static int calls;}`
				if dispatch == "virtual" {
					source += `class SelectorProvider{static final SelectorProvider instance=new ActualProvider();` + root + `Mode choose(` + root + `Mode m){throw new AssertionError("lost virtual dispatch");}}class ActualProvider extends SelectorProvider{` + root + `Mode choose(` + root + `Mode m){SelectorEffects.calls++;return m;}}`
				} else {
					source += `interface SelectorProvider{SelectorProvider instance=new ActualProvider();` + root + `Mode choose(` + root + `Mode m);}class ActualProvider implements SelectorProvider{public ` + root + `Mode choose(` + root + `Mode m){SelectorEffects.calls++;return m;}}`
				}
				sources[root+".java"] = source
				testNativeEnumSwitchSourceFixture(t, sources, root, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
			})
		}
	}
}

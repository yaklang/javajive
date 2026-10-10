package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialEnumSwitchFieldSelectorPreservesHeapAndLexicalIdentityRoundTrip(t *testing.T) {
	for _, root := range []string{"FieldSwitchOwner", "RenamedFieldSwitchOwner"} {
		t.Run(root, func(t *testing.T) {
			sources := nativeEnumSwitchFamilySources(root)
			source := sources[root+".java"]
			source = strings.Replace(source, "private final int offset;", `private final int offset;private `+root+`Mode saved;private int writes;public void storeMode(`+root+`Mode value){writes++;saved=value;}public `+root+`Mode savedMode(){return saved;}public int writes(){return writes;}`, 1)
			source = strings.Replace(source, "switch(m)", root+".this.storeMode(m);switch("+root+".this.saved)", 1)
			source = strings.ReplaceAll(source, "if(task.run(m)!=expected.intValue())", "int before=owner.writes();if(task.run(m)!=expected.intValue()||owner.savedMode()!=m||owner.writes()!=before+1)")
			source = strings.ReplaceAll(source, "try{task.run(null)", "int beforeNull=owner.writes();try{task.run(null)")
			source = strings.ReplaceAll(source, "catch(NullPointerException expected){}", "catch(NullPointerException expected){if(owner.savedMode()!=null||owner.writes()!=beforeNull+1)throw new AssertionError(\"null write/read order\");}")
			sources[root+".java"] = source
			testNativeEnumSwitchSourceFixture(t, sources, root, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
		})
	}
}

func TestAdversarialEnumSwitchStaticFieldSelectorReadsActualHeapValueRoundTrip(t *testing.T) {
	for _, root := range []string{"StaticFieldSwitchOwner", "RenamedStaticFieldSwitchOwner"} {
		t.Run(root, func(t *testing.T) {
			sources := nativeEnumSwitchFamilySources(root)
			source := sources[root+".java"]
			source = strings.Replace(source, "switch(m)", "switch(SelectorHeap.current)", 1)
			source = strings.ReplaceAll(source, "if(task.run(m)!=expected.intValue())", "SelectorHeap.current=m;if(task.run(m)!=expected.intValue())")
			source = strings.ReplaceAll(source, "try{task.run(null)", "SelectorHeap.current=null;try{task.run("+root+"Mode.C)")
			source += `class SelectorHeap{static ` + root + `Mode current;}`
			sources[root+".java"] = source
			testNativeEnumSwitchSourceFixture(t, sources, root, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
		})
	}
}

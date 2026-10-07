package javaclassparser

import (
	"strings"
	"testing"
)

// A descriptor parameter need not stay untouched after its last selector read.
// The original-first driver covers all constants, overflow and null; the common
// harness also checks generated synthetic packets and lexical metadata.
func TestAdversarialEnumSelectorParameterWritesAfterReadRoundTrip(t *testing.T) {
	for _, variant := range []string{"after switch", "conditional after switch", "disjoint earlier branch", "after switch in loop-free try"} {
		t.Run(variant, func(t *testing.T) {
			owner := "LiveParameterOwner"
			if variant == "disjoint earlier branch" {
				owner = "DifferentLiveParameterOwner"
			}
			sources := nativeEnumSwitchFamilySources(owner)
			source := sources[owner+".java"]
			original := "switch(m){case A:return offset+x;case B:return offset-x;default:return offset;}"
			body := "int result;switch(m){case A:result=offset+x;break;case B:result=offset-x;break;default:result=offset;}"
			switch variant {
			case "after switch":
				body += "m=" + owner + "Mode.C;return result+(m==" + owner + "Mode.C?0:79);"
			case "conditional after switch":
				body += "if(result==17)m=" + owner + "Mode.A;else m=" + owner + "Mode.B;return result;"
			case "disjoint earlier branch":
				// This write precedes the read physically, but its branch returns.
				// The driver does not use D; a separate original call checks it.
				body = "if(m==" + owner + "Mode.D){m=" + owner + "Mode.A;return 61;}" + body + "return result;"
				sources[owner+"Mode.java"] = "public enum " + owner + "Mode{A,B,C,D}"
				source = strings.Replace(source, owner+"Mode.values()", "new "+owner+"Mode[]{"+owner+"Mode.A,"+owner+"Mode.B,"+owner+"Mode.C}", 1)
				source = strings.Replace(source, "System.out.println(rows", "if(new "+owner+"(0).make(0).run("+owner+"Mode.D)!=61)throw new AssertionError(\"disjoint branch\");System.out.println(rows", 1)
			case "after switch in loop-free try":
				body += "try{m=" + owner + "Mode.C;return result;}catch(RuntimeException ignored){return result;}"
			}
			if strings.Count(source, original) != 1 {
				t.Fatal("fixture packet was not replaced exactly once")
			}
			sources[owner+".java"] = strings.Replace(source, original, body, 1)
			testNativeEnumSwitchSourceFixture(t, sources, owner, "SwitchFamilyDriver", "75:enum:lexical:captured:case:null:overflow\n")
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

// Assertions retain their lexical JVM owner even when nested in control blocks.
// Disabled assertions must still execute surrounding loop/monitor/handler code,
// but never evaluate the assertion condition or message.
func TestNativeAnonymousAssertionStructuredBlocksRoundTrip(t *testing.T) {
	for _, shape := range []struct{ name, body, success, failure string }{
		{"for", `for(int i=0;i<3;i++){MemberAssertionEffects.trace+="E";assert condition(value):message(token);}`, "ECECEC", "ECM"},
		{"while", `int i=0;while(i++<3){MemberAssertionEffects.trace+="E";assert condition(value):message(token);}`, "ECECEC", "ECM"},
		{"do", `int i=0;do{MemberAssertionEffects.trace+="E";assert condition(value):message(token);}while(++i<3);`, "ECECEC", "ECM"},
		{"switch", `switch(token==null?0:1){case 0:MemberAssertionEffects.trace+="E";assert condition(value):message(token);break;default:MemberAssertionEffects.trace+="E";assert condition(value):message(token);}`, "EC", "ECM"},
		{"monitor", `synchronized(this){MemberAssertionEffects.trace+="E";assert condition(value):message(token);}`, "EC", "ECM"},
		{"try", `try{MemberAssertionEffects.trace+="E";assert condition(value):message(token);}catch(AssertionError e){MemberAssertionEffects.trace+="H";throw e;}`, "EC", "ECMH"},
	} {
		for _, enabled := range []bool{false, true} {
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			t.Run(shape.name+"/"+state, func(t *testing.T) {
				f := strings.Replace(nativeAnonymousAssertionFixture, "assert condition(value):message(token);", shape.body, 1)
				disabled := strings.Repeat("E", strings.Count(shape.success, "E"))
				f = strings.Replace(f, `enabled?"C":""`, `enabled?"`+shape.success+`":"`+disabled+`"`, 1)
				f = strings.Replace(f, `enabled?"CM":""`, `enabled?"`+shape.failure+`":"`+disabled+`"`, 1)
				expected := "assertions:false:owner:condition:message:order\n"
				if enabled {
					f = strings.Replace(f, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`, 1)
					expected = "assertions:true:owner:condition:message:order\n"
				}
				testNativePrivateSetterFixture(t, f, "MemberAssertionOwner", "MemberAssertionDriver", expected)
			})
		}
	}
}

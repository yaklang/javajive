package javaclassparser

import (
	"strings"
	"testing"
)

// A loop can return from its successful path while its failure arm reaches a
// terminal footer outside the generated loop. Assertion status and all original
// condition/message effects are independent JVM oracles, not source text checks.
func TestNativeAnonymousAssertionLoopTerminalRoundTrip(t *testing.T) {
	for _, owner := range []string{"MemberAssertionOwner", "IndependentTerminalOwner"} {
		for _, enabled := range []bool{false, true} {
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			t.Run(owner+"/"+state, func(t *testing.T) {
				f := strings.Replace(nativeAnonymousAssertionFixture, "assert condition(value):message(token);", `int i=3;while(true){assert condition(value):message(token);MemberAssertionEffects.trace+="E";if(--i==0)return;}`, 1)
				f = strings.Replace(f, `enabled?"C":""`, `enabled?"CECECE":"EEE"`, 1)
				f = strings.Replace(f, `enabled?"CM":""`, `enabled?"CM":"EEE"`, 1)
				f = strings.ReplaceAll(f, "MemberAssertionOwner", owner)
				expected := "assertions:false:owner:condition:message:order\n"
				if enabled {
					f = strings.Replace(f, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`, 1)
					expected = "assertions:true:owner:condition:message:order\n"
				}
				testNativePrivateSetterFixture(t, f, owner, "MemberAssertionDriver", expected)
			})
		}
	}
}

// Ordinary guards use the same abrupt-exit classification. They remain explicit
// throws and are a positive semantic control, not a claim of a formerly wrong
// runtime result. Original driver bytes compare all constructor/ownership and
// condition/message effects under every policy/debug variant.
func TestNativeAnonymousOrdinaryLoopThrowTerminalRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousAssertionFixture, "assert condition(value):message(token);", `int i=3;while(true){if(!condition(value))throw new IllegalArgumentException(String.valueOf(message(token)));MemberAssertionEffects.trace+="E";if(--i==0)return;}`, 1)
	f = strings.Replace(f, `Boolean.parseBoolean(System.getProperty("fixture.assertions","false"))`, `true`, 1)
	f = strings.Replace(f, `enabled?"C":""`, `enabled?"CECECE":"EEE"`, 1)
	f = strings.Replace(f, `catch(AssertionError e)`, `catch(IllegalArgumentException e)`, 1)
	f = strings.Replace(f, `java.lang.reflect.Field field=child.getClass().getDeclaredField("$assertionsDisabled");if(!field.isSynthetic()||field.getModifiers()!=0x1018)throw new AssertionError("regenerated assertion field metadata");`, ``, 1)
	testNativePrivateSetterFixture(t, f, "MemberAssertionOwner", "MemberAssertionDriver", "assertions:true:owner:condition:message:order\n")
}

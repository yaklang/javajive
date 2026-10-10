package javaclassparser

import (
	"fmt"
	"testing"
)

// These authored inputs are executed by the original JVM after the version
// edit. The same classfile ownership/bridge evidence must remain authoritative
// before MethodHandle/InvokeDynamic existed; a number alone grants no feature.
func TestNativeLegacyForestMixedScopesRoundTrip(t *testing.T) {
	for _, major := range []uint16{49, 50} {
		for _, row := range []struct{ name, source, owner, driver, want string }{
			{"named and anonymous", nativeAccessorAnonymousScopeFixture, "AccessScopeOwner", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n"},
			{"private marker reuse", nativeAccessorAnonymousConstructorMarkerFixture(), "AccessScopeOwner", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n"},
			{"nonstatic callback", nativeRootMemberAnonymousFixture, "RootAnonMemberOwner", "RootAnonMemberDriver", "12:root:member:anonymous:identity:callback:checked\n"},
		} {
			t.Run(fmt.Sprintf("%d/%s", major, row.name), func(t *testing.T) {
				testNativePrivateSetterFixtureWithMutation(t, row.source, row.owner, row.driver, row.want, func(t *testing.T, files map[string][]byte) {
					for n, raw := range files {
						obj, e := Parse(raw)
						if e != nil {
							t.Fatal(e)
						}
						obj.MajorVersion = major
						obj.MinorVersion = 0
						files[n] = obj.Bytes()
					}
				})
			})
		}
	}
}

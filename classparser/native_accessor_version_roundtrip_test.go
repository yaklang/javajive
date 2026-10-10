package javaclassparser

import (
	"fmt"
	"testing"
)

// These valid authored originals retain the same closed JVM accessor packets
// while using pre-51 classfiles, where later CP tags are illegal. The original
// JVM independently validates and executes them before any rebuilt source.
func TestNativeAccessorLegacyVersionRoundTrip(t *testing.T) {
	for _, major := range []uint16{49, 50} {
		for _, row := range []struct{ name, source, owner, driver, want string }{
			{"numeric", nativeCompoundNumericSource(), "NumericOwner", "NumericDriver", "numeric:compound:width:narrow:overflow:binding:order:identity\n"},
			{"boolean", nativeBooleanAccessorFixture, "BoolAccessOwner", "BoolAccessDriver", "2:boolean:accessor:store:result:order\n"},
			{"staticcall", nativeStaticPrivateCallFixture, "StaticCallOwner", "StaticCallDriver", "2:static:private:call:binding:order\n"},
		} {
			t.Run(fmt.Sprintf("%d/%s", major, row.name), func(t *testing.T) {
				testNativePrivateSetterFixtureWithMutation(t, row.source, row.owner, row.driver, row.want, func(t *testing.T, files map[string][]byte) {
					for name, raw := range files {
						obj, e := Parse(raw)
						if e != nil {
							t.Fatal(e)
						}
						obj.MajorVersion = major
						obj.MinorVersion = 0
						files[name] = obj.Bytes()
					}
				})
			})
		}
	}
}

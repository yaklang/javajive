package javaclassparser

import (
	"strings"
	"testing"
)

func testNativeIndependentFamilyFixture(t *testing.T, fixture string, owners []string, driver, want string, verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	testNativeIndependentMutatedFamilyFixture(t, fixture, owners, driver, want, nil, verify...)
}

// Mutations operate only on authored class files before the independent JVM
// oracle runs; the rebuilt family must reproduce that valid original program.
func testNativeIndependentMutatedFamilyFixture(t *testing.T, fixture string, owners []string, driver, want string, mutate func(*testing.T, map[string][]byte), verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, owners, driver, want, mutate, verify...)
}

func testNativeIndependentCompiledFamilyFixture(t *testing.T, compile func(string) map[string][]byte, owners []string, driver, want string, mutate func(*testing.T, map[string][]byte), verify ...func(*testing.T, string, []byte, []byte)) {
	t.Helper()
	javac, _ := t04Tools(t)
	testNativeIndependentCompilerFamilyFixture(t, compile, ModernJavac, javac, owners, driver, want, mutate, verify...)
}

func TestNativeStaticDependencyBothFamiliesRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeStaticDependencyFixture, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyOnlySignatureGenericArrayRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `static class Value{final Object token;Value(Object token){this.token=token;}Object value(){return token;}}`, `static class Value<T>{final T token;Value(T token){this.token=token;}T value(){return token;}}`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value){return value;}`, `<U> OtherScope.Value<U>[] pass(OtherScope.Value<U>[] value){return value;}`)
	f = strings.ReplaceAll(f, `OtherScope.Value value=new OtherScope.Value(token);`, `OtherScope.Value<Object> value=new OtherScope.Value<Object>(token);OtherScope.Value<Object>[] array=(OtherScope.Value<Object>[])new OtherScope.Value<?>[]{value,null};`)
	f = strings.ReplaceAll(f, `child.pass(value)!=value`, `child.pass(array)!=array`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyKeepsLocalMemberNameRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `class DependencyOwner{`, `class DependencyOwner{static class Value{final int n;Value(int n){this.n=n;}}`)
	f = strings.ReplaceAll(f, `OtherScope.Value pass(OtherScope.Value value){return value;}`, `OtherScope.Value pass(OtherScope.Value value){Value local=new Value(31);if(local.n!=31)throw new AssertionError("local member binding");return value;}`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}
func TestNativeStaticDependencyTransitiveFamiliesRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticDependencyFixture, `class OtherScope{`, `class LastScope{static class Item{final Object value;Item(Object value){this.value=value;}}}class OtherScope{`)
	f = strings.ReplaceAll(f, `static class Value{final Object token;Value(Object token){this.token=token;}Object value(){return token;}}`, `static class Value{final LastScope.Item token;Value(Object token){this.token=new LastScope.Item(token);}Object value(){return token.value;}}`)
	testNativeIndependentFamilyFixture(t, f, []string{"DependencyOwner", "OtherScope", "LastScope"}, "DependencyDriver", "static:dependency:identity:callback\n")
}

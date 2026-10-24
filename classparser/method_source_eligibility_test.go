package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialMethodSourceOrdinaryReferencesKeepInvocationBoundary(t *testing.T) {
	for _, fixture := range []methodParameterNameCase{
		{name: "ordinary lambda-dollar synchronized reference", declaration: `class NamedOwner{static synchronized int lambda$ordinary(int value){return Thread.holdsLock(NamedOwner.class)?value:-1;}static java.util.function.IntUnaryOperator first(){return NamedOwner::lambda$ordinary;}static int read(int originalNamed){return originalNamed;}}`, check: `if(NamedOwner.first().applyAsInt(9)!=9||!java.lang.reflect.Modifier.isSynchronized(NamedOwner.class.getDeclaredMethod("lambda$ordinary",int.class).getModifiers()))throw new AssertionError("ordinary synchronized methodref");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true},
		{name: "ordinary virtual reference", declaration: `class NamedOwner{int impl(int value){return value+1;}static java.util.function.IntUnaryOperator first(NamedOwner receiver){return receiver::impl;}static int read(int originalNamed){return originalNamed;}}class NamedSubclass extends NamedOwner{int impl(int value){return value+100;}}`, check: `if(NamedOwner.first(new NamedSubclass()).applyAsInt(9)!=109)throw new AssertionError("original invokevirtual dispatch");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true},
		{name: "ordinary varargs reference", declaration: `class NamedOwner{static int impl(int... values){return values.length;}static java.util.function.ToIntFunction<int[]> first(){return NamedOwner::impl;}static int read(int originalNamed){return originalNamed;}}`, check: `if(NamedOwner.first().applyAsInt(new int[]{1,2})!=2||!NamedOwner.class.getDeclaredMethod("impl",int[].class).isVarArgs())throw new AssertionError("ordinary varargs methodref");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true},
	} {
		t.Run(fixture.name, func(t *testing.T) { methodParameterNameRoundTrip(t, fixture, "none") })
	}
}

func TestAdversarialMethodSourceSynchronizedPartialApplicationRefused(t *testing.T) {
	methodParameterNameRoundTrip(t, methodParameterNameCase{name: "ordinary synchronized implementation", declaration: `class NamedOwner{static synchronized int impl(int captured,int value){return Thread.holdsLock(NamedOwner.class)?captured+value:-1;}static java.util.function.IntUnaryOperator first(int captured){return value->impl(captured,value);}static int read(int originalNamed){return originalNamed;}}`, check: `if(NamedOwner.impl(7,2)!=9||NamedOwner.first(7).applyAsInt(2)!=9)throw new AssertionError("original synchronized monitor");`, parameterTypes: `int.class`, names: []string{"originalNamed"}, parameters: true, refuseSource: true, mutate: func(t *testing.T, obj *ClassObject) { methodParameterRetargetLambdaFactories(t, obj, "(II)I", 1) }}, "none")
}

func TestAdversarialMethodSourceOrdinaryPartialApplicationOrdersRefused(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "early", true: "late"}[reverse], func(t *testing.T) {
			fixture := methodParameterNonsyntheticLambdaFixture()
			fixture.refuseSource = true
			if reverse {
				declaration := `static int impl(int capture,int left,int right){return 100*capture+10*left+right;}`
				fixture.declaration = removeMethodSourceFixtureDeclaration(t, fixture.declaration, declaration)
			}
			methodParameterNameRoundTrip(t, fixture, "none")
		})
	}
}

func removeMethodSourceFixtureDeclaration(t *testing.T, source, declaration string) string {
	t.Helper()
	if strings.Count(source, declaration) != 1 {
		t.Fatal("one authored declaration required")
	}
	return strings.TrimSuffix(strings.Replace(source, declaration, "", 1), "}") + declaration + "}"
}

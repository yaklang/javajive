package javaclassparser

import (
	"strings"
	"testing"
)

const enumLocalSelectorFixture = `enum LocalOriginChoice{FIRST,SECOND,THIRD}
class LocalOriginRoot{static int created;static class Anchor{}private final LocalOriginChoice choice;LocalOriginRoot(LocalOriginChoice c){created++;choice=c;}
 static class Provider{int pick(LocalOriginChoice c){LocalOriginRoot allocated=new LocalOriginRoot(c);switch(allocated.choice){case FIRST:return 11;case SECOND:return -7;default:return 23;}}}}
class LocalOriginDriver{public static void main(String[]args){int rows=0;LocalOriginRoot.Provider provider=new LocalOriginRoot.Provider();
 for(LocalOriginChoice c:new LocalOriginChoice[]{LocalOriginChoice.FIRST,LocalOriginChoice.SECOND,LocalOriginChoice.THIRD,null}){int before=LocalOriginRoot.created;try{int got=provider.pick(c);int want=c==LocalOriginChoice.FIRST?11:c==LocalOriginChoice.SECOND?-7:23;if(c==null||got!=want)throw new AssertionError("local selector value");}catch(NullPointerException e){if(c!=null)throw e;}if(LocalOriginRoot.created!=before+1)throw new AssertionError("local allocation/exception order");rows++;}
 System.out.println(rows+":local:allocation:selector");}}`

func TestAdversarialEnumLocalSelectorPreservesOriginalAllocation(t *testing.T) {
	testNativeIndependentFamilyFixture(t, enumLocalSelectorFixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorPreservesFactoryResult(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `static class Provider{`, `static LocalOriginRoot make(LocalOriginChoice c){return new LocalOriginRoot(c);}static class Provider{`, 1)
	fixture = strings.Replace(fixture, `allocated=new LocalOriginRoot(c)`, `allocated=LocalOriginRoot.make(c)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorPreservesCopiedParameter(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `int pick(LocalOriginChoice c){LocalOriginRoot allocated=new LocalOriginRoot(c);`, `int pick(LocalOriginRoot input){LocalOriginRoot allocated=input;`, 1)
	fixture = strings.Replace(fixture, `provider.pick(c)`, `provider.pick(new LocalOriginRoot(c))`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorDoesNotBorrowOverwrittenParameter(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `int pick(LocalOriginChoice c){LocalOriginRoot allocated=new LocalOriginRoot(c);`, `int pick(LocalOriginRoot input){LocalOriginRoot allocated=input;input=new LocalOriginRoot(LocalOriginChoice.SECOND);`, 1)
	fixture = strings.Replace(fixture, `provider.pick(c)`, `provider.pick(new LocalOriginRoot(c))`, 1)
	fixture = strings.Replace(fixture, `created!=before+1`, `created!=before+2`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorRetainsOriginalCastBeforeTableRead(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `static class Provider{`, `static Object make(LocalOriginChoice c){if(c==LocalOriginChoice.THIRD)return "wrong runtime class";return new LocalOriginRoot(c);}static class Provider{`, 1)
	fixture = strings.Replace(fixture, `allocated=new LocalOriginRoot(c)`, `allocated=(LocalOriginRoot)LocalOriginRoot.make(c)`, 1)
	fixture = strings.Replace(fixture, `if(c==null||got!=want)`, `if(c==null||c==LocalOriginChoice.THIRD||got!=want)`, 1)
	fixture = strings.Replace(fixture, `catch(NullPointerException e)`, `catch(ClassCastException e){if(c!=LocalOriginChoice.THIRD)throw e;}catch(NullPointerException e)`, 1)
	fixture = strings.Replace(fixture, `created!=before+1`, `created!=before+(c==LocalOriginChoice.THIRD?0:1)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorAllowsLaterStoreOnDisjointExit(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `case FIRST:return 11;`, `case FIRST:allocated=new LocalOriginRoot(LocalOriginChoice.SECOND);return 11;`, 1)
	fixture = strings.Replace(fixture, `created!=before+1`, `created!=before+(c==LocalOriginChoice.FIRST?2:1)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

func TestAdversarialEnumLocalSelectorKeepsFactoryExceptionBeforeTableRead(t *testing.T) {
	fixture := strings.Replace(enumLocalSelectorFixture, `static class Provider{`, `static LocalOriginRoot make(LocalOriginChoice c){if(c==LocalOriginChoice.SECOND)throw new IllegalArgumentException("factory");return new LocalOriginRoot(c);}static class Provider{`, 1)
	fixture = strings.Replace(fixture, `allocated=new LocalOriginRoot(c)`, `allocated=LocalOriginRoot.make(c)`, 1)
	fixture = strings.Replace(fixture, `if(c==null||got!=want)`, `if(c==null||c==LocalOriginChoice.SECOND||got!=want)`, 1)
	fixture = strings.Replace(fixture, `catch(NullPointerException e)`, `catch(IllegalArgumentException e){if(c!=LocalOriginChoice.SECOND||!"factory".equals(e.getMessage()))throw e;}catch(NullPointerException e)`, 1)
	fixture = strings.Replace(fixture, `created!=before+1`, `created!=before+(c==LocalOriginChoice.SECOND?0:1)`, 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"LocalOriginRoot"}, "LocalOriginDriver", "4:local:allocation:selector\n", nativeLexicalExactSignatures)
}

package javaclassparser

import (
	"strings"
	"testing"
)

const booleanDelegationFixture = `class BooleanTarget{final boolean flag;BooleanTarget(boolean flag){this.flag=flag;}BooleanTarget(int word){throw new AssertionError("wrong int overload");}Object token(){return null;}}
class BooleanOwner{BooleanTarget yes(final Object token){return new BooleanTarget(true){Object token(){return token;}};}BooleanTarget no(final Object token){return new BooleanTarget(false){Object token(){return token;}};}}
class BooleanDriver{public static void main(String[]args){BooleanOwner owner=new BooleanOwner();Object shared=new Object();int rows=0;for(Object token:new Object[]{null,shared}){BooleanTarget yes=owner.yes(token),no=owner.no(token);if(!yes.flag||no.flag||yes.token()!=token||no.token()!=token||yes==no)throw new AssertionError("word/descriptor/capture identity");rows++;}System.out.println(rows+":boolean:constructor:identity");}}`

func TestAdversarialConstructorBooleanDelegationRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, booleanDelegationFixture, []string{"BooleanOwner"}, "BooleanDriver", "2:boolean:constructor:identity\n")
}
func TestAdversarialConstructorBooleanDelegationRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(booleanDelegationFixture, "Boolean", "SeparateWord")
	testNativeIndependentFamilyFixture(t, f, []string{"SeparateWordOwner"}, "SeparateWordDriver", "2:boolean:constructor:identity\n")
}

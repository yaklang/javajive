package javaclassparser

import (
	"strings"
	"testing"
)

const nativeSetterRegistrationFixture = `class SetterRegistrationEffects{static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("original");static SetterRegistrationOwner left(SetterRegistrationOwner owner){trace+="L";if(fail==1)throw failure;return owner;}static SetterRegistrationOwner right(SetterRegistrationOwner owner){trace+="R";if(fail==2)throw failure;return owner;}}
class SetterRegistrationOwner{private Object first;private volatile Object second;SetterRegistrationOwner(Object first,Object second){this.first=first;this.second=second;}class Writer{Object copy(SetterRegistrationOwner left,SetterRegistrationOwner right){return SetterRegistrationEffects.left(left).first=SetterRegistrationEffects.right(right).second;}Object first(){return first;}}Writer writer(){return new Writer();}}
class SetterRegistrationDriver{public static void main(String[]args){Object before=new Object(),token=new Object();SetterRegistrationOwner left=new SetterRegistrationOwner(before,null),right=new SetterRegistrationOwner(null,token);SetterRegistrationOwner.Writer writer=left.writer();SetterRegistrationEffects.trace="";if(writer.copy(left,right)!=token||writer.first()!=token||!SetterRegistrationEffects.trace.equals("LR"))throw new AssertionError("field/assignment/binding/once");int rows=0;for(int nullSide:new int[]{0,1})for(int fail:new int[]{0,1,2}){SetterRegistrationEffects.fail=fail;SetterRegistrationEffects.trace="";try{writer.copy(nullSide==0?null:left,nullSide==1?null:right);throw new AssertionError("missing failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=SetterRegistrationEffects.failure)throw new AssertionError("failure identity");}if(!SetterRegistrationEffects.trace.equals(fail==1?"L":"LR"))throw new AssertionError("receiver/RHS order");rows++;}System.out.println(rows+":private:setter:registration:evaluation:order");}}`

func TestNativeSetterRegistrationRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeSetterRegistrationFixture, "SetterRegistrationOwner", "SetterRegistrationDriver", "6:private:setter:registration:evaluation:order\n")
}
func TestNativeSetterRegistrationRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeSetterRegistrationFixture, "SetterRegistrationOwner", "OtherPrivateWriteOrderScope")
	testNativePrivateSetterFixture(t, f, "OtherPrivateWriteOrderScope", "SetterRegistrationDriver", "6:private:setter:registration:evaluation:order\n")
}

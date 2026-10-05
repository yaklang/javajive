package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAccessorAnonymousScopeFixture = `class AccessScopeOwner{private Object token;AccessScopeOwner(Object token){this.token=token;}void reset(Object token){this.token=token;}static class Reader{Object get(AccessScopeOwner owner){return owner.token;}}static java.util.concurrent.Callable<Object> callback(final AccessScopeOwner owner){return new java.util.concurrent.Callable<Object>(){public Object call(){return owner.token;}};}}
class AccessScopeDriver{public static void main(String[]args)throws Exception{AccessScopeOwner.Reader reader=new AccessScopeOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){AccessScopeOwner owner=new AccessScopeOwner(token);java.util.concurrent.Callable<Object> callback=AccessScopeOwner.callback(owner);if(reader.get(owner)!=token||callback.call()!=token)throw new AssertionError("named/anonymous private binding");Object next=new Object();owner.reset(next);if(reader.get(owner)!=next||callback.call()!=next)throw new AssertionError("captured reference versus field snapshot");rows++;}try{AccessScopeOwner.callback(null).call();throw new AssertionError("missing null capture failure");}catch(NullPointerException expected){}System.out.println(rows+":private:accessor:named:anonymous:scope:identity");}}`

func TestNativeAccessorAnonymousScopeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAccessorAnonymousScopeFixture, "AccessScopeOwner", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n")
}
func TestNativeAccessorAnonymousScopeRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeAccessorAnonymousScopeFixture, "AccessScopeOwner", "OtherCommittedAccessScope")
	testNativePrivateSetterFixture(t, source, "OtherCommittedAccessScope", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n")
}

func nativeAccessorAnonymousConstructorMarkerFixture() string {
	source := strings.Replace(nativeAccessorAnonymousScopeFixture, "static class Reader{Object get", "static class Reader{private Reader(){}Object get", 1)
	source = strings.Replace(source, "static java.util.concurrent.Callable<Object> callback", "static Reader reader(){return new Reader();}static java.util.concurrent.Callable<Object> callback", 1)
	return strings.Replace(source, "new AccessScopeOwner.Reader()", "AccessScopeOwner.reader()", 1)
}

func TestNativeAccessorAnonymousConstructorMarkerRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAccessorAnonymousConstructorMarkerFixture(), "AccessScopeOwner", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n")
}

func TestNativeAccessorAnonymousConstructorMarkerRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeAccessorAnonymousConstructorMarkerFixture(), "AccessScopeOwner", "OtherCommittedAccessScope")
	testNativePrivateSetterFixture(t, source, "OtherCommittedAccessScope", "AccessScopeDriver", "2:private:accessor:named:anonymous:scope:identity\n")
}

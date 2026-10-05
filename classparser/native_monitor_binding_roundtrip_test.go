package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMonitorBindingFixture = `class MonitorBindingOwner{private Object token;MonitorBindingOwner(Object token){this.token=token;}Object read(Object gate,boolean take){synchronized(gate){if(take)return token;}return null;}static class Reader{Object get(MonitorBindingOwner owner,Object gate,boolean take){return owner.read(gate,take);}Object direct(MonitorBindingOwner owner){return owner.token;}}}
class MonitorBindingDriver{public static void main(String[]args){MonitorBindingOwner.Reader reader=new MonitorBindingOwner.Reader();int rows=0;for(Object token:new Object[]{null,new Object()}){MonitorBindingOwner owner=new MonitorBindingOwner(token);Object gate=new Object();if(reader.direct(owner)!=token)throw new AssertionError("private capture");for(boolean take:new boolean[]{false,true}){if(reader.get(owner,gate,take)!=(take?token:null))throw new AssertionError("lock binding/identity");try{reader.get(owner,null,take);throw new AssertionError("null monitor");}catch(NullPointerException expected){}rows++;}}System.out.println(rows+":monitor:binding:private:identity");}}`

func TestNativeMonitorNamespaceRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMonitorBindingFixture, "MonitorBindingOwner", "MonitorBindingDriver", "4:monitor:binding:private:identity\n")
}
func TestNativeMonitorNamespaceRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMonitorBindingFixture, "MonitorBindingOwner", "ReleaseScope")
	f = strings.ReplaceAll(f, "token", "reservedValue")
	testNativePrivateSetterFixture(t, f, "ReleaseScope", "MonitorBindingDriver", "4:monitor:binding:private:identity\n")
}

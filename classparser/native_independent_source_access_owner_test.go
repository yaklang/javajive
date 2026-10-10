package javaclassparser

import (
	"testing"
)

const nativeIndependentClientFixture = `class ClientScope{static class Root{class Value{final Object token;Value(Object token){this.token=token;}Object read(){return token;}}}}class SeparateClient{static ClientScope.Root.Value make(ClientScope.Root root,Object token){return root.new Value(token);}}class ClientDriver{public static void main(String[]args){int rows=0;Object same=new Object();for(Object token:new Object[]{null,same,"same",new String("same")})for(boolean nil:new boolean[]{false,true}){ClientScope.Root root=nil?null:new ClientScope.Root();try{ClientScope.Root.Value value=SeparateClient.make(root,token);if(nil||value.read()!=token||value.getClass().getDeclaringClass()!=ClientScope.Root.class)throw new AssertionError("owned constructor/binding/identity");}catch(NullPointerException e){if(!nil)throw e;}rows++;}System.out.println(rows+":separate:constructor:client");}}`

func nativeIndependentClientInput(t *testing.T, files map[string][]byte) {
	t.Helper()
	o, e := Parse(files["ClientScope.class"])
	if e != nil {
		t.Fatal(e)
	}
	o.MajorVersion = 55
	files["ClientScope.class"] = o.Bytes()
}

func TestNativeIndependentSourceAccessDoesNotClaimConstructorClients(t *testing.T) {
	files := nativeCompileClasses(t, nativeIndependentClientFixture)
	nativeIndependentClientInput(t, files)
	z := nativeArchive(t, files)
	defer z.Close()
	obj, e := Parse(files["ClientScope$Root.class"])
	if e != nil {
		t.Fatal(e)
	}
	d := z.nativeMemberReader(obj)
	cert := d.originalNativeMemberIndependentRoot()
	if cert == nil {
		t.Fatal("original static boundary")
	}
	p := z.prepareNativeMemberFamilyFromRoot(obj, nil, cert)
	if p == nil {
		t.Fatal("original family admission")
	}
	if p.objects["SeparateClient"] == nil || p.family.lexicalObjects["SeparateClient"] != nil {
		t.Fatal("original external constructor client must remain a validation input, not source/private ownership")
	}
	if !z.nativeMemberIndependentInvocationsClosed(p) {
		t.Fatal("external constructor user incorrectly required to be source-owned")
	}
	if z.sourceOwnership.bytes != 0 {
		t.Fatal("read-only query published source")
	}
}

func TestNativeIndependentSourceExternalConstructorClientsRoundTrip(t *testing.T) {
	testNativeIndependentMutatedFamilyFixture(t, nativeIndependentClientFixture, []string{"ClientScope$Root", "SeparateClient"}, "ClientDriver", "8:separate:constructor:client\n", nativeIndependentClientInput, nativeLexicalExactSignatures)
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialPlatformSuperclassMetadataPreservesSourceOwnership(t *testing.T) {
	for _, prefix := range []string{"Header", "Cove"} {
		t.Run(prefix, func(t *testing.T) {
			sources := map[string]string{prefix + "Owner.java": `package q;public class HeaderOwner extends java.util.zip.ZipEntry{private Object token;public HeaderOwner(String name,Object value){super(name);token=value;}public static class Reader{public Object read(HeaderOwner owner){return owner.token;}}}`, prefix + "Driver.java": `package q;public class HeaderDriver{public static void main(String[]args)throws Exception{if(HeaderOwner.Reader.class.getDeclaringClass()!=HeaderOwner.class||!HeaderOwner.class.getDeclaredMethod("access$000",HeaderOwner.class).isSynthetic())throw new AssertionError("original source owner/accessor flags");HeaderOwner.Reader reader=new HeaderOwner.Reader();int rows=0;Object token=new Object();for(String name:new String[]{"","plain","\u03bb\ud83d\ude00"})for(Object value:new Object[]{null,token}){HeaderOwner owner=new HeaderOwner(name,value);if(reader.read(owner)!=value||!owner.getName().equals(name))throw new AssertionError("original constructor/name/storage binding");rows++;}try{new HeaderOwner(null,token);throw new AssertionError("missing original constructor validation");}catch(NullPointerException expected){}try{reader.read(null);throw new AssertionError("missing original accessor null receiver");}catch(NullPointerException expected){}System.out.println(rows+":platform:original:metadata:scope:constructor:identity");}}`}
			ss := map[string]string{}
			for n, s := range sources {
				ss["q/"+n] = strings.ReplaceAll(s, "Header", prefix)
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/"+prefix+"Owner", "q."+prefix+"Driver", "6:platform:original:metadata:scope:constructor:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, ss, debug, "8")
			}, nil, []string{"normal"})
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAnonymousSetterKeepsUnrelatedNamedCaptureScope(t *testing.T) {
	const fixture = `interface ScopeView{int get();}
public class ScopeOwner{ScopeView first(){return new ScopeView(){private int var0;class Writer{int put(int next){return var0=next;}}public int get(){return new Writer().put(7);}};}class Sibling{ScopeView delayed(final int var0){return new ScopeView(){public int get(){return var0;}};}}Sibling sibling(){return new Sibling();}}
class ScopeDriver{public static void main(String[]args)throws Exception{ScopeOwner root=new ScopeOwner();if(root.first().get()!=7)throw new AssertionError("anonymous write");int rows=0;for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){ScopeView view=root.sibling().delayed(seed);if(view.get()!=seed||view.getClass().getEnclosingMethod().getDeclaringClass()!=ScopeOwner.Sibling.class)throw new AssertionError("unrelated original named capture scope");java.lang.reflect.Field field=view.getClass().getDeclaredField("val$var0");if(!field.isSynthetic())throw new AssertionError("original capture binding");rows++;}System.out.println(rows+":disjoint:anonymous:write:named:capture");}}`
	for _, prefix := range []string{"Scope", "Cove"} {
		t.Run(prefix, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "Scope", prefix)
			testNativePrivateSetterCompiledFixtureWithPolicies(t, prefix+"Owner", prefix+"Driver", "5:disjoint:anonymous:write:named:capture\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{prefix + "Owner.java": source}, debug, "8")
			}, nil, []string{"normal"})
		})
	}
}

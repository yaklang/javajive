package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The source constructor has no explicit parameters, but the original binary
// has independently typed enclosing/reference/wide capture ports. Absence of
// Signature is observable: reflection then returns the physical parameter
// types. A modern compiler's ()V must not silently turn that list into empty.
func TestAdversarialImplicitConstructorMetadataAcrossCompilerTargets(t *testing.T) {
	const source = `class CaptureEffects {static Object token;static Object owner;static long word;static String trace;}
class CaptureOwner {final long offset;CaptureOwner(long x){offset=x;}Runnable make(final Object token,final long value){return new Runnable(){public void run(){CaptureEffects.trace+="R";CaptureEffects.token=token;CaptureEffects.owner=CaptureOwner.this;CaptureEffects.word=value+offset;}};}}
class CaptureDriver {public static void main(String[]args)throws Exception{int rows=0;Object id=new Object();for(Object token:new Object[]{null,id})for(long value:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long offset:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){CaptureOwner owner=new CaptureOwner(offset);Runnable action=owner.make(token,value);java.lang.reflect.Constructor<?>[]ctors=action.getClass().getDeclaredConstructors();if(ctors.length!=1)throw new AssertionError("constructor universe");Class<?>[]physical=ctors[0].getParameterTypes();java.lang.reflect.Type[]generic=ctors[0].getGenericParameterTypes();if(physical.length!=3||generic.length!=physical.length)throw new AssertionError("implicit parameter reflection");for(int i=0;i<physical.length;i++)if(physical[i]!=generic[i])throw new AssertionError("physical generic role");if(action.getClass().getEnclosingClass()!=CaptureOwner.class||!action.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("lexical owner");CaptureEffects.trace="";action.run();long want=java.math.BigInteger.valueOf(value).add(java.math.BigInteger.valueOf(offset)).longValue();if(CaptureEffects.owner!=owner||CaptureEffects.token!=token||CaptureEffects.word!=want||!CaptureEffects.trace.equals("R"))throw new AssertionError("capture/overflow/identity/effect");rows++;}System.out.println(rows+":capture:constructor:reflection:identity:overflow");}}`
	for _, name := range []string{"CaptureOwner", "RenamedCaptureOwner"} {
		for _, original := range []int{5, 7, 8} {
			for _, target := range []int{8, 16} {
				for _, debug := range []string{"none", "source,lines,vars"} {
					t.Run(fmt.Sprintf("%s/original%d/target%d/%s", name, original, target, debug), func(t *testing.T) {
						fixture := strings.ReplaceAll(source, "CaptureOwner", name)
						roundTripConstructorBridgeArtifact(t, fixture, name, "CaptureDriver", "30:capture:constructor:reflection:identity:overflow\n", debug, target, original)
					})
				}
			}
		}
	}
}

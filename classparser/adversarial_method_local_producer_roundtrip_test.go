package javaclassparser

import (
	"strings"
	"testing"
)

// Producer captures require their actual source declaration and a common
// dominating lexical block. A parameter-only method-head placement cannot
// move a producer call, delay its exception or regenerate an uninitialized
// capture when a superclass invokes the overridden method.
func TestAdversarialMethodLocalProducerDeclarationAndPreSuperEffects(t *testing.T) {
	for _, root := range []string{"LocalMadeOwner", "OtherLocalMadeOwner", "StaticLocalMadeOwner", "pkg/LocalMadeOwner", "VirtualLocalMadeOwner", "SpecialLocalMadeOwner", "InterfaceLocalMadeOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `abstract class LocalMadeBase {
 static int trace,failMode;static Object published,observed;static long observedLong;static final RuntimeException failure=new IllegalArgumentException("identity");
 LocalMadeBase(){trace=trace*10+3;published=this;observedLong=read(0);observed=token();if(failMode==3)throw failure;}
 abstract long read(long delta);abstract Object token();
 static long derive(long seed){trace=trace*10+1;if(failMode==1)throw failure;return seed*31+7;}
 static Object choose(Object token){trace=trace*10+2;if(failMode==2)throw failure;return token;}}
class LocalMadeOwner {LocalMadeBase make(long seed,Object token,boolean flag){final long made=LocalMadeBase.derive(seed);final Object held=LocalMadeBase.choose(token);class Entry extends LocalMadeBase {long read(long delta){return made+delta;}Object token(){return held;}}if(flag)return new Entry();return new Entry();}}
class LocalMadeDriver {public static void main(String[] args){int count=0;Object identity=new Object();for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long delta:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(int mode=0;mode<4;mode++)for(boolean flag:new boolean[]{false,true}){LocalMadeBase.trace=0;LocalMadeBase.failMode=mode;LocalMadeBase.published=null;LocalMadeBase.observed=null;LocalMadeBase.observedLong=0;LocalMadeBase value=null;Throwable failure=null;try{value=new LocalMadeOwner().make(seed,token,flag);}catch(Throwable ex){failure=ex;}if(mode==0?failure!=null:failure!=LocalMadeBase.failure)throw new AssertionError("exception identity/priority");if(LocalMadeBase.trace!=(mode==1?1:mode==2?12:123))throw new AssertionError("capture producer/super call order");long made=java.math.BigInteger.valueOf(seed).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(7)).longValue();if(mode==0||mode==3){if(LocalMadeBase.observedLong!=made||LocalMadeBase.observed!=token||LocalMadeBase.published==null)throw new AssertionError("capture before superclass callback");}else if(LocalMadeBase.published!=null)throw new AssertionError("published before capture completion");if(mode==0){if(value!=LocalMadeBase.published||value.token()!=token||value.read(delta)!=java.math.BigInteger.valueOf(made).add(java.math.BigInteger.valueOf(delta)).longValue())throw new AssertionError("producer capture arithmetic/reference identity");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getDeclaringClass()!=null||kind.getEnclosingClass()!=LocalMadeOwner.class||!kind.getEnclosingMethod().getName().equals("make")||!kind.getName().equals("LocalMadeOwner$1Entry")||kind.getDeclaredConstructors()[0].getParameterCount()!=3)throw new AssertionError("original method-local constructor ABI");}count++;}System.out.println(count+":local:producer:pre-super:effects:exception-identity:wide:ordinal");}}`
			driver := "LocalMadeDriver"
			simple := root
			if strings.Contains(root, "/") {
				simple = "LocalMadeOwner"
				source = "package pkg;" + source
				driver = "pkg.LocalMadeDriver"
				source = strings.ReplaceAll(source, "equals(\"LocalMadeOwner$1Entry\")", "equals(\"pkg.LocalMadeOwner$1Entry\")")
			}
			source = strings.ReplaceAll(source, "LocalMadeOwner", simple)
			if strings.HasPrefix(root, "Static") {
				source = strings.ReplaceAll(source, "{LocalMadeBase make(", "{static LocalMadeBase make(")
				source = strings.ReplaceAll(source, "getParameterCount()!=3", "getParameterCount()!=2")
			}
			if strings.HasPrefix(root, "Virtual") || strings.HasPrefix(root, "Special") {
				visibility := ""
				if strings.HasPrefix(root, "Special") {
					visibility = "private "
				}
				source = strings.Replace(source, "class "+root+" {", "class "+root+" {"+visibility+"long derive(long seed){return LocalMadeBase.derive(seed);}", 1)
				source = strings.Replace(source, "final long made=LocalMadeBase.derive(seed)", "final long made=derive(seed)", 1)
			}
			if strings.HasPrefix(root, "Interface") {
				source = "interface ProducedFunction { long apply(long seed); }" + source
				source = strings.Replace(source, "make(long seed,Object token,boolean flag)", "make(long seed,Object token,boolean flag,ProducedFunction function)", 1)
				source = strings.Replace(source, "final long made=LocalMadeBase.derive(seed)", "final long made=function.apply(seed)", 1)
				source = strings.Replace(source, ".make(seed,token,flag)", ".make(seed,token,flag,new ProducedFunction(){public long apply(long input){return LocalMadeBase.derive(input);}})", 1)
			}
			testNativeIndependentFamilyFixture(t, source, []string{root}, driver, "400:local:producer:pre-super:effects:exception-identity:wide:ordinal\n")
		})
	}
}

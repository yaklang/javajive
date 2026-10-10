package javaclassparser

import (
	"strings"
	"testing"
)

// A runtime CHECKCAST remains after the exact producer and before NEW/super.
// A same-typed replacement or invented binding cast cannot move, omit or repeat
// this failure boundary. Original JVM properties run before every rebuild.
func TestAdversarialMethodLocalCheckedProducerRoundTrip(t *testing.T) {
	for _, owner := range []string{"CastLocalOwner", "OtherCastLocalOwner", "GenericCastLocalOwner"} {
		t.Run(owner, func(t *testing.T) {
			source := `interface CastFunction{Object apply(Object input);}
abstract class CastLocalBase {static int trace,mode;static Object observed,published;static final RuntimeException failure=new IllegalArgumentException("identity");CastLocalBase(){trace=trace*10+2;published=this;observed=get();if(mode==2)throw failure;}abstract CharSequence get();}
class CastLocalOwner {CastLocalBase make(CastFunction function,Object input,boolean branch){final CharSequence made=(CharSequence)function.apply(input);class Entry extends CastLocalBase {CharSequence get(){return made;}}if(branch)return new Entry();return new Entry();}}
class CastLocalDriver{public static void main(String[] args){int count=0;Object[] inputs={null,"identity",new StringBuilder("builder"),new Object()};CastFunction function=new CastFunction(){public Object apply(Object input){CastLocalBase.trace=CastLocalBase.trace*10+1;if(CastLocalBase.mode==1)throw CastLocalBase.failure;return input;}};for(Object input:inputs)for(int mode=0;mode<3;mode++)for(boolean branch:new boolean[]{false,true}){CastLocalBase.mode=mode;CastLocalBase.trace=0;CastLocalBase.observed=null;CastLocalBase.published=null;Throwable failure=null;CastLocalBase value=null;try{value=new CastLocalOwner().make(function,input,branch);}catch(Throwable e){failure=e;}boolean invalid=input!=null&&!(input instanceof CharSequence);if(mode==1){if(failure!=CastLocalBase.failure||CastLocalBase.trace!=1||CastLocalBase.published!=null)throw new AssertionError("producer exception identity/order");}else if(invalid){if(failure==null||failure.getClass()!=ClassCastException.class||CastLocalBase.trace!=1||CastLocalBase.published!=null)throw new AssertionError("actual checkcast before NEW/super");}else{if(CastLocalBase.trace!=12||CastLocalBase.published==null||CastLocalBase.observed!=input)throw new AssertionError("captured reference before callback");if(mode==2){if(failure!=CastLocalBase.failure)throw new AssertionError("super exception identity");}else{if(failure!=null||value!=CastLocalBase.published||value.get()!=input)throw new AssertionError("reference identity");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getEnclosingClass()!=CastLocalOwner.class||!kind.getEnclosingMethod().getName().equals("make")||kind.getDeclaredConstructors()[0].getParameterCount()!=2)throw new AssertionError("local/constructor ABI");}}count++;}System.out.println(count+":cast:producer:identity:pre-super:exception-order");}}`
			source = strings.ReplaceAll(source, "CastLocalOwner", owner)
			if strings.HasPrefix(owner, "Generic") {
				source = strings.Replace(source, "{CastLocalBase make(", "{<T extends CharSequence> CastLocalBase make(", 1)
				source = strings.Replace(source, "final CharSequence made=(CharSequence)function.apply(input)", "final T made=(T)function.apply(input)", 1)
				source = strings.Replace(source, "CharSequence get(){return made;}", "T get(){return made;}", 1)
				source = strings.Replace(source, "count++;", `if(value!=null){try{java.lang.reflect.Type type=value.getClass().getDeclaredMethod("get").getGenericReturnType();if(!type.equals(value.getClass().getEnclosingMethod().getTypeParameters()[0]))throw new AssertionError("original enclosing method type variable");}catch(NoSuchMethodException missing){throw new AssertionError(missing);}}count++;`, 1)
			}
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "CastLocalDriver", "24:cast:producer:identity:pre-super:exception-order\n")
		})
	}
}

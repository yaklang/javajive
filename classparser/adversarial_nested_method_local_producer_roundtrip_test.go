package javaclassparser

import (
	"strings"
	"testing"
)

// Method formals shadow named-class formals, while a CHECKCAST must retain its
// failure position before NEW and superclass observation in the actual owner.
func TestAdversarialNestedMethodLocalCheckedProducerRoundTrip(t *testing.T) {
	for _, scope := range []string{"instance", "renamed", "static ancestor", "shadowed formals"} {
		t.Run(scope, func(t *testing.T) {
			source := `interface NestedFunction{Object apply(Object input);}
abstract class NestedProducerBase{static int trace,mode;static Object observed,published;static final RuntimeException failure=new IllegalArgumentException("identity");NestedProducerBase(){trace=trace*10+2;published=this;observed=get();if(mode==2)throw failure;}abstract CharSequence get();}
class NestedProducerOwner{class Level{class Inner{<T extends CharSequence> NestedProducerBase make(NestedFunction function,Object input,boolean branch){final T made=(T)function.apply(input);class Entry extends NestedProducerBase{T get(){return made;}}if(branch)return new Entry();return new Entry();}}}NestedProducerBase nested(NestedFunction function,Object input,boolean branch){return new Level().new Inner().make(function,input,branch);}}
class NestedProducerDriver{public static void main(String[] args)throws Exception{int count=0;Object[] inputs={null,"identity",new StringBuilder("builder"),new Object()};NestedFunction function=new NestedFunction(){public Object apply(Object input){NestedProducerBase.trace=NestedProducerBase.trace*10+1;if(NestedProducerBase.mode==1)throw NestedProducerBase.failure;return input;}};for(Object input:inputs)for(int mode=0;mode<3;mode++)for(boolean branch:new boolean[]{false,true}){NestedProducerBase.mode=mode;NestedProducerBase.trace=0;NestedProducerBase.observed=null;NestedProducerBase.published=null;Throwable failure=null;NestedProducerBase value=null;try{value=new NestedProducerOwner().nested(function,input,branch);}catch(Throwable e){failure=e;}boolean invalid=input!=null&&!(input instanceof CharSequence);if(mode==1){if(failure!=NestedProducerBase.failure||NestedProducerBase.trace!=1||NestedProducerBase.published!=null)throw new AssertionError("producer exception identity/order");}else if(invalid){if(failure==null||failure.getClass()!=ClassCastException.class||NestedProducerBase.trace!=1||NestedProducerBase.published!=null)throw new AssertionError("cast before NEW/super");}else{if(NestedProducerBase.trace!=12||NestedProducerBase.published==null||NestedProducerBase.observed!=input)throw new AssertionError("capture before callback");if(mode==2){if(failure!=NestedProducerBase.failure)throw new AssertionError("super failure identity");}else{if(failure!=null||value!=NestedProducerBase.published||value.get()!=input)throw new AssertionError("captured reference identity");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getEnclosingClass()!=NestedProducerOwner.Level.Inner.class||kind.getEnclosingMethod().getDeclaringClass()!=NestedProducerOwner.Level.Inner.class||!kind.getEnclosingMethod().getName().equals("make")||!kind.getName().equals("NestedProducerOwner$Level$Inner$1Entry")||kind.getDeclaredConstructors()[0].getParameterCount()!=2)throw new AssertionError("actual method owner and capture ABI");if(!kind.getDeclaredMethod("get").getGenericReturnType().equals(kind.getEnclosingMethod().getTypeParameters()[0]))throw new AssertionError("actual method binder identity");}}count++;}System.out.println(count+":nested:cast:producer:type-binder:identity:pre-super");}}`
			root := "NestedProducerOwner"
			switch scope {
			case "renamed":
				root = "OtherProducerScope"
				source = strings.ReplaceAll(source, "NestedProducerOwner", root)
			case "static ancestor":
				source = strings.Replace(source, "class Level", "static class Level", 1)
			case "shadowed formals":
				source = strings.Replace(source, "class NestedProducerOwner{", "class NestedProducerOwner<T extends Number>{", 1)
				source = strings.Replace(source, "class Level{", "class Level<T extends Number>{", 1)
				source = strings.Replace(source, "class Inner{", "class Inner<T extends Number>{", 1)
			}
			testNativeIndependentFamilyFixture(t, source, []string{root}, "NestedProducerDriver", "24:nested:cast:producer:type-binder:identity:pre-super\n")
		})
	}
}

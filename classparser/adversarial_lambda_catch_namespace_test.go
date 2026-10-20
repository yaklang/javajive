package javaclassparser

import "testing"

// A lambda body shares Java's enclosing local namespace. An ignored catch
// parameter is still a declaration, even when no expression reads its binder.
func TestAdversarialLambdaCatchDeclarationKeepsLexicalScope(t *testing.T) {
	const source = `import java.util.function.*;import java.lang.reflect.*;import java.util.*;
class CatchProbe {static int mode,calls,created;static Object last,consumer;static final RuntimeException FAILURE=new RuntimeException("callback");static final Error ERROR=new AssertionError("callback");static void accept(Object input){calls++;last=input;if(mode==1)throw FAILURE;if(mode==2)throw ERROR;}static void created(Consumer<Object> value,int code){if(code!=mode)throw new AssertionError("factory order");created++;consumer=value;}}
public class UncapturedLambdaNames {
 public static Consumer<Object> factory(int code){Consumer<Object> consumer=input->{try{CatchProbe.accept(input);}catch(Throwable originalFailure){}};CatchProbe.created(consumer,code);return consumer;}
}
class LambdaParameterDriver {
 public static void main(String[]args)throws Exception{Method implementation=null;for(Method m:UncapturedLambdaNames.class.getDeclaredMethods())if(m.isSynthetic()){if(implementation!=null||!Arrays.equals(m.getParameterTypes(),new Class<?>[]{Object.class})||m.getReturnType()!=void.class)throw new AssertionError("physical lambda");implementation=m;}if(implementation==null)throw new AssertionError("missing implementation");implementation.setAccessible(true);int rows=0;StringBuilder trace=new StringBuilder();Object token=new Object();for(int mode=0;mode<3;mode++)for(Object value:new Object[]{null,"text",token})for(int path=0;path<2;path++){CatchProbe.mode=mode;CatchProbe.calls=CatchProbe.created=0;CatchProbe.last=CatchProbe.consumer=null;Consumer<Object> consumer=UncapturedLambdaNames.factory(mode);if(CatchProbe.calls!=0||CatchProbe.created!=1||CatchProbe.consumer!=consumer)throw new AssertionError("eager callback");try{if(path==0)consumer.accept(value);else implementation.invoke(null,value);}catch(Throwable failure){throw new AssertionError("swallowed callback escaped",failure);}if(CatchProbe.calls!=1||CatchProbe.last!=value||CatchProbe.created!=1)throw new AssertionError("callback order/identity");trace.append(mode);rows++;}if(rows!=18)throw new AssertionError(rows);System.out.println("18:uncaptured:lambda:parameters:"+trace);}
}`
	for _, named := range []bool{false, true} {
		label := "absent"
		if named {
			label = "named"
		}
		t.Run(label, func(t *testing.T) { testUncapturedLambdaParameterMetadataRoundTrip(t, source, 1, "18", named) })
	}
}

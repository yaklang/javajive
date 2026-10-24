package javaclassparser

import (
	"strings"
	"testing"
)

// A concrete override can omit checked throws while its original symbolic
// interface/class owner still declares them. Narrowing the local's source type
// must not change that invocation contract or remove its original handler.
func TestAdversarialCheckedReceiverKeepsOriginalDeclaration(t *testing.T) {
	const source = `interface CheckedContract {void operation()throws java.io.IOException;}
class CheckedImplementation implements CheckedContract {public void operation(){CheckedEffects.work();}}
class CheckedEffects {
 static int mode,calls;static Object published;static Throwable caught;
 static final java.io.IOException checked=new java.io.IOException("same");static final RuntimeException unchecked=new IllegalStateException("same");
 static <E extends Throwable>void raise(Throwable failure)throws E{throw (E)failure;}
 static void work(){calls++;if(mode==1)CheckedEffects.<RuntimeException>raise(checked);if(mode==2)throw unchecked;}
 static void publish(Object value){published=value;}
}
public class CheckedReceiverOwner {
 public static int run(){CheckedContract callback=new CheckedImplementation();CheckedEffects.publish(callback);
  try{callback.operation();callback.operation();return 7;}catch(java.io.IOException caught){CheckedEffects.caught=caught;return 11;}
 }
}
class CheckedReceiverDriver {public static void main(String[]args){int rows=0;for(int mode=0;mode<3;mode++){
 CheckedEffects.mode=mode;CheckedEffects.calls=0;CheckedEffects.caught=null;CheckedEffects.published=null;Throwable failure=null;int result=-1;
 try{result=CheckedReceiverOwner.run();}catch(Throwable caught){failure=caught;}
 if(!(CheckedEffects.published instanceof CheckedContract)||CheckedEffects.calls!=(mode==0?2:1))throw new AssertionError("publication/call count");
 if(mode==0){if(result!=7||failure!=null||CheckedEffects.caught!=null)throw new AssertionError("normal");}
 else if(mode==1){if(result!=11||failure!=null||CheckedEffects.caught!=CheckedEffects.checked)throw new AssertionError("original checked handler");}
 else if(failure!=CheckedEffects.unchecked||CheckedEffects.caught!=null)throw new AssertionError("unchecked identity");rows++;
 }System.out.println(rows+":receiver:declared:throws:binding");}}
`
	for _, hierarchy := range []string{"interface", "class"} {
		t.Run(hierarchy, func(t *testing.T) {
			for _, shape := range []string{"no arguments", "competing overload", "covariant result"} {
				t.Run(shape, func(t *testing.T) {
					fixture := source
					if hierarchy == "class" {
						fixture = strings.Replace(fixture, "interface CheckedContract {void operation()throws java.io.IOException;}", "abstract class CheckedContract {public abstract void operation()throws java.io.IOException;}", 1)
						fixture = strings.Replace(fixture, "implements CheckedContract", "extends CheckedContract", 1)
					}
					if shape != "no arguments" {
						fixture = strings.Replace(fixture, "void operation()throws", "Object operation(Object value)throws", 1)
						resultType := "Object"
						if shape == "covariant result" {
							resultType = "String"
						}
						fixture = strings.Replace(fixture, "void operation(){CheckedEffects.work();}", resultType+` operation(Object value){CheckedEffects.work();return "same";}public String operation(String value){throw new AssertionError("wrong overload");}`, 1)
						fixture = strings.ReplaceAll(fixture, "callback.operation();", `if(!"same".equals(callback.operation("argument")))throw new AssertionError("result");`)
					}
					testNativePrivateSetterCompiledFixtureWithPolicies(t, "CheckedReceiverOwner", "CheckedReceiverDriver", "3:receiver:declared:throws:binding\n", func(t *testing.T, debug string) map[string][]byte {
						return nativeCompileSourceReleaseClasses(t, map[string]string{"CheckedReceiverOwner.java": fixture}, debug, "8")
					}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
				})
			}
		})
	}
}

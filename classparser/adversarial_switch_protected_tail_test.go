package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialProtectedSwitchDefaultReachesEffectfulReturn(t *testing.T) {
	const fixture = `
public class GuardedChoice{
 static final RuntimeException[] FAILURES={new IllegalStateException("identity"),new ClassCastException("identity"),new RuntimeException("identity")};StringBuilder trace=new StringBuilder();int calls,fail,flavor;
 void event(String text){trace.append(text);if(++calls==fail)throw FAILURES[flavor];}
 Object value(Object value,String text){event(text);return value;}
 Object select(boolean enabled,int code,Object input){try{event("S;");if(enabled){switch(code){case 0:return value(input,"A;");case 1:return value(null,"B;");}}return value(input,"C;");}catch(IllegalStateException error){event("H;");throw error;}catch(ClassCastException error){event("X;");throw error;}catch(RuntimeException error){event("Y;");throw error;}}
}
class GuardedDriver{public static void main(String[]args){int rows=0;Object token=new Object();
 for(boolean enabled:new boolean[]{false,true})for(int code:new int[]{-1,0,1,2,17})for(Object input:new Object[]{null,token})for(int flavor=0;flavor<3;flavor++)for(int fail=0;fail<4;fail++){
 String label=enabled&&code==0?"A;":enabled&&code==1?"B;":"C;";String handler=flavor==0?"H;":flavor==1?"X;":"Y;";String expected=fail==1?"S;"+handler:fail==2?"S;"+label+handler:"S;"+label;
 GuardedChoice target=new GuardedChoice();target.fail=fail;target.flavor=flavor;Object actual=null;Throwable error=null;try{actual=target.select(enabled,code,input);}catch(Throwable ex){error=ex;}
 if(fail==1||fail==2?error!=GuardedChoice.FAILURES[flavor]:error!=null||actual!=(enabled&&code==1?null:input))throw new AssertionError("return/exception "+enabled+" code="+code+" fail="+fail,error);
 if(!expected.equals(target.trace.toString()))throw new AssertionError("trace "+target.trace+" != "+expected);rows++;
 }System.out.println(rows+":switch:protected:shared-tail:identity:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "GuardedChoice"
			if spelling == "renamed" {
				text = strings.NewReplacer("GuardedChoice", "ProtectedRoute", "select(", "route(", "value(", "produce(", "event(", "record(").Replace(text)
				owner = "ProtectedRoute"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "GuardedDriver", "240:switch:protected:shared-tail:identity:failure\n")
		})
	}
}

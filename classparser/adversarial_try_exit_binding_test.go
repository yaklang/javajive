package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialTryExitKeepsCastDefinitionBeforeSlotReuse(t *testing.T) {
	const fixture = `
import java.util.*;import java.util.concurrent.Callable;
public class TryExit {
 final ArrayDeque<Collection<Object>> buffers=new ArrayDeque<Collection<Object>>();
 final Callable<?> factory;final StringBuilder trace=new StringBuilder();
 TryExit(Callable<?> source){factory=source;}
 Object process(boolean create,Object value){Collection<Object> buffer;
 if(create){PROTECTED_BODY trace.append("Q;");buffers.offer(buffer);}
 buffer=buffers.peek();if(buffer!=null){trace.append("A;");buffer.add(value);}return buffer;}
}
class TryExitDriver {public static void main(String[]args){int rows=0;final RuntimeException marker=new IllegalStateException("identity");Object token=new Object();
 for(boolean create:new boolean[]{false,true})for(int kind=0;kind<4;kind++)for(boolean prior:new boolean[]{false,true})for(Object value:new Object[]{null,token}){
 final List<Object> made=new ArrayList<Object>();final int flavor=kind;
 Callable<Object> factory=new Callable<Object>(){public Object call(){if(flavor==1)throw marker;return flavor==2?null:flavor==3?"wrong type":made;}};
 TryExit target=new TryExit(factory);List<Object> initial=new ArrayList<Object>();if(prior)target.buffers.add(initial);
 Object result=target.process(create,value);String trace;int size=prior?1:0;
 if(create&&kind!=0){if(kind==1?result!=marker:kind==2?!(result instanceof NullPointerException):!(result instanceof ClassCastException))throw new AssertionError("exception identity/category");trace=FAILURE_TRACE;}
 else {Object expected=prior?initial:create?made:null;trace=create?SUCCESS_TRACE:"";if(expected!=null){trace+="A;";if(((Collection<?>)expected).size()!=1||((List<?>)expected).get(0)!=value)throw new AssertionError("mutation");}if(result!=expected)throw new AssertionError("return identity");if(create)size++;}
 if(target.buffers.size()!=size||!trace.equals(target.trace.toString()))throw new AssertionError("trace/queue "+create+" kind="+kind+" prior="+prior+" trace="+target.trace+" expected="+trace);rows++;
 }System.out.println(rows+":try-exit:cast:identity:slot-reuse:effects");}}
`
	for _, region := range []string{"ordinary", "finally", "nested"} {
		t.Run(region, func(t *testing.T) {
			definition := "buffer=(Collection<Object>)Objects.requireNonNull(factory.call());"
			handler := "catch(Throwable error){trace.append(\"H;\");return error;}"
			body, success, failure := "try{"+definition+"}"+handler, "Q;", "H;"
			if region == "finally" {
				body += "finally{trace.append(\"F;\");}"
				success, failure = "F;Q;", "H;F;"
			}
			if region == "nested" {
				body = "try{try{" + definition + "}finally{trace.append(\"F;\");}}" + handler
				success, failure = "F;Q;", "F;H;"
			}
			variant := strings.NewReplacer("PROTECTED_BODY", body, "SUCCESS_TRACE", "\""+success+"\"", "FAILURE_TRACE", "\""+failure+"\"").Replace(fixture)
			for _, spelling := range []string{"original", "renamed"} {
				t.Run(spelling, func(t *testing.T) {
					text, owner := variant, "TryExit"
					if spelling == "renamed" {
						text = strings.NewReplacer("TryExit {", "ScopedResult {", "TryExit(", "ScopedResult(", "TryExit target", "ScopedResult target", "process(", "receive(").Replace(text)
						owner = "ScopedResult"
					}
					testAuthoredOwnerAcrossSourceModes(t, text, owner, "TryExitDriver", "32:try-exit:cast:identity:slot-reuse:effects\n")
				})
			}
		})
	}
}

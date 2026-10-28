package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The field table describes declarations, not <clinit>'s execution order.
// Loading each scenario through a fresh loader also checks initialization
// failure identity, partial effects and the JVM's erroneous-class state.
func TestAdversarialStaticStoresRespectExecutionOrder(t *testing.T) {
	const fixture = `
import java.util.*;
public class OrderedState {
 DECLARATIONS
 static {cache=createCache();result=createResult();tail=createTail();}
 static Map<String,Object> createCache(){
  InitTrace.events.append("C;");if(result!=null||tail!=null)throw new AssertionError("default state");InitTrace.fail(1);
  Map<String,Object> map=new HashMap<String,Object>();map.put("key",InitTrace.marker);return map;
 }
 static Object createResult(){InitTrace.events.append("R;");InitTrace.fail(2);if(tail!=null)throw new AssertionError("tail too early");return cache.get("key");}
 static Object createTail(){InitTrace.events.append("T;");InitTrace.fail(3);if(result!=InitTrace.marker)throw new AssertionError("result unavailable");return result;}
}
class InitTrace {
 public static int phase;public static StringBuilder events=new StringBuilder();
 public static final Object marker=new Object();public static final RuntimeException FAILURE=new IllegalArgumentException("identity");
 static void fail(int point){if(phase==point)throw FAILURE;}
}
class InitializationDriver {public static void main(String[]args)throws Exception{int rows=0;
 java.net.URL location=InitializationDriver.class.getProtectionDomain().getCodeSource().getLocation();
 for(int phase=0;phase<4;phase++)try(java.net.URLClassLoader loader=new java.net.URLClassLoader(new java.net.URL[]{location},null)){
  Class<?> trace=Class.forName("InitTrace",true,loader);java.lang.reflect.Field p=trace.getField("phase"),e=trace.getField("events"),f=trace.getField("FAILURE"),m=trace.getField("marker");p.setAccessible(true);e.setAccessible(true);f.setAccessible(true);m.setAccessible(true);p.setInt(null,phase);
  Throwable actual=null;Class<?> owner=null;try{owner=Class.forName("OrderedState",true,loader);}catch(Throwable error){actual=error;}
  String expected=phase==1?"C;":phase==2?"C;R;":"C;R;T;";
  if(!expected.equals(e.get(null).toString()))throw new AssertionError("phase="+phase+" effects="+e.get(null));
  if(phase!=0){if(!(actual instanceof ExceptionInInitializerError)||actual.getCause()!=f.get(null))throw new AssertionError("initial failure",actual);
   try{Class.forName("OrderedState",true,loader);throw new AssertionError("erroneous class retried");}catch(NoClassDefFoundError correct){}
   if(!expected.equals(e.get(null).toString()))throw new AssertionError("initialization replayed");
  }else{if(actual!=null)throw new AssertionError("initialization failed",actual);Object token=m.get(null);
   if(owner.getField("result").get(null)!=token||owner.getField("tail").get(null)!=token||((Map<?,?>)owner.getField("cache").get(null)).get("key")!=token)throw new AssertionError("publication identity");
   if(Class.forName("OrderedState",true,loader)!=owner||!expected.equals(e.get(null).toString()))throw new AssertionError("successful initialization replayed");
  }rows++;
 }System.out.println(rows+":initialization:order:defaults:failure:publication");}}
`
	fields := []string{"public static final Map<String,Object> cache;", "public static final Object result;", "public static final Object tail;"}
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			source := strings.Replace(fixture, "DECLARATIONS", fields[order[0]]+fields[order[1]]+fields[order[2]], 1)
			for _, spelling := range []string{"original", "renamed"} {
				t.Run(spelling, func(t *testing.T) {
					text, owner := source, "OrderedState"
					if spelling == "renamed" {
						text = strings.NewReplacer("OrderedState", "DeferredRegistry", "cache", "storage", "result", "published", "tail", "following").Replace(text)
						owner = "DeferredRegistry"
					}
					testAuthoredOwnerAcrossSourceModes(t, text, owner, "InitializationDriver", "4:initialization:order:defaults:failure:publication\n")
				})
			}
		})
	}
}

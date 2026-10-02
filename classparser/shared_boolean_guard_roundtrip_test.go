package javaclassparser

import (
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialBooleanReturnFoldRequiresTwoExplicitReturns(t *testing.T) {
	boolean := types.NewJavaPrimer(types.JavaBoolean)
	ctx := &class_context.ClassContext{FunctionType: types.NewJavaFuncType("()Z", nil, boolean)}
	ret := func(v values.JavaValue) statements.Statement { return statements.NewReturnStatement(v) }
	yes := values.NewJavaLiteral(true, boolean)
	no := values.NewJavaLiteral(false, boolean)
	for _, tc := range []struct {
		name            string
		then, otherwise []statements.Statement
		want            bool
	}{
		{"two explicit returns", []statements.Statement{ret(yes)}, []statements.Statement{ret(no)}, true},
		{"empty fallthrough", nil, []statements.Statement{ret(no)}, false},
		{"void return", []statements.Statement{ret(nil)}, []statements.Statement{ret(no)}, false},
		{"false leaf", []statements.Statement{ret(no)}, []statements.Statement{ret(yes)}, false},
		{"else fallthrough", []statements.Statement{ret(yes)}, nil, false},
		{"then effects", []statements.Statement{statements.NewExpressionStatement(yes), ret(yes)}, []statements.Statement{ret(no)}, false},
		{"else effects", []statements.Statement{ret(yes)}, []statements.Statement{statements.NewExpressionStatement(no), ret(no)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			branch := statements.NewIfStatement(yes, tc.then, tc.otherwise)
			if got := isBoolReturnIfElse(branch, ctx); got != tc.want {
				t.Fatalf("fold=%t, want %t", got, tc.want)
			}
		})
	}
}

func TestAdversarialSharedBooleanGuardBeforeEffectsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedBooleanGuard", `import java.util.*;
public class SharedBooleanGuard {
 final Map<String,String> forward=new LinkedHashMap<>(),reverse=new LinkedHashMap<>();
 String trace="";
 boolean remove(Object object){
  if(!(object instanceof Map.Entry))return false;
  Map.Entry entry=(Map.Entry)object;Object key=entry.getKey();
  if(forward.containsKey(key)){
   Object value=forward.get(key);
   if(value==null?entry.getValue()==null:value.equals(entry.getValue())){
    trace+="remove;";forward.remove(key);reverse.remove(value);return true;
   }
  }return false;
 }
 public static void main(String[] args){
  for(String stored:new String[]{null,"same","other"})for(String given:new String[]{null,"same","other"}){
   SharedBooleanGuard x=new SharedBooleanGuard();x.forward.put("key",stored);x.reverse.put(stored,"key");
   System.out.println(stored+":"+given+":"+x.remove(new AbstractMap.SimpleEntry<>("key",given))+":"+x.trace+":"+x.forward+":"+x.reverse);
  }
  SharedBooleanGuard x=new SharedBooleanGuard();System.out.println(x.remove(null)+":"+x.remove("bad")+":"+x.remove(new AbstractMap.SimpleEntry<>("missing",null)));
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialBooleanShortCircuitBeforeCachedClassRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BooleanCachedClass", `public class BooleanCachedClass {
 static Class cached;
 static String trace;
 static int faults;
 static final RuntimeException failure=new IllegalStateException("probe");
 static boolean enhanced(Class type){trace+="E;";if((faults&1)!=0)throw failure;return type.getName().indexOf("String")>=0;}
 static Class marker(){trace+="M;";if((faults&2)!=0)throw failure;return Integer.class;}
 static boolean accepts(Class type){return type!=null && enhanced(type) && type.getName().indexOf("Builder")<=0 || type==(cached==null?(cached=marker()):cached);}
 static String run(Class type){try{return "value:"+accepts(type);}catch(RuntimeException e){return "error:"+(e==failure);}}
 public static void main(String[] args){for(faults=0;faults<4;faults++)for(boolean initialized:new boolean[]{false,true})for(Class type:new Class[]{null,String.class,StringBuilder.class,Integer.class,Long.class}){
  cached=initialized?Integer.class:null;trace="";System.out.println(faults+":"+initialized+":"+type+":"+run(type)+":"+trace+":"+cached);
 }}
}`, Precision, Compatibility, "legacy")
}

// The cached result is a receiver as well as a compared operand. Its branch
// store must survive an outer short circuit and retain its reference type.
func TestAdversarialBooleanCachedReceiverRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "BooleanCachedReceiver", `public class BooleanCachedReceiver {
 static Class cached;static String trace;static boolean fault;static final RuntimeException failure=new IllegalStateException("probe");
 static Class marker(){trace+="M";if(fault)throw failure;return Number.class;}
 static boolean leading(Class type){return type==null || (cached==null?(cached=marker()):cached).isAssignableFrom(type);}
 static boolean guarding(Class type){return type!=null && (cached==null?(cached=marker()):cached).isAssignableFrom(type);}
 static boolean nested(Class type,boolean enabled){return enabled && type!=null && ((cached==null?(cached=marker()):cached).isAssignableFrom(type) || type==String.class);}
 static String run(Class type,int mode){try{return "value:"+(mode==0?leading(type):mode==1?guarding(type):nested(type,mode==2));}catch(RuntimeException e){return "error:"+(e==failure);}}
 public static void main(String[] args){for(boolean failureMode:new boolean[]{false,true})for(boolean initialized:new boolean[]{false,true})for(Class type:new Class[]{null,String.class,Integer.class,Number.class})for(int mode=0;mode<4;mode++){fault=failureMode;cached=initialized?Number.class:null;trace="";System.out.println(failureMode+":"+initialized+":"+type+":"+mode+":"+run(type,mode)+":"+trace+":"+cached);}}
}`, Precision, Compatibility, "legacy")
}

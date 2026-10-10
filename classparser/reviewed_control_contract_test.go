package javaclassparser

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Validate handler multiplicity against original exception-table identities.
// Several protected ranges may lead to the same handler; count that handler
// once, while keeping multiple handlers for the same exception type distinct.
// This rejects invented/duplicated catches without pinning generated local IDs.
func assertReviewedHandlerMultiplicity(t *testing.T, raw []byte, switchName string) {
	t.Helper()
	original, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&original.ConstantPool)
	want := map[string]int{}
	for _, method := range original.Methods {
		seen := map[string]bool{}
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			for _, entry := range code.ExceptionTable {
				if entry.CatchType == 0 {
					t.Fatal("catch-all cleanup requires a separate oracle")
				}
				key := fmt.Sprintf("%d:%d", entry.HandlerPc, entry.CatchType)
				if seen[key] {
					continue
				}
				seen[key] = true
				name := cp.GetClassName(int(entry.CatchType))
				want[name[strings.LastIndex(name, "/")+1:]]++
			}
		}
	}
	catch := regexp.MustCompile(`catch\s*\(\s*([\w.$]+(?:\s*\|\s*[\w.$]+)*)\s+\w+\s*\)`)
	for _, setting := range []string{"", "1"} {
		t.Setenv(switchName, setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, match := range catch.FindAllStringSubmatch(source, -1) {
			for _, name := range strings.Split(match[1], "|") {
				name = strings.TrimSpace(name)
				got[name[strings.LastIndex(name, ".")+1:]]++
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s=%q: handler multiplicities %v differ from original %v\n%s", switchName, setting, got, want, source)
		}
	}
}

// Fixture counts were independently read with javap, not inferred from generated
// Java. Each constructor containing this(...) must start with that invocation.
// Behavioral evidence for evaluation order lives in the trusted MVP below.
func assertReviewedConstructorDelegations(t *testing.T, path, className, switchName string, count int, required ...string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	declaration := regexp.MustCompile(`^\s*(?:(?:public|protected|private)\s+)?` + regexp.QuoteMeta(className) + `\([^{};]*\)(?:\s+throws[^{}]*)?\s*\{\s*$`)
	call := regexp.MustCompile(`(?m)^\s*this\(`)
	for _, setting := range []string{"", "1"} {
		t.Setenv(switchName, setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(source, "\n")
		seen := 0
		for i, line := range lines {
			if !declaration.MatchString(line) {
				continue
			}
			_, end := methodBodyRange(lines, i)
			if end <= i {
				t.Fatalf("constructor body unavailable: %s", line)
			}
			body := strings.Join(lines[i+1:end-1], "\n")
			if !call.MatchString(body) {
				continue
			}
			seen++
			if !strings.HasPrefix(strings.TrimSpace(body), "this(") {
				t.Fatalf("%s=%q: declaration or effect precedes delegation:\n%s", switchName, setting, body)
			}
			if len(call.FindAllStringIndex(body, -1)) != 1 {
				t.Fatalf("multiple delegations in one constructor:\n%s", body)
			}
		}
		if seen != count {
			t.Fatalf("%s=%q: found %d delegated constructors, original has %d\n%s", switchName, setting, seen, count, source)
		}
		for _, fragment := range required {
			if !strings.Contains(source, fragment) {
				t.Fatalf("%s=%q: original effect/argument %q lost\n%s", switchName, setting, fragment, source)
			}
		}
	}
}

func TestAdversarialReviewedConstructorOrderRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_COMMONS_IO_REMAINING_OFF", "JDEC_FREEMARKER_REMAINING_OFF", "JDEC_POOL2_REMAINING_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "ReviewedDelegation", `import java.util.*;
class DelegationEffects {
 static String trace="";
 static String mark(String value,int fail,int at){trace+=value+";";if(fail==at)throw new IllegalArgumentException(value);return value;}
 static String[] copy(String[] values){trace+="copy;";return values.clone();}
}
public class ReviewedDelegation {
 final String tag;final String[] values;
 ReviewedDelegation(String tag,String... values){DelegationEffects.trace+="target;";this.tag=tag;this.values=DelegationEffects.copy(values);}
 ReviewedDelegation(int fail){this(DelegationEffects.mark("first",fail,1),new String[]{DelegationEffects.mark("second",fail,2),DelegationEffects.mark("third",fail,3)});DelegationEffects.trace+="body;";}
 ReviewedDelegation(Collection<String> input){this("collection",input.toArray(new String[0]));DelegationEffects.trace+="iterate;";for(String value:input)if(value==null)throw new NullPointerException();}
 public static void main(String[] args){
  for(int fail=0;fail<5;fail++){DelegationEffects.trace="";try{ReviewedDelegation x=new ReviewedDelegation(fail);System.out.println(x.tag+":"+Arrays.toString(x.values));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+e.getMessage());}System.out.println(DelegationEffects.trace);}
  for(Collection<String> input:Arrays.asList(Collections.<String>emptyList(),Arrays.asList("x","y"),Arrays.asList("x",null))){DelegationEffects.trace="";try{ReviewedDelegation x=new ReviewedDelegation(input);System.out.println(Arrays.toString(x.values));}catch(Throwable e){System.out.println(e.getClass().getName());}System.out.println(DelegationEffects.trace);}
 }
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialReviewedHandlerScopeRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_ALREADY_CAUGHT_OFF", "JDEC_ASSERTJ_REMAINING_OFF", "JDEC_FREEMARKER_REMAINING_OFF", "JDEC_HARDJAR_SHAPE_OFF", "JDEC_WRAP_GETCONSTRUCTOR_OFF", "JDEC_FIX_TRY_BREAK_RETURN_OFF"} {
		t.Setenv(key, "1")
	}
	roundTripGenericFlow(t, "ReviewedHandlers", `import java.util.concurrent.*;import java.lang.reflect.*;import java.security.*;
class HandlerEffects {
 static String trace="";static final Object identity=new Object();
 static Object value(int kind)throws Exception{trace+="value;";if(kind==1)throw new Exception("checked");if(kind==2)throw new PrivilegedActionException(new IllegalStateException("nested"));if(kind==3)throw new AssertionError("error");return identity;}
 static String attempt(int kind,int at)throws Exception{trace+="attempt"+at+";";if(at<kind)throw new CancellationException();if(kind==3)throw new ExecutionException(new IllegalStateException("cause"));if(kind==4)throw new AssertionError("retry-error");return "done";}
 public static class Failing {public Failing(){throw new IllegalArgumentException("ctor");}}
 public static abstract class Abstract {public Abstract(){}}
}
public class ReviewedHandlers {
 static Object unwrap(int kind){try{return HandlerEffects.value(kind);}catch(Exception e){if(e instanceof PrivilegedActionException)e=((PrivilegedActionException)e).getException();HandlerEffects.trace+="handled:"+e.getClass().getName()+";";return e;}}
 static Object construct(String name){try{return Class.forName(name).getDeclaredConstructor(new Class[0]).newInstance(new Object[0]);}catch(NoSuchMethodException|IllegalAccessException|InstantiationException|InvocationTargetException e){HandlerEffects.trace+="reflect:"+e.getClass().getName()+";";return e;}catch(ClassNotFoundException e){return e;}}
 static String retry(int kind)throws Exception{int attempt=0;while(true){try{return HandlerEffects.attempt(kind,attempt++);}catch(CancellationException e){HandlerEffects.trace+="retry;";}catch(ExecutionException e){throw new RuntimeException(e.getCause());}}}
 public static void main(String[] args){
  for(int kind=0;kind<4;kind++){HandlerEffects.trace="";try{Object result=unwrap(kind);System.out.println((result==HandlerEffects.identity)+":"+result.getClass().getName());}catch(Throwable e){System.out.println("escape:"+e.getClass().getName());}System.out.println(HandlerEffects.trace);}
  for(String name:new String[]{"java.lang.Object","java.lang.Integer","java.lang.System","HandlerEffects$Failing","HandlerEffects$Abstract","no.such.Type"}){HandlerEffects.trace="";Object result=construct(name);System.out.println(result.getClass().getName()+":"+HandlerEffects.trace);}
  for(int kind=0;kind<5;kind++){HandlerEffects.trace="";try{System.out.println(retry(kind));}catch(Throwable e){System.out.println(e.getClass().getName()+":"+(e.getCause()==null?"none":e.getCause().getClass().getName()));}System.out.println(HandlerEffects.trace);}
 }
}`, Precision, Compatibility, "legacy")
}

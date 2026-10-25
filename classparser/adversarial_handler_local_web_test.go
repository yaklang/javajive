package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The unchanged driver observes both the value chosen at the join and the
// order/identity of exceptions. Reusing a JVM slot is not evidence that the
// branch temporary and the later return temporary are one source variable.
const handlerLocalWebFixture = `
class WebEffects {static String trace="";static int mode;static Object seen;static final RuntimeException first=new RuntimeException("first"),last=new RuntimeException("last");}
interface WebResult {int value();}
interface WebInput {int read();}
class WebNumber implements WebInput {public int read(){return 31;}}
class WebWrapped implements WebResult {final WebInput input;WebWrapped(WebInput x){input=x;}public int value(){return input.read()+5;}}
class WebDirect implements WebResult {public int value(){return -19;}}
class WebResource implements AutoCloseable {
 void consume(java.util.Map x,long n){WebEffects.trace+="M";x.put("n",Long.valueOf(n));}
 void consume(java.util.List x){WebEffects.trace+="L";x.add("value");}
 public void close(){WebEffects.trace+="C";if(WebEffects.mode==2)throw WebEffects.last;}
}
class WebOwner {
 static Object create(boolean choice){WebEffects.trace+="N";if(WebEffects.mode==1)throw WebEffects.first;return choice?new WebNumber():new WebDirect();}
 static WebResult branch(boolean choice,boolean publish){
  WebResult selected;
  try {if(choice){WebInput input=(WebInput)create(true);selected=new WebWrapped(input);}else{selected=(WebResult)create(false);}}
  catch(RuntimeException failure){WebEffects.trace+="H";throw failure;}
  if(publish){WebEffects.trace+="P";WebEffects.seen=selected;if(WebEffects.mode==2)throw WebEffects.last;}
  return selected;
 }
 static WebResult copied(boolean choice,boolean publish){
  WebResult result=null;
  try {WebResult selected;if(choice){WebInput input=(WebInput)create(true);selected=new WebWrapped(input);}else{selected=(WebResult)create(false);}result=selected;}
  catch(RuntimeException failure){WebEffects.trace+="H";throw failure;}
  if(publish){WebEffects.trace+="P";WebEffects.seen=result;if(WebEffects.mode==2)throw WebEffects.last;}
  return result;
 }
 static Object resource(boolean choice){
  try(WebResource r=new WebResource()){
   Object result;
   if(choice){java.util.HashMap item=new java.util.HashMap();r.consume(item,23L);result=item;}
   else{java.util.ArrayList item=new java.util.ArrayList();r.consume(item);result=item;}
   if(WebEffects.mode==1)throw WebEffects.first;
   return result;
  }
 }
}
class WebDriver {public static void main(String[]args){int rows=0;
 for(boolean copy:new boolean[]{false,true})for(boolean choice:new boolean[]{false,true})for(boolean publish:new boolean[]{false,true})for(int mode=0;mode<3;mode++){
  WebEffects.trace="";WebEffects.mode=mode;WebEffects.seen=null;RuntimeException caught=null;WebResult result=null;
  try{result=copy?WebOwner.copied(choice,publish):WebOwner.branch(choice,publish);}catch(RuntimeException e){caught=e;}
  RuntimeException want=mode==1?WebEffects.first:mode==2&&publish?WebEffects.last:null;
  if(caught!=want||!WebEffects.trace.equals(mode==1?"NH":publish?"NP":"N"))throw new AssertionError("branch effects: "+WebEffects.trace);
  if(want==null&&(result.value()!=(choice?36:-19)||publish&&WebEffects.seen!=result))throw new AssertionError("branch value/identity");rows++;
 }
 for(boolean choice:new boolean[]{false,true})for(int mode=0;mode<3;mode++){
  WebEffects.trace="";WebEffects.mode=mode;RuntimeException caught=null;Object result=null;
  try{result=WebOwner.resource(choice);}catch(RuntimeException e){caught=e;}
  RuntimeException want=mode==1?WebEffects.first:mode==2?WebEffects.last:null;
  if(caught!=want||!WebEffects.trace.equals(choice?"MC":"LC"))throw new AssertionError("resource effects");
  if(mode==0&&(choice?!Long.valueOf(23).equals(((java.util.Map)result).get("n")):!"value".equals(((java.util.List)result).get(0))))throw new AssertionError("resource value");rows++;
 }System.out.println(rows+":local-web:join:handler:resource");}}
`

func TestAdversarialHandlerContinuationLocalWeb(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, owner := range []string{"WebOwner", "RenamedScopeOwner"} {
			t.Run(string(profile)+"/"+owner, func(t *testing.T) {
				fixture := strings.ReplaceAll(handlerLocalWebFixture, "WebOwner", owner)
				testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, "WebDriver", "30:local-web:join:handler:resource\n", func(t *testing.T, debug string) map[string][]byte {
					javac := os.Getenv("JAVA8_JAVAC")
					if javac == "" {
						t.Fatal("native compiler required")
					}
					if profile == ModernJavac {
						javac = modern
					}
					return compileLambdaParameterMetadataFixture(t, javac, profile, fixture, owner, debug, false)
				}, nil, []string{"normal"})
			})
		}
	}
}

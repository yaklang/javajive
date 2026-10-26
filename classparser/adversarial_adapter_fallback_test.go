package javaclassparser

import (
	"strings"
	"testing"
)

// Independent adapter and text capabilities can overlap. A rejected adapter
// result must retain the protected fallback, its cleanup, and selected identity.
func TestAdversarialAdapterFailurePreservesIndependentFallback(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
interface Adapter{Object adapt();} interface Text{String text();}
public class AdaptedChoice {
 public static final RuntimeException ADAPT=new RuntimeException("adapt"),MISSING=new RuntimeException("missing");
 public static int reads,parses,consumes,closes;public static Object retained;
 public static Integer parse(String text){parses++;return Integer.valueOf(text);}
 public static int choose(Object input){Integer result;
  selected:{if(input instanceof Adapter){Object candidate=((Adapter)input).adapt();if(candidate instanceof Integer){result=(Integer)candidate;break selected;}}
   if(input instanceof Text){reads++;try{result=parse(((Text)input).text());}catch(NumberFormatException failure){throw new IllegalArgumentException("wrapped",failure);}__CLEANUP__}else throw MISSING;
  }
  consumes++;retained=result;return result.intValue();
 }
}
class AdaptedChoiceDriver {
 static class A implements Adapter{int mode,reads;Integer value;A(int m,int v){mode=m;value=new Integer(v);}public Object adapt(){reads++;if(mode==0)return value;if(mode==1)return null;if(mode==2)return "not a number";if(mode==3)throw AdaptedChoice.ADAPT;return new Object();}}
 static class Dual extends A implements Text{String word;int texts;Dual(int m,int v,String s){super(m,v);word=s;}public String text(){texts++;return word;}}
 static class TextOnly implements Text{String word;int texts;TextOnly(String s){word=s;}public String text(){texts++;return word;}}
 public static void main(String[]args){int rows=0;
 for(int kind=0;kind<5;kind++)for(int mode=0;mode<5;mode++)for(int v:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})for(boolean bad:new boolean[]{false,true}){
  String word=bad?"bad":Integer.toString(v^21930);Object input=kind==0?new Dual(mode,v,word):kind==1?new A(mode,v):kind==2?new TextOnly(word):kind==3?new Object():null;
  AdaptedChoice.reads=AdaptedChoice.parses=AdaptedChoice.consumes=AdaptedChoice.closes=0;AdaptedChoice.retained=null;
  boolean adapter=kind<=1,text=kind==0||kind==2,selected=adapter&&mode==0,adaptFailed=adapter&&mode==3,fallback=!selected&&!adaptFailed&&text,missing=!selected&&!adaptFailed&&!text,parseFailed=fallback&&bad;
  RuntimeException failure=null;int actual=0;try{actual=AdaptedChoice.choose(input);}catch(RuntimeException ex){failure=ex;}
  if(adaptFailed){if(failure!=AdaptedChoice.ADAPT)throw new AssertionError("adapter failure");}
  else if(missing){if(failure!=AdaptedChoice.MISSING)throw new AssertionError("missing failure");}
  else if(parseFailed){if(!(failure instanceof IllegalArgumentException)||!(failure.getCause() instanceof NumberFormatException))throw new AssertionError("parse failure");}
  else if(failure!=null||actual!=(selected?v:v^21930))throw new AssertionError("selected result");
  if(adapter&&((A)input).reads!=1)throw new AssertionError("adapter visits");
  if(AdaptedChoice.reads!=(fallback?1:0)||AdaptedChoice.parses!=(fallback?1:0)||AdaptedChoice.consumes!=(failure==null?1:0))throw new AssertionError("effects");
  if(AdaptedChoice.closes!=(fallback?__CLOSES__:0))throw new AssertionError("cleanup count");
  if(selected&&AdaptedChoice.retained!=((A)input).value)throw new AssertionError("selected identity");rows++;
 }System.out.println(rows+":adapter:fallback");}
}
`
	for _, cleanup := range []bool{false, true} {
		mode := "catch"
		suffix := ""
		closes := "0"
		if cleanup {
			mode = "catch-finally"
			suffix = "finally{closes++;}"
			closes = "1"
		}
		text := strings.ReplaceAll(strings.ReplaceAll(source, "__CLEANUP__", suffix), "__CLOSES__", closes)
		t.Run(mode, func(t *testing.T) {
			for _, name := range []string{"AdaptedChoice", "ConversionRoute"} {
				t.Run(name, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "AdaptedChoice", name), name, name+"Driver", "200:adapter:fallback\n")
				})
			}
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedBooleanWordKeepsBothFrontiers(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class WordDecision {
 public static char initial(String text,StringBuilder trace,boolean fail,RuntimeException marker){trace.append('c');if(fail)throw marker;return text.charAt(0);}
 public static int classify(DecisionInput input,StringBuilder trace,boolean fail,RuntimeException marker){
  char initial;WORDTYPE wide=(input.kind==5||input.kind==6||(input.kind==17&&((initial=initial(input.text,trace,fail,marker))=='J'||initial=='D')))?TRUEWORD:FALSEWORD;
  int result=RESULT;trace.append(result);return result;
 }
}
class DecisionInput {int kind;String text;}
class WordDecisionDriver {
 public static void main(String[]args){int rows=0;
  for(int kind=-1;kind<=30;kind++)for(String text:new String[]{"J","D","I","","x","JJ",null})for(boolean fail:new boolean[]{false,true}){
   DecisionInput input=new DecisionInput();input.kind=kind;input.text=text;
   boolean read=kind==17;boolean abrupt=read&&(fail||text==null||text.length()==0);
   int wanted=(kind==5||kind==6||read&&!abrupt&&(text.charAt(0)=='J'||text.charAt(0)=='D'))?TRUEEXPECTED:FALSEEXPECTED;
   String expected=(read?"c":"")+(abrupt?"":Integer.toString(wanted));StringBuilder trace=new StringBuilder();RuntimeException marker=new RuntimeException("marker");
   try{int got=WordDecision.classify(input,trace,fail,marker);if(abrupt||got!=wanted)throw new AssertionError("kind="+kind+" value="+got+" expected="+wanted);}
   catch(RuntimeException caught){if(!abrupt||(fail?caught!=marker:text==null?!(caught instanceof NullPointerException):!(caught instanceof StringIndexOutOfBoundsException)))throw new AssertionError("exception",caught);}
   if(!trace.toString().equals(expected))throw new AssertionError("trace="+trace+" expected="+expected);rows++;
  }System.out.println(rows+":boolean-frontiers:selected-word:effects");
 }
}
`
	for _, domain := range []string{"boolean", "integer"} {
		text := source
		if domain == "boolean" {
			text = strings.NewReplacer("WORDTYPE", "boolean", "TRUEWORD", "true", "FALSEWORD", "false", "RESULT", "wide?2:1", "TRUEEXPECTED", "2", "FALSEEXPECTED", "1").Replace(text)
		} else {
			text = strings.NewReplacer("WORDTYPE", "int", "TRUEWORD", "-37", "FALSEWORD", "91", "RESULT", "wide", "TRUEEXPECTED", "-37", "FALSEEXPECTED", "91").Replace(text)
		}
		for _, name := range []string{"WordDecision", "AlternativeClassifier"} {
			t.Run(domain+"/"+name, func(t *testing.T) {
				testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(text, "WordDecision", name), name, name+"Driver", "448:boolean-frontiers:selected-word:effects\n")
			})
		}
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialPartialDecisionKeepsItsBypassContinuation(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class PartialDecision {
 public static String execute(int[] values,int depth,boolean gate){StringBuilder trace=new StringBuilder();
 for(int x:values){stage:{if(gate){int code;
 if(x==0)code=65;else if(x==1)code=0;else if(x==2)code=1;else if(x==3)code=127;else if(x==4)code=255;else if(x==5)code=256;else if(x==6)code=55296;else if(x==7)code=65535;else break stage;
 char[] word=new char[depth+1];for(int j=0;j<depth;j++)word[j]='[';word[depth]=(char)code;trace.append('W').append(word.length).append(':').append((int)word[depth]).append(';');}}
 trace.append('T').append(x).append(';');}return trace.toString();}
}
class PartialDecisionDriver{public static void main(String[]args){int rows=0;int[] codes={65,0,1,127,255,256,55296,65535};
for(int size=0;size<17;size++)for(int seed=0;seed<11;seed++)for(int depth=0;depth<4;depth++)for(boolean gate:new boolean[]{false,true}){int[] values=new int[size];StringBuilder expected=new StringBuilder();for(int i=0;i<size;i++){int x=(i*5+seed)%12-2;values[i]=x;if(gate&&x>=0&&x<codes.length)expected.append('W').append(depth+1).append(':').append(codes[x]).append(';');expected.append('T').append(x).append(';');}String actual=PartialDecision.execute(values,depth,gate);if(!actual.equals(expected.toString()))throw new AssertionError("decision bypass size="+size+" seed="+seed+" actual="+actual+" expected="+expected);rows++;}System.out.println(rows+":partial-decision:bypass:loop:effects");}}
`
	for _, shape := range []string{"guarded", "unguarded", "effectful bypass"} {
		t.Run(shape, func(t *testing.T) {
			text := source
			switch shape {
			case "unguarded":
				text = strings.ReplaceAll(text, "stage:{if(gate){int code;", "stage:{{int code;")
				text = strings.ReplaceAll(text, "if(gate&&x>=0", "if(x>=0")
			case "effectful bypass":
				text = strings.ReplaceAll(text, "else break stage;", "else {trace.append('B').append(x).append(';');break stage;}")
				text = strings.ReplaceAll(text, "expected.append('T').append(x)", "if(gate&&(x<0||x>=codes.length))expected.append('B').append(x).append(';');expected.append('T').append(x)")
			}
			for _, name := range []string{"PartialDecision", "ClassifyWords"} {
				t.Run(name, func(t *testing.T) {
					renamed := strings.ReplaceAll(text, "PartialDecision", name)
					testAuthoredOwnerAcrossSourceModes(t, renamed, name, name+"Driver", "1496:partial-decision:bypass:loop:effects\n")
				})
			}
		})
	}
}

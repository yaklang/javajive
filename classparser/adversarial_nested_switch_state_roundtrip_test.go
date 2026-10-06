package javaclassparser

import (
	"strings"
	"testing"
)

// The independent driver exercises a state transition whose inner switch and
// enclosing conditional share the outer switch exit. A real fall-through is
// included to distinguish that edge from the shared exit.
const nestedSwitchStateFixture = `class NestedStateScanner {
 static String scan(String pattern){java.util.List<String> pieces=new java.util.ArrayList<String>();parse(pattern,pieces,false,false,false);StringBuilder out=new StringBuilder();for(String piece:pieces)out.append(piece);return out.toString();}
 enum State{LITERAL,CONVERTER,MIN,DOT,MAX}
 static void parse(String pattern,java.util.List<String> pieces,boolean left,boolean truncate,boolean pad){StringBuilder literal=new StringBuilder(32);int length=pattern.length();State state=State.LITERAL;int i=0;int width=0;while(i<length){char c=pattern.charAt(i++);switch(state){case LITERAL:if(i==length){literal.append(c);break;}else{if(c=='%'){switch(pattern.charAt(i)){case '%':literal.append(c);i++;break;default:if(literal.length()!=0)pieces.add(literal.toString());literal.setLength(0);literal.append(c);state=State.CONVERTER;width=0;break;}}else{literal.append(c);break;}}break;
 case CONVERTER:literal.append(c);switch(c){case '0':pad=true;break;case '-':left=true;break;case '.':state=State.DOT;break;default:if(c>='0'&&c<='9'){width=c-'0';state=State.MIN;break;}else{i=finish(c,pattern,i,literal,width,pieces,left,truncate,pad);state=State.LITERAL;width=0;literal.setLength(0);break;}}break;
 case MIN:literal.append(c);if(c>='0'&&c<='9'){width=width*10+c-'0';break;}else{if(c=='.'){state=State.DOT;break;}else{i=finish(c,pattern,i,literal,width,pieces,left,truncate,pad);state=State.LITERAL;width=0;literal.setLength(0);break;}}
 case DOT:literal.append(c);switch(c){case '-':truncate=false;break;default:if(c>='0'&&c<='9'){width=c-'0';state=State.MAX;break;}else{state=State.LITERAL;break;}}break;
 case MAX:literal.append(c);if(c>='0'&&c<='9'){width=width*10+c-'0';break;}else{i=finish(c,pattern,i,literal,width,pieces,left,truncate,pad);state=State.LITERAL;width=0;literal.setLength(0);break;}
 }}if(literal.length()!=0)pieces.add(literal.toString());}
 static int finish(char c,String text,int i,StringBuilder literal,int width,java.util.List<String> pieces,boolean left,boolean truncate,boolean pad){if(!Character.isUnicodeIdentifierStart(c))throw new AssertionError("conversion:"+c);StringBuilder name=new StringBuilder();name.append(c);while(i<text.length()&&Character.isUnicodeIdentifierPart(text.charAt(i))){name.append(text.charAt(i));literal.append(text.charAt(i++));}pieces.add("["+name+"]");return i;}

 static String falling(int a,int b){StringBuilder out=new StringBuilder();switch(a){case 0:if(b<0){out.append('N');break;}else{switch(b){case 0:out.append('Z');break;default:out.append('P');break;}}case 1:out.append('F');break;default:out.append('D');}return out.toString();}
}
class NestedStateDriver{public static void main(String[]args){String[] input={"","a","%","%%","%m","x%m","%m%p","%%%m","%m%%tail"};String[] want={"","a","%","%","[m]","x[m]","[m][p]","%[m]","[m]%tail"};int rows=0;for(int i=0;i<input.length;i++){String got=NestedStateScanner.scan(input[i]);if(!got.equals(want[i]))throw new AssertionError(input[i]+":"+got+":"+want[i]);rows++;}for(int a=-1;a<3;a++)for(int b=-1;b<3;b++){String expected=a==0?(b<0?"N":b==0?"ZF":"PF"):a==1?"F":"D";if(!NestedStateScanner.falling(a,b).equals(expected))throw new AssertionError("intentional fall-through");rows++;}System.out.println(rows+":nested-switch:shared-exit:real-fallthrough");}}
`

func TestAdversarialConditionalNestedSwitchStateRoundTrip(t *testing.T) {
	for _, owner := range []string{"NestedStateScanner", "RenamedStateScanner"} {
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterFixture(t, strings.ReplaceAll(nestedSwitchStateFixture, "NestedStateScanner", owner), owner, "NestedStateDriver", "25:nested-switch:shared-exit:real-fallthrough\n")
		})
	}
}

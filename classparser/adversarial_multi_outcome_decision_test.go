package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialMultipleOutcomeDecisionPreservesSelectedEffects(t *testing.T) {
	const fixture = `
public class MultiOutcome {
 static final RuntimeException FAILURE=new IllegalStateException("identity");
 final Object[] tokens={new Object(),new Object(),new Object()};StringBuilder events=new StringBuilder();int fail;boolean nullable;
 boolean check(int mask,int bit){events.append("P").append(bit).append(";");if(fail==bit+1)throw FAILURE;return (mask&(1<<bit))!=0;}
 Object value(int index){events.append("V").append(index).append(";");if(fail==index+5)throw FAILURE;return nullable?null:tokens[index];}
 Object select(int mask){return check(mask,0)?(check(mask,1)?value(0):value(2)):(check(mask,2)&&check(mask,3)?value(1):value(2));}
}
class OutcomeDriver{public static void main(String[]args){int rows=0;for(int mask=0;mask<16;mask++)for(int fail=0;fail<8;fail++)for(boolean nullable:new boolean[]{false,true}){
 MultiOutcome target=new MultiOutcome();target.fail=fail;target.nullable=nullable;StringBuilder expected=new StringBuilder();boolean thrown=false;int selected=-1;
 int[] visited=(mask&1)!=0?new int[]{0,1}:(mask&4)!=0?new int[]{0,2,3}:new int[]{0,2};
 for(int bit:visited){expected.append("P").append(bit).append(";");if(fail==bit+1){thrown=true;break;}}
 if(!thrown){if((mask&1)!=0)selected=(mask&2)!=0?0:2;else selected=(mask&12)==12?1:2;expected.append("V").append(selected).append(";");thrown=fail==selected+5;}
 Object actual=null;Throwable error=null;try{actual=target.select(mask);}catch(Throwable ex){error=ex;}
 if(thrown?error!=MultiOutcome.FAILURE:error!=null||actual!=(nullable?null:target.tokens[selected]))throw new AssertionError("selected mask="+mask+" failure="+fail,error);
 if(!expected.toString().equals(target.events.toString()))throw new AssertionError("trace mask="+mask+" failure="+fail+" "+target.events+" expected="+expected);rows++;
 }System.out.println(rows+":decision:multiple:identity:effects:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "MultiOutcome"
			if spelling == "renamed" {
				text = strings.NewReplacer("MultiOutcome", "ChoiceRouter", "select(", "dispatch(", "check(", "observe(", "value(", "fetch(").Replace(text)
				owner = "ChoiceRouter"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "OutcomeDriver", "256:decision:multiple:identity:effects:failure\n")
		})
	}
}

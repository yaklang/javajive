package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSharedCallWithNestedOperandDecision(t *testing.T) {
	const fixture = `
enum RoutePhase { RECEIVE,SEND,DONE,IDLE }
public class RouteDecision {
 int failure;boolean nullable;StringBuilder events=new StringBuilder();
 static final RuntimeException FAILURE=new IllegalStateException("identity");
 RoutePhase advance(){events.append("A;");if(failure==1)throw FAILURE;return nullable?null:RoutePhase.SEND;}
 RoutePhase status(){events.append("Q;");if(failure==2)throw FAILURE;return nullable?null:RoutePhase.IDLE;}
 RoutePhase settle(RoutePhase phase){events.append("S;");if(failure==3)throw FAILURE;return phase;}
 RoutePhase decide(RoutePhase phase,int incoming,int outgoing){return (phase==RoutePhase.RECEIVE&&outgoing>0)||(phase==RoutePhase.SEND&&incoming>0)?advance():settle(phase!=RoutePhase.DONE?status():RoutePhase.DONE);}
}
class RouteDecisionDriver {public static void main(String[]args){int rows=0;
 for(RoutePhase phase:new RoutePhase[]{null,RoutePhase.RECEIVE,RoutePhase.SEND,RoutePhase.DONE,RoutePhase.IDLE})for(int incoming:new int[]{-1,0,1})for(int outgoing:new int[]{-1,0,1})for(int failure=0;failure<4;failure++)for(boolean nullable:new boolean[]{false,true}){
  RouteDecision x=new RouteDecision();x.failure=failure;x.nullable=nullable;
  boolean advance=phase==RoutePhase.RECEIVE?outgoing>0:phase==RoutePhase.SEND&&incoming>0;
  boolean query=!advance&&phase!=RoutePhase.DONE;
  boolean thrown=advance&&failure==1||query&&failure==2||!advance&&failure==3;
  String trace=advance?"A;":(query?"Q;":"")+(query&&failure==2?"":"S;");
  RoutePhase expected=advance?(nullable?null:RoutePhase.SEND):phase==RoutePhase.DONE?RoutePhase.DONE:nullable?null:RoutePhase.IDLE;
  RoutePhase actual=null;Throwable caught=null;try{actual=x.decide(phase,incoming,outgoing);}catch(Throwable error){caught=error;}
  if(thrown?caught!=RouteDecision.FAILURE:caught!=null||actual!=expected)throw new AssertionError("value "+phase+" "+incoming+" "+outgoing+" "+failure);
  if(!trace.equals(x.events.toString()))throw new AssertionError("trace "+x.events+" != "+trace);rows++;
 }System.out.println(rows+":nested-operand:shared-call:effects:exceptions");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			source, owner := fixture, "RouteDecision"
			if spelling == "renamed" {
				source = strings.NewReplacer("RouteDecision", "DispatchPolicy", "decide(", "apply(", "incoming", "firstCount", "outgoing", "secondCount", "advance(", "progress(").Replace(source)
				owner = "DispatchPolicy"
			}
			testAuthoredOwnerAcrossSourceModes(t, source, owner, owner+"Driver", "360:nested-operand:shared-call:effects:exceptions\n")
		})
	}
}

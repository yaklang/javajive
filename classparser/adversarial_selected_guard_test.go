package javaclassparser

import (
	"strings"
	"testing"
)

// A conditional boolean selector reconverges before a shared predicate. Both
// selected arms must keep that predicate and the final value-producing merge.
func TestAdversarialSelectedGuardKeepsSharedPredicate(t *testing.T) {
	const fixture = `
interface GatePredicate {boolean test(Object value);}
class TestGate implements GatePredicate {SelectedGuard owner;boolean matches;TestGate(boolean flag){matches=flag;}public boolean test(Object value){owner.events.append("P;");if(value!=owner.token)throw new AssertionError("argument");if(owner.phase==1)throw SelectedGuard.FAILURE;return matches;}}
class GateVerdict {final Object token;GateVerdict(Object value){token=value;}}
public class SelectedGuard {
 boolean left,right;Object token=new Object(),fallback;GatePredicate predicate;
 int phase;StringBuilder events=new StringBuilder();
 static final RuntimeException FAILURE=new IllegalArgumentException("identity");
 SelectedGuard(GatePredicate gate){predicate=gate;}
 Object argument(){events.append("A;");if(phase==2)throw FAILURE;return token;}
 Object select(boolean choose){return (choose?right:left)&&predicate.test(token)?new GateVerdict(argument()):fallback;}
}

class SelectedGuardDriver {public static void main(String[]args){int rows=0;
 for(boolean choose:new boolean[]{false,true})for(boolean left:new boolean[]{false,true})for(boolean right:new boolean[]{false,true})for(boolean matches:new boolean[]{false,true})for(int phase=0;phase<3;phase++)for(boolean nullable:new boolean[]{false,true}){
  TestGate gate=new TestGate(matches);SelectedGuard x=new SelectedGuard(gate);gate.owner=x;x.left=left;x.right=right;x.phase=phase;x.fallback=nullable?null:new Object();
  boolean selected=choose?right:left,accepted=selected&&matches;
  boolean throwing=selected&&phase==1||accepted&&phase==2;
  String expected=(selected?"P;":"")+(accepted&&phase!=1?"A;":"");
  Object result=null;Throwable caught=null;try{result=x.select(choose);}catch(Throwable error){caught=error;}
  if(throwing?caught!=SelectedGuard.FAILURE:caught!=null|| (accepted?!(result instanceof GateVerdict)||((GateVerdict)result).token!=x.token:result!=x.fallback))throw new AssertionError("value/throw choose="+choose+" left="+left+" right="+right+" matches="+matches+" phase="+phase);
  if(!expected.equals(x.events.toString()))throw new AssertionError("effects "+x.events+" != "+expected+" choose="+choose+" left="+left+" right="+right+" matches="+matches+" phase="+phase);
  rows++;
 }System.out.println(rows+":selection:shared-check:identity:effects");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			source, owner := fixture, "SelectedGuard"
			if spelling == "renamed" {
				source = strings.NewReplacer("SelectedGuard", "BranchPolicy", "select(", "resolve(", "left", "alpha", "right", "beta", "predicate", "filter").Replace(source)
				owner = "BranchPolicy"
			}
			driver := owner + "Driver"
			testAuthoredOwnerAcrossSourceModes(t, source, owner, driver, "96:selection:shared-check:identity:effects\n")
		})
	}
}

func TestAdversarialSharedDecisionKeepsBothValueExits(t *testing.T) {
	const fixture = `
public class DecisionPolicy {
 boolean a,b,c,d;int version,phase;Object accepted=new Object(),rejected=new Object();
 StringBuilder events=new StringBuilder();
 static final RuntimeException FAILURE=new IllegalStateException("identity");
 void prepare(){events.append("P;");if(phase==1)throw FAILURE;}
 int revision(){events.append("V;");if(phase==2)throw FAILURE;return version;}
 Object decide(){prepare();return (a||b)&&(revision()<2||(!c&&!d))?accepted:rejected;}
}
class DecisionPolicyDriver {public static void main(String[]args){int rows=0;
 for(int bits=0;bits<16;bits++)for(int version:new int[]{1,2,3})for(int phase=0;phase<3;phase++)for(boolean nullable:new boolean[]{false,true}){
  DecisionPolicy x=new DecisionPolicy();x.a=(bits&1)!=0;x.b=(bits&2)!=0;x.c=(bits&4)!=0;x.d=(bits&8)!=0;x.version=version;x.phase=phase;if(nullable)x.rejected=null;
  boolean reachesVersion=(bits&3)!=0,allow=reachesVersion&&(version<2||(bits&12)==0),throwsExpected=phase==1||phase==2&&reachesVersion;
  String expected="P;"+(phase!=1&&reachesVersion?"V;":"");Object actual=null;Throwable caught=null;try{actual=x.decide();}catch(Throwable error){caught=error;}
  if(throwsExpected?caught!=DecisionPolicy.FAILURE:caught!=null||actual!=(allow?x.accepted:x.rejected))throw new AssertionError("decision bits="+bits+" version="+version+" phase="+phase);
  if(!expected.equals(x.events.toString()))throw new AssertionError("effects "+x.events+" != "+expected);rows++;
 }System.out.println(rows+":routing:two-exits:identity:effects");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			source, owner := fixture, "DecisionPolicy"
			if spelling == "renamed" {
				source = strings.NewReplacer("DecisionPolicy", "AdmissionCheck", "decide(", "finish(", "revision(", "level(", "accepted", "firstResult", "rejected", "secondResult").Replace(source)
				owner = "AdmissionCheck"
			}
			testAuthoredOwnerAcrossSourceModes(t, source, owner, owner+"Driver", "288:routing:two-exits:identity:effects\n")
		})
	}
}

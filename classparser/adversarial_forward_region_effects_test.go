package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialForwardRegionPreservesEffectsAndFailures(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class RoutedWork {
 public static void step(StringBuilder trace,int event,int fail,RuntimeException marker){trace.append(event).append(';');if(event==fail)throw marker;}
 public static boolean test(StringBuilder trace,int event,int fail,RuntimeException marker,int x,int key){step(trace,event,fail,marker);return x==key;}
 public static int execute(int x,int depth,boolean gate,int fail,RuntimeException marker,StringBuilder trace){
  int total=2;step(trace,0,fail,marker);
  region:{if(gate){int code;
   if(test(trace,1,fail,marker,x,1))code=70;
   else if(test(trace,2,fail,marker,x,2))code=80;
   else if(test(trace,3,fail,marker,x,3))code=90;
   else {step(trace,4,fail,marker);break region;}
   for(int i=0;i<depth;i++){step(trace,5,fail,marker);total+=code*i;}
  }}
  step(trace,6,fail,marker);return total;
 }
}
class RoutedWorkDriver {
 public static void main(String[] args){int rows=0;
  for(int x=-2;x<=5;x++)for(int depth=0;depth<=4;depth++)for(boolean gate:new boolean[]{false,true})for(int fail=-1;fail<=7;fail++){
   java.util.ArrayList<Integer> events=new java.util.ArrayList<Integer>();events.add(0);
   boolean selected=gate&&x>=1&&x<=3;
   if(gate){int tests=selected?x:3;for(int j=1;j<=tests;j++)events.add(j);if(selected){for(int j=0;j<depth;j++)events.add(5);}else events.add(4);}
   events.add(6);StringBuilder expected=new StringBuilder();boolean failed=false;
   for(int e:events){expected.append(e).append(';');if(e==fail){failed=true;break;}}
   int value=2;if(selected)value+=(x*10+60)*depth*(depth-1)/2;
   RuntimeException marker=new RuntimeException("marker");StringBuilder actual=new StringBuilder();
   try{int result=RoutedWork.execute(x,depth,gate,fail,marker,actual);if(failed||result!=value)throw new AssertionError("outcome x="+x+" depth="+depth+" fail="+fail+" value="+result+" wanted="+value);}
   catch(RuntimeException caught){if(!failed||caught!=marker)throw new AssertionError("exception identity",caught);}
   if(!expected.toString().equals(actual.toString()))throw new AssertionError("trace actual="+actual+" expected="+expected);
   rows++;
  }System.out.println(rows+":forward-region:effect-order:exception-identity");
 }
}
`
	for _, name := range []string{"RoutedWork", "SelectiveCalculation"} {
		t.Run(name, func(t *testing.T) {
			text := strings.ReplaceAll(source, "RoutedWork", name)
			testAuthoredOwnerAcrossSourceModes(t, text, name, name+"Driver", "720:forward-region:effect-order:exception-identity\n")
		})
	}
}

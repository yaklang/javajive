package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialFloatingIntervalDecisionPreservesAllOutcomes(t *testing.T) {
	const fixture = `
public class IntervalChoice{
 static final RuntimeException FAILURE=new IllegalArgumentException("identity");StringBuilder trace=new StringBuilder();int fail,count;
 double tail(double x){trace.append("T;");if(++count==fail)throw FAILURE;return x*x+3*x;}
 double wave(double x){trace.append("W;");if(++count==fail)throw FAILURE;return x*x*x-2*x;}
 double compute(double a,double b){if(a>b)return -compute(b,a);return a < -2?(b < 0?tail(-b)-tail(-a):wave(b)-wave(a)):(b > 2&&a > 0?tail(a)-tail(b):wave(b)-wave(a));}
}
class IntervalDriver{static double tail(double x){return x*x+3*x;}static double wave(double x){return x*x*x-2*x;}
 public static void main(String[]args){int rows=0;double[] samples={Double.NaN,Double.NEGATIVE_INFINITY,-3,-2,Math.nextUp(-2.0),-1,-0.0,0.0,1,2,Math.nextUp(2.0),3,Double.POSITIVE_INFINITY};
 for(double a:samples)for(double b:samples)for(int fail=0;fail<3;fail++){boolean swap=a>b;double lo=swap?b:a,hi=swap?a:b;boolean negative=lo < -2&&hi < 0,positive=lo >= -2&&hi > 2&&lo > 0;double result;if(negative)result=tail(-hi)-tail(-lo);else if(positive)result=tail(lo)-tail(hi);else result=wave(hi)-wave(lo);if(swap)result=-result;
 IntervalChoice target=new IntervalChoice();target.fail=fail;double actual=0;Throwable error=null;try{actual=target.compute(a,b);}catch(Throwable ex){error=ex;}
 if(fail==0?error!=null||Double.doubleToLongBits(actual)!=Double.doubleToLongBits(result):error!=IntervalChoice.FAILURE)throw new AssertionError("value "+a+","+b+" fail="+fail,error);
 String mark=negative||positive?"T;":"W;";String expected=fail==1?mark:mark+mark;if(!expected.equals(target.trace.toString()))throw new AssertionError("trace "+a+","+b+" "+target.trace+" != "+expected);rows++;
 }System.out.println(rows+":interval:floating:effects:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "IntervalChoice"
			if spelling == "renamed" {
				text = strings.NewReplacer("IntervalChoice", "SegmentRoute", "compute(", "evaluate(", "tail(", "outer(", "wave(", "central(").Replace(text)
				owner = "SegmentRoute"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "IntervalDriver", "507:interval:floating:effects:failure\n")
		})
	}
}

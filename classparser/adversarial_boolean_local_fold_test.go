package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialBooleanLocalFoldPreservesConsumerType(t *testing.T) {
	const fixture = `
public class BooleanFold {
 StringBuilder trace=new StringBuilder();
 double observe(double x){trace.append(Double.doubleToRawLongBits(x)).append(';');return x;}
 int direction(boolean forward,double x,double end){boolean reached=forward?observe(x)>=end:observe(x)<=end;if(reached)return 3;return 7;}
 int outside(double x,double low,double mid,double high){boolean bad=(x<mid&&mid-low>0.95*(high-low))||(x>mid&&high-mid>0.95*(high-low))||x==mid;if(!bad)return 11;return 13;}
}
class BooleanFoldDriver {public static void main(String[] args){int rows=0;double[] inputs={Double.NEGATIVE_INFINITY,-17,-1,-0.0,0.0,Double.MIN_VALUE,1,17,Double.POSITIVE_INFINITY,Double.NaN};
 for(boolean forward:new boolean[]{false,true})for(double x:inputs)for(double end:inputs){BooleanFold owner=new BooleanFold();boolean relation=forward?Double.compare(x,end)>=0:Double.compare(x,end)<=0;
 if(x==0.0&&end==0.0)relation=true;if(Double.isNaN(x)||Double.isNaN(end))relation=false;
 int got=owner.direction(forward,x,end);String expected=Double.doubleToRawLongBits(x)+";";if(got!=(relation?3:7)||!expected.equals(owner.trace.toString()))throw new AssertionError("direction "+forward+" x="+x+" end="+end+" got="+got+" trace="+owner.trace);rows++;}
 for(double x:inputs)for(double low:new double[]{-20,0,1})for(double mid:new double[]{-1,0,1})for(double high:new double[]{0,1,20}){boolean bad=false;if(x<mid)bad=mid-low>0.95*(high-low);else if(x>mid)bad=high-mid>0.95*(high-low);else if(x==mid)bad=true;int got=new BooleanFold().outside(x,low,mid,high);if(got!=(bad?13:11))throw new AssertionError("outside "+x+" "+low+" "+mid+" "+high+" got="+got);rows++;}
 System.out.println(rows+":boolean-fold:selected-effect:NaN:signed-zero:infinity");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "BooleanFold"
			if spelling == "renamed" {
				text = strings.NewReplacer("BooleanFold {", "TruthBoundary {", "new BooleanFold()", "new TruthBoundary()", "BooleanFold owner", "TruthBoundary owner", "direction(", "route(", "outside(", "beyond(").Replace(text)
				owner = "TruthBoundary"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "BooleanFoldDriver", "470:boolean-fold:selected-effect:NaN:signed-zero:infinity\n")
		})
	}
}

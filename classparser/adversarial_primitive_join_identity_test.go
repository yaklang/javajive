package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialPrimitiveJoinRetainsEveryDefinition(t *testing.T) {
	const fixture = `
public class WordJoin {
 int[] digits; int sign; StringBuilder trace=new StringBuilder();
 int word(int x){trace.append(x).append(';');return x;}
 int choose(int mode,int x){boolean increment=false;switch(mode){
 case 0:increment=false;break;
 case 1:increment=word(x)!=0;break;
 case 2:increment=word(x)>=17;break;
 case 3:increment=word(x)>17;break;
 case 4:increment=word(x)>17||(word(x)==17&&(digits[0]&1)==1);break;
 case 5:increment=word(x)>17||(word(x)==17&&(digits[0]&1)==0);break;
 case 6:increment=sign==1&&word(x)!=0;break;
 default:increment=sign==-1&&word(x)!=0;break;}
 return increment?29:-31;}
 int carry(int base,int[] a,int[] b){int carry=1;for(int i=0;i<a.length;i++){int total=word(base-1-b[i]+a[i]+carry);carry=total/base;digits[i]=total-carry*base;}return carry;}
}

class WordDriver {
 public static void main(String[] args){int rows=0;
 for(int mode=-1;mode<9;mode++)for(int x:new int[]{-3,0,1,16,17,18,33})for(int bit=0;bit<2;bit++)for(int sign:new int[]{-1,0,1}){
 WordJoin obj=new WordJoin();obj.sign=sign;obj.digits=new int[]{bit};boolean expect;String trace="";
 if(mode==0){expect=false;}else if(mode>=1&&mode<=3){expect=mode==1?x!=0:mode==2?x>=17:x>17;trace=x+";";}
 else if(mode==4||mode==5){expect=x>17||(x==17&&bit==(mode==4?1:0));trace=x+";"+(x>17?"":x+";");}
 else {expect=sign==(mode==6?1:-1)&&x!=0;trace=sign==(mode==6?1:-1)?x+";":"";}
 int got=obj.choose(mode,x);if(got!=(expect?29:-31)||!trace.equals(obj.trace.toString()))throw new AssertionError("choice="+mode+" x="+x+" bit="+bit+" sign="+sign+" got="+got+" trace="+obj.trace+" expected="+trace);rows++;
 }
 for(int base:new int[]{2,3,10,97})for(int n=0;n<8;n++)for(int seed=0;seed<9;seed++){
 int[] a=new int[n],b=new int[n],expected=new int[n];int model=1;StringBuilder trace=new StringBuilder();
 for(int i=0;i<n;i++){a[i]=(seed*7+i*3)%base;b[i]=(seed*5+i*11)%base;int total=base-1-b[i]+a[i]+model;trace.append(total).append(';');model=total/base;expected[i]=total%base;}
 WordJoin obj=new WordJoin();obj.digits=new int[n];int got=obj.carry(base,a,b);if(got!=model||!java.util.Arrays.equals(expected,obj.digits)||!trace.toString().equals(obj.trace.toString()))throw new AssertionError("carry base="+base+" n="+n+" seed="+seed+" got="+got+" expected="+model+" digits="+java.util.Arrays.toString(obj.digits));rows++;
 }System.out.println(rows+":primitive:joined-definitions:trace:empty-loop:carry");}
}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "WordJoin"
			if spelling == "renamed" {
				text = strings.NewReplacer("WordJoin", "PhiRoute", "choose(", "select(", "carry(", "propagate(", "word(", "observe(", "digits", "limbs").Replace(text)
				owner = "PhiRoute"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "WordDriver", "708:primitive:joined-definitions:trace:empty-loop:carry\n")
		})
	}
}

func TestAdversarialArithmeticLoopCarryKeepsNumericDefinition(t *testing.T) {
	const fixture = `
public class CarryLoop {
 int[] digits;
 int apply(int base,int factor,int[] a,int[] b){
 int carry=0;
 for(int i=0;i<digits.length;i++){int limb=i<b.length?b[i]:0;int total=limb*factor+carry;carry=total/base;digits[i]=total-carry*base;}
 carry=1;
 for(int i=0;i<digits.length;i++){int total=base-1-digits[i]+a[i]+carry;carry=total/base;digits[i]=total-carry*base;}
 return carry==0?-31:29;
 }
}
class CarryDriver {
 public static void main(String[] args){int rows=0;
 for(int base:new int[]{2,3,10,97})for(int n=0;n<8;n++)for(int seed=0;seed<9;seed++)for(int factor:new int[]{0,1,2,5}){
 int[] a=new int[n],b=new int[n==0?0:n-1],expected=new int[n];int model=0;
 for(int i=0;i<n;i++){a[i]=(seed*7+i*3)%base;if(i<b.length)b[i]=(seed*5+i*11)%base;int total=(i<b.length?b[i]:0)*factor+model;model=total/base;expected[i]=total%base;}
 model=1;for(int i=0;i<n;i++){int total=base-1-expected[i]+a[i]+model;model=total/base;expected[i]=total%base;}
 CarryLoop target=new CarryLoop();target.digits=new int[n];int actual=target.apply(base,factor,a,b);
 if(actual!=(model==0?-31:29)||!java.util.Arrays.equals(expected,target.digits))throw new AssertionError("base="+base+" n="+n+" seed="+seed+" factor="+factor+" actual="+actual+" digits="+java.util.Arrays.toString(target.digits)+" expected="+java.util.Arrays.toString(expected));rows++;
 }System.out.println(rows+":arithmetic:carry:reused-slot:zero-trip");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "CarryLoop"
			if spelling == "renamed" {
				text = strings.NewReplacer("CarryLoop", "DigitRecurrence", "apply(", "transform(", "digits", "limbs").Replace(text)
				owner = "DigitRecurrence"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "CarryDriver", "1152:arithmetic:carry:reused-slot:zero-trip\n")
		})
	}
}

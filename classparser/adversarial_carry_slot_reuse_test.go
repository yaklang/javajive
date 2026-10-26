package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialCarryWebSurvivesLaterIndexIncrement(t *testing.T) {
	const source = `
import java.math.BigInteger;
public class CarryReuse {
 int[] digits;
 int apply(int base,int factor,int[] a,int[] b){
 int carry=0;
 for(int i=0;i<digits.length;i++){int limb=i<b.length?b[i]:0;int total=limb*factor+carry;carry=total/base;digits[i]=total-carry*base;}
 carry=1;
 for(int i=0;i<digits.length;i++){int total=base-1-digits[i]+a[i]+carry;carry=total/base;digits[i]=total-carry*base;}
 int result=carry==0?-31:29;for(carry=0;carry<digits.length;carry++)result=31*result+digits[carry];return result;
 }
}
class CarryReuseDriver{public static void main(String[]args){int rows=0;
 for(int base:new int[]{2,3,10,97})for(int n=0;n<8;n++)for(int seed=0;seed<9;seed++)for(int factor:new int[]{0,1,2,5}){
 int[] a=new int[n],b=new int[n==0?0:n-1];BigInteger radix=BigInteger.valueOf(base),av=BigInteger.ZERO,bv=BigInteger.ZERO,power=BigInteger.ONE;
 for(int i=0;i<n;i++){a[i]=(seed*7+i*3)%base;if(i<b.length)b[i]=(seed*5+i*11)%base;av=av.add(power.multiply(BigInteger.valueOf(a[i])));if(i<b.length)bv=bv.add(power.multiply(BigInteger.valueOf(b[i])));power=power.multiply(radix);}
 BigInteger full=power.add(av).subtract(bv.multiply(BigInteger.valueOf(factor)).mod(power));BigInteger[] split=full.divideAndRemainder(power);int model=split[0].signum()==0?-31:29;BigInteger rest=split[1];int[] expected=new int[n];for(int i=0;i<n;i++){BigInteger[] qr=rest.divideAndRemainder(radix);expected[i]=qr[1].intValue();rest=qr[0];model=31*model+expected[i];}
 CarryReuse target=new CarryReuse();target.digits=new int[n];int actual=target.apply(base,factor,a,b);if(actual!=model||!java.util.Arrays.equals(expected,target.digits))throw new AssertionError("base="+base+" n="+n+" seed="+seed+" factor="+factor+" actual="+actual+" expected="+model+" digits="+java.util.Arrays.toString(target.digits));rows++;
 }System.out.println(rows+":carry:disjoint-increment:BigInteger:zero-trip");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := source, "CarryReuse"
			if spelling == "renamed" {
				text = strings.NewReplacer("CarryReuse {", "RadixTransform {", "CarryReuse target", "RadixTransform target", "new CarryReuse()", "new RadixTransform()", "apply(", "convert(", "digits", "limbs").Replace(text)
				owner = "RadixTransform"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "CarryReuseDriver", "1152:carry:disjoint-increment:BigInteger:zero-trip\n")
		})
	}
}

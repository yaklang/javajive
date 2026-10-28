package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAbruptGuardCannotOwnFollowingLambdaDecision(t *testing.T) {
	const fixture = `
import java.util.function.IntPredicate;
public class AssertionCapture {
 static StringBuilder trace=new StringBuilder();
 static IntPredicate choose(int size,MaskTrace left,MaskTrace right,MaskTrace mask){
 GUARD
 return left==null||left==right ? index->!mask.get(index) : index->left.get(index)&&!mask.get(index);
 }
}
class MaskTrace {final int bits;final String name;MaskTrace(int b,String n){bits=b;name=n;}
 int length(){AssertionCapture.trace.append(name).append("L;");return 4;}
 boolean get(int index){AssertionCapture.trace.append(name).append(index).append(';');return (bits&(1<<index))!=0;}}
class CaptureDriver {public static void main(String[] args){
 boolean enabled=ENABLED,explicit=EXPLICIT;
 ClassLoader.getSystemClassLoader().setClassAssertionStatus("AssertionCapture",enabled);
 int rows=0;
 for(int branch=0;branch<3;branch++)for(int size:new int[]{3,4})for(int leftBits:new int[]{0,5,10,15})for(int maskBits=0;maskBits<16;maskBits++)for(int index=0;index<4;index++){
 MaskTrace left=branch==0?null:new MaskTrace(leftBits,"a"),right=branch==1?left:new MaskTrace(0,"b"),mask=new MaskTrace(maskBits,"m");
 AssertionCapture.trace.setLength(0);Throwable failure=null;boolean actual=false;
 try{actual=AssertionCapture.choose(size,left,right,mask).test(index);}catch(Throwable error){failure=error;}
 boolean rejected=(enabled||explicit)&&left!=null&&size!=4;
 boolean expected=(branch!=2||(leftBits&(1<<index))!=0)&&(maskBits&(1<<index))==0;
 String trace=(enabled||explicit)&&left!=null?"aL;":"";
 if(!rejected){if(branch==2)trace+="a"+index+";";if(branch!=2||(leftBits&(1<<index))!=0)trace+="m"+index+";";}
 if(rejected?(explicit?!(failure instanceof IllegalArgumentException):!(failure instanceof AssertionError)):failure!=null||actual!=expected)throw new AssertionError("branch="+branch+" size="+size+" left="+leftBits+" mask="+maskBits+" index="+index+" actual="+actual+" expected="+expected,failure);
 if(!trace.equals(AssertionCapture.trace.toString()))throw new AssertionError("trace="+AssertionCapture.trace+" expected="+trace);rows++;
 }System.out.println(rows+":guard:lambda:selection:trace:failure");}}
`
	for _, guard := range []string{"disabled", "enabled", "explicit"} {
		t.Run(guard, func(t *testing.T) {
			for _, spelling := range []string{"original", "renamed"} {
				t.Run(spelling, func(t *testing.T) {
					statement, enabled, explicit := "assert left==null||left.length()==size;", "false", "false"
					if guard == "enabled" {
						enabled = "true"
					}
					if guard == "explicit" {
						statement, explicit = "if(left!=null&&left.length()!=size)throw new IllegalArgumentException();", "true"
					}
					text := strings.NewReplacer("GUARD", statement, "ENABLED", enabled, "EXPLICIT", explicit).Replace(fixture)
					owner := "AssertionCapture"
					if spelling == "renamed" {
						text = strings.NewReplacer("AssertionCapture", "GuardedPredicate", "choose(", "select(", "MaskTrace", "BitSource").Replace(text)
						owner = "GuardedPredicate"
					}
					testAuthoredOwnerAcrossSourceModes(t, text, owner, "CaptureDriver", "1536:guard:lambda:selection:trace:failure\n")
				})
			}
		})
	}
}

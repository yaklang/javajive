package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const constructorConstantOperandFixture = `
class LiteralOperandBase {
 final int number;final long wide;final double floating;final Object absent;final String text;
 LiteralOperandBase(int n,long w,double d,Object o,String s){number=n;wide=w;floating=d;absent=o;text=s;}
 Object capture(){return null;}Object owner(){return null;}
}
class LiteralOperandObservedBase {final Object seen;LiteralOperandObservedBase(int n){seen=capture();}Object capture(){return null;}}
class LiteralOperandOwner {
 class Observed extends LiteralOperandObservedBase {Observed(){super(7);}Object capture(){return LiteralOperandOwner.this;}}
 class First extends LiteralOperandBase {final Object token;First(Object token,int n){super(n,0x123456789abcdefL,-0.0,null,"literal");this.token=token;}Object capture(){return token;}Object owner(){return LiteralOperandOwner.this;}}
 LiteralOperandBase make(Object token,int n){return new First(token,n);}
 class Second extends LiteralOperandBase {final Object token;Second(Object token){super(-32769,1L,1.25,null,"second");this.token=token;}Object capture(){return token;}Object owner(){return LiteralOperandOwner.this;}}
 LiteralOperandBase negative(Object token){return new Second(token);}
}
class LiteralOperandOracle {static void run(){LiteralOperandOwner owner=new LiteralOperandOwner();Object token=new Object();if(owner.new Observed().seen!=owner)throw new AssertionError("original early capture observation");
 for(Object value:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
  LiteralOperandBase a=owner.make(value,n),b=owner.negative(value);
  if(a.number!=n||a.wide!=0x123456789abcdefL||Double.doubleToRawLongBits(a.floating)!=Long.MIN_VALUE||a.absent!=null||!a.text.equals("literal")||a.capture()!=value||a.owner()!=owner)throw new AssertionError("literal/parameter/identity");
  if(b.number!=-32769||b.wide!=1L||b.floating!=1.25||b.absent!=null||!b.text.equals("second")||b.capture()!=value||b.owner()!=owner)throw new AssertionError("constant widths/identity");
  System.out.println(n+":"+a.wide+":"+Double.doubleToRawLongBits(a.floating)+":"+(value==token));
 }
}}
public class LiteralOperandDriver {public static void main(String[]args){LiteralOperandOracle.run();}}
`

func TestAdversarialConstructorCaptureWithConstantDelegationOperands(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "LiteralOperandDriver", constructorConstantOperandFixture, nil, []string{"LiteralOperandOwner", "LiteralOperandOwner$First", "LiteralOperandOwner$Second"}, true, Precision, Compatibility, "legacy")
}

func TestConstructorConstantOperandDoesNotBypassReceiverObservation(t *testing.T) {
	javac, java := t04Tools(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "LiteralOperandDriver.java")
	if err := os.WriteFile(path, []byte(constructorConstantOperandFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, path).CombinedOutput(); err != nil {
		t.Fatalf("original: %v\n%s", err, out)
	}
	_ = t04RunJava(t, java, dir, "LiteralOperandDriver")
	resolve := func(name string) ([]byte, bool) {
		raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
		return raw, err == nil
	}
	raw, _ := resolve("LiteralOperandOwner$Observed")
	r, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: resolve})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Source, DecompileStubMarker) {
		t.Fatal("constant operand bypassed observing parent proof", r.Source)
	}
}

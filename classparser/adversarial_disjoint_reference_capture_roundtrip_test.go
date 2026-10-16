package javaclassparser

import (
	"strings"
	"testing"
)

const disjointReferenceCaptureFixture = `
class LifetimeCell {final Object token;LifetimeCell(Object x){token=x;}}
class LifetimeEffects {static int mode;static String trace;static LifetimeBase published;static final RuntimeException failure=new RuntimeException("identity");static LifetimeCell choose(Object x){trace+="E";if(mode==1)throw failure;return new LifetimeCell(x);}static void observe(int pad,LifetimeCell x){if(pad!=37||x==null)throw new AssertionError("prior scope");trace+="S";}}
abstract class LifetimeBase {final LifetimeCell early;LifetimeBase(){LifetimeEffects.trace+="P";LifetimeEffects.published=this;early=read();if(LifetimeEffects.mode==2)throw LifetimeEffects.failure;}abstract LifetimeCell read();}
class LifetimeOwner {LifetimeBase[] make(Object[] inputs,LifetimeBase[] result,boolean prior){if(prior){int padding=37;LifetimeCell earlier=new LifetimeCell(this);LifetimeEffects.observe(padding,earlier);}for(int i=0;i<inputs.length;i++){final LifetimeCell selected=LifetimeEffects.choose(inputs[i]);result[i]=new LifetimeBase(){LifetimeCell read(){return selected;}};}return result;}}
class LifetimeDriver {public static void main(String[]args){int rows=0;Object identity=new Object();for(Object[] input:new Object[][]{{},{null},{identity},{null,identity,new Object(),identity,null}})for(int mode=0;mode<3;mode++)for(boolean prior:new boolean[]{false,true}){LifetimeEffects.mode=mode;LifetimeEffects.trace="";LifetimeEffects.published=null;LifetimeBase[] output=new LifetimeBase[input.length];Throwable failure=null;try{new LifetimeOwner().make(input,output,prior);}catch(Throwable e){failure=e;}boolean failed=mode!=0&&input.length>0;if(failed?failure!=LifetimeEffects.failure:failure!=null)throw new AssertionError("failure identity");String expected=prior?"S":"";if(failed){expected+=mode==1?"E":"EP";if(mode==2&&LifetimeEffects.published.read().token!=input[0])throw new AssertionError("pre-super capture identity");}else for(int i=0;i<input.length;i++){expected+="EP";if(output[i].read()!=output[i].early||output[i].read().token!=input[i])throw new AssertionError("fresh iteration capture");for(int j=0;j<i;j++)if(output[i].read()==output[j].read())throw new AssertionError("different allocation lifetime");}if(!expected.equals(LifetimeEffects.trace))throw new AssertionError("effects order");rows++;}System.out.println(rows+":same-type:disjoint-slot:fresh-capture:early:failure");}}`

// An earlier block and a later loop reuse one physical JVM reference slot.
// Equal declared types do not make those reaching-definition webs one local.
// The unchanged binary driver checks every retained closure, superclass-time
// observation, allocation identity, producer failure and constructor failure.
func TestAdversarialDisjointReferenceCaptureLifetimes(t *testing.T) {
	for _, shape := range []string{"method", "constructor", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture := disjointReferenceCaptureFixture
			owner, driver := "LifetimeOwner", "LifetimeDriver"
			if shape == "constructor" {
				fixture = strings.Replace(fixture, "LifetimeBase[] make(Object[] inputs,LifetimeBase[] result,boolean prior)", "LifetimeOwner(Object[] inputs,LifetimeBase[] result,boolean prior)", 1)
				fixture = strings.Replace(fixture, "return result;}}", "}}", 1)
				fixture = strings.Replace(fixture, "new LifetimeOwner().make(input,output,prior)", "new LifetimeOwner(input,output,prior)", 1)
			}
			if shape == "renamed" {
				fixture = strings.ReplaceAll(fixture, "Lifetime", "FreshBinding")
				fixture = strings.ReplaceAll(fixture, "selected", "capturedWord")
				owner, driver = "FreshBindingOwner", "FreshBindingDriver"
			}
			testNativeIndependentFamilyFixture(t, fixture, []string{owner}, driver, "24:same-type:disjoint-slot:fresh-capture:early:failure\n", nativeLexicalExactSignatures)
		})
	}
}

package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// All assertions live in unchanged original Check classfiles. This bounded
// family covers distinct 3/4-way definition webs, null and cached exits, both
// interface declaration orders, integer boundaries and exact effect order.
func TestAdversarialReferenceDefinitionFamilyBankIndependentOriginalOracle(t *testing.T) {
	var source, driver strings.Builder
	owners := []string{}
	source.WriteString(`interface DefinitionFirst{int value();}interface DefinitionSecond{int value();}class DefinitionEffects{static String trace="";static boolean choose(int mode,int arm){trace+="C"+arm;return mode==arm;}}`)
	driver.WriteString(`class DefinitionDriver{public static void main(String[]args){int rows=0;`)
	for i := 0; i < 100; i++ {
		first, second := "DefinitionFirst", "DefinitionSecond"
		if i%2 == 1 {
			first, second = second, first
		}
		for arm := 0; arm < 4; arm++ {
			parents := "DefinitionSecond"
			if arm < 2 {
				parents = first + "," + second
			}
			fmt.Fprintf(&source, `class DefinitionValue%d_%d implements %s{public int value(){return %d;}}`, i, arm, parents, i*101+arm)
		}
		owner := fmt.Sprintf("DefinitionOwner%d", i)
		owners = append(owners, owner)
		fmt.Fprintf(&source, `class %s{DefinitionSecond cached;DefinitionSecond resolve(int mode){if(cached!=null)return cached;DefinitionSecond value;if(DefinitionEffects.choose(mode,0)){value=new DefinitionValue%d_0();DefinitionEffects.trace+="A0";}else if(DefinitionEffects.choose(mode,1)){value=new DefinitionValue%d_1();DefinitionEffects.trace+="A1";}`, owner, i, i)
		arms := 3
		if i%2 == 1 {
			arms = 4
			fmt.Fprintf(&source, `else if(DefinitionEffects.choose(mode,2)){value=new DefinitionValue%d_2();DefinitionEffects.trace+="A2";}`, i)
		}
		fmt.Fprintf(&source, `else{value=new DefinitionValue%d_%d();DefinitionEffects.trace+="A%d";}cached=value;return value;}}`, i, arms-1, arms-1)
		fmt.Fprintf(&source, `class DefinitionCheck%d{static int verify(){int rows=0;for(int mode:new int[]{0,1,2,3,-1,-2,Integer.MIN_VALUE,Integer.MAX_VALUE,101,127}){DefinitionOwner%d owner=new DefinitionOwner%d();DefinitionEffects.trace="";DefinitionSecond a=owner.resolve(mode);String firstTrace=DefinitionEffects.trace;DefinitionSecond b=owner.resolve(mode==0?1:0);int arm=mode==0?0:mode==1?1:`, i, i, i)
		if arms == 4 {
			source.WriteString(`mode==2?2:3;`)
		} else {
			source.WriteString(`2;`)
		}
		source.WriteString(`String expected=arm==0?"C0A0":arm==1?"C0C1A1":`)
		if arms == 4 {
			source.WriteString(`arm==2?"C0C1C2A2":"C0C1C2A3";`)
		} else {
			source.WriteString(`"C0C1A2";`)
		}
		fmt.Fprintf(&source, `if(a==null||a!=b||a!=owner.cached||a.value()!=%d+arm||!expected.equals(firstTrace)||!firstTrace.equals(DefinitionEffects.trace))throw new AssertionError("original definition/consumer/identity/order:"+%d+":"+mode);rows++;}return rows;}}`, i*101, i)
		fmt.Fprintf(&driver, `rows+=DefinitionCheck%d.verify();`, i)
	}
	driver.WriteString(`if(rows!=1000)throw new AssertionError("row conservation");System.out.println("1000:distinct-definitions:3/4-way:boundaries:identity:order");}}`)
	source.WriteString(driver.String())
	testNativeIndependentFamilyFixture(t, source.String(), owners, "DefinitionDriver", "1000:distinct-definitions:3/4-way:boundaries:identity:order\n", nativeLexicalExactSignatures)
}

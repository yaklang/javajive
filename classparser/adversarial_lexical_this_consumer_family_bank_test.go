package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// 100 structurally distinct nonstatic families: five inheritance depths, five
// local-copy depths, two descriptor orders and two original erasure bounds.
// The original Check classfiles alone assert values, identities and effects.
func TestAdversarialLexicalThisConsumerFamilyBankIndependentOracle(t *testing.T) {
	var source, driver strings.Builder
	owners := []string{}
	source.WriteString(`class LexicalBankWord<T>{final T value;LexicalBankWord(T value){this.value=value;}T key(){return value;}}class LexicalBankEffects{static String trace="";}`)
	driver.WriteString(`class LexicalBankDriver{public static void main(String[]args){int rows=0;`)
	for i := 0; i < 100; i++ {
		depth := i%5 + 1
		copies := i/5%5 + 1
		reverse := i/25%2 == 1
		bounded := i/50 == 1
		formal := []string{"K", "S", "U"}[i%3]
		owner := fmt.Sprintf("LexicalBankOwner%d", i)
		owners = append(owners, owner)
		bound := ""
		if bounded {
			bound = " extends Number"
		}
		params := formal + " value,boolean last"
		args := "(" + formal + ")key,last"
		decoy := "Object value,Boolean wrong"
		if reverse {
			params = "boolean last," + formal + " value"
			args = "last,(" + formal + ")key"
			decoy = "Boolean wrong,Object value"
		}
		fmt.Fprintf(&source, `class %s<%s%s>{final LexicalBankWord<%s> item;%s(%s value){item=new LexicalBankWord<%s>(value);}abstract class Base{boolean accept(%s){LexicalBankEffects.trace+=last?"L":"F";return value==item.value;}boolean accept(%s){throw new AssertionError("wrong selected overload");}}`, owner, formal, bound, formal, owner, formal, formal, params, decoy)
		parent := "Base"
		for n := 1; n < depth; n++ {
			layer := fmt.Sprintf("Layer%d", n)
			fmt.Fprintf(&source, `abstract class %s extends %s{}`, layer, parent)
			parent = layer
		}
		fmt.Fprintf(&source, `class View extends %s{%s select(boolean present,boolean last){LexicalBankWord raw=item;Object copy0=present?raw.key():null;`, parent, formal)
		for c := 1; c < copies; c++ {
			fmt.Fprintf(&source, `Object copy%d=copy%d;`, c, c-1)
		}
		fmt.Fprintf(&source, `Object key=copy%d;if(key!=null&&accept(%s))return (%s)key;throw new java.util.NoSuchElementException();}}}`, copies-1, args, formal)
		actual := "Object"
		token := "new Object()"
		if bounded {
			actual = "Number"
			token = "Integer.valueOf(scenario%3==0?Integer.MIN_VALUE:scenario%3==1?Integer.MAX_VALUE:0)"
		}
		fmt.Fprintf(&source, `class LexicalBankCheck%d{static int verify(){int rows=0;for(int scenario=0;scenario<10;scenario++){%s token=%s;%s<%s> owner=new %s<%s>(token);%s<%s>.View view=owner.new View();boolean present=scenario%%3!=0;boolean last=scenario%%2==0;LexicalBankEffects.trace="";try{Object got=view.select(present,last);if(!present||got!=token||!LexicalBankEffects.trace.equals(last?"L":"F"))throw new AssertionError("original lexical result/identity/overload:%d:"+scenario);}catch(java.util.NoSuchElementException e){if(present||!LexicalBankEffects.trace.equals(""))throw new AssertionError("original lexical failure/order:%d:"+scenario);}rows++;}return rows;}}`, i, actual, token, owner, actual, owner, actual, owner, actual, i, i)
		fmt.Fprintf(&driver, `rows+=LexicalBankCheck%d.verify();`, i)
	}
	driver.WriteString(`if(rows!=1000)throw new AssertionError("row conservation");System.out.println("1000:lexical-this:owners:inheritance:bounds:overloads:identity");}}`)
	source.WriteString(driver.String())
	testNativeIndependentFamilyFixture(t, source.String(), owners, "LexicalBankDriver", "1000:lexical-this:owners:inheritance:bounds:overloads:identity\n", nativeLexicalExactSignatures)
}

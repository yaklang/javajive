package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Only callers are rebuilt. The 1000 assertions and all producer methods stay
// in unchanged original classfiles; staticness/arity/scope/copy depth vary.
func TestAdversarialGenericSupplierWitnessFamilyBankIndependentOracle(t *testing.T) {
	var source, driver strings.Builder
	owners := []string{}
	source.WriteString(`class WitnessBankBox<T>{final T value;WitnessBankBox(T value){this.value=value;}}class WitnessBankEffects{static String trace="";static void copy(Object a,Object b){trace+="C";if(a!=b)throw new AssertionError("copy identity");}}`)
	driver.WriteString(`class WitnessBankDriver{public static void main(String[]args){int rows=0;`)
	for i := 0; i < 100; i++ {
		copies := i%5 + 1
		arity := i / 5 % 5
		static := i/25%2 == 1
		classScope := i/50 == 1
		reversed := i%2 == 1
		formal := []string{"T", "S", "U"}[i%3]
		maker := fmt.Sprintf("WitnessBankMaker%d", i)
		owner := fmt.Sprintf("WitnessBankOwner%d", i)
		owners = append(owners, owner)
		flag := ""
		if static {
			flag = "static "
		}
		params := "WitnessBankBox<V> box,V present"
		args := "box,(" + formal + ")present"
		if reversed {
			params = "V present,WitnessBankBox<V> box"
			args = "(" + formal + ")present,box"
		}
		for a := 0; a < arity; a++ {
			params += fmt.Sprintf(",int d%d", a)
			args += fmt.Sprintf(",%d", i+a)
		}
		fmt.Fprintf(&source, `class %s{%s<V>java.util.Optional<V> pick(%s){WitnessBankEffects.trace+="P";`, maker, flag, params)
		for a := 0; a < arity; a++ {
			fmt.Fprintf(&source, `if(d%d!=%d)throw new AssertionError("word order");`, a, i+a)
		}
		fmt.Fprintf(&source, `return java.util.Optional.ofNullable(present);}%s<V>V build(WitnessBankBox<V> box){WitnessBankEffects.trace+="B";return box.value;}}`, flag)
		header := owner
		method := "static <" + formal + ">"
		if classScope {
			header += "<" + formal + ">"
			method = ""
		}
		fmt.Fprintf(&source, `class %s{%s %s make(%s maker,WitnessBankBox<%s> box,Object present,Object source){%s result;if(source!=null){`, header, method, formal, maker, formal, formal)
		previous := "box"
		for c := 0; c < copies; c++ {
			capture := fmt.Sprintf("capture%d", c)
			fmt.Fprintf(&source, `WitnessBankBox<%s> %s=%s;`, formal, capture, previous)
			previous = capture
		}
		fmt.Fprintf(&source, `%s producer=maker;Object copied=source;result=maker.pick(%s).orElseGet(()->{%s value=producer.build(%s);WitnessBankEffects.copy(copied,value);return value;});}else{result=maker.build(box);}return result;}}`, maker, args, formal, previous)
		call := owner + ".make(maker,box,present,copy)"
		if classScope {
			call = "new " + owner + "<Object>().make(maker,box,present,copy)"
		}
		fmt.Fprintf(&source, `class WitnessBankCheck%d{static int verify(){int rows=0;for(int scenario=0;scenario<10;scenario++){Object token=new Object();Object other=new Object();WitnessBankBox<Object> box=new WitnessBankBox<Object>(token);%s maker=new %s();Object present=scenario%%3==0?null:scenario%%3==1?token:other;Object copy=scenario%%2==0?null:token;WitnessBankEffects.trace="";Object got=%s;Object expected=copy!=null&&present!=null?present:token;String trace=copy==null?"B":present==null?"PBC":"P";if(got!=expected||!trace.equals(WitnessBankEffects.trace))throw new AssertionError("original supplier identity/laziness/order:%d:"+scenario);rows++;}return rows;}}`, i, maker, maker, call, i)
		fmt.Fprintf(&driver, `rows+=WitnessBankCheck%d.verify();`, i)
	}
	driver.WriteString(`if(rows!=1000)throw new AssertionError("row conservation");System.out.println("1000:generic-witness:erased-word:identity:laziness:order");}}`)
	source.WriteString(driver.String())
	testNativeIndependentFamilyFixture(t, source.String(), owners, "WitnessBankDriver", "1000:generic-witness:erased-word:identity:laziness:order\n", nativeLexicalExactSignatures)
}

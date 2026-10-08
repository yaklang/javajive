package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// 100 source structures: ten receiver-chain lengths crossed with ten distinct
// conditional trees. Original javac and the unchanged original caller check
// 1,200 branch/payload combinations per debug/policy variant. Factory and step
// counts expose eager evaluation; polluted raw payloads expose new/lost checks.
func TestAdversarialConditionalGenericAssignmentHundredStructureBank(t *testing.T) {
	testAdversarialConditionalGenericAssignmentBank(t, false)
}

func TestAdversarialClassFormalConditionalAssignmentHundredStructureBank(t *testing.T) {
	testAdversarialConditionalGenericAssignmentBank(t, true)
}

// Cross both original return declarations with the same independent selection,
// identity, count and failure model. Debug/policy variants are not structures.
func testAdversarialConditionalGenericAssignmentBank(t *testing.T, classFormal bool) {
	t.Helper()
	var owner, checks strings.Builder
	owner.WriteString(`interface BankLookup<E>{E lookup();}
class BankEntry{final int value;BankEntry(int n){value=n;}}
class BankProduct<E> implements BankLookup<E>{final E value;BankProduct(E e){value=e;}public E lookup(){return value;}}
`)
	if classFormal {
		owner.WriteString(`class BankProducer<E,R extends BankLookup<E>>{static int factories,steps;R result;static <E>BankProducer<E,BankProduct<E>> create(){factories++;return new BankProducer<E,BankProduct<E>>();}BankProducer<E,R> register(E v){result=(R)new BankProduct<E>(v);return this;}BankProducer<E,R> step(){steps++;return this;}R build(){return result;}}
`)
	} else {
		owner.WriteString(`class BankProducer<E>{static int factories,steps;E value;static <E>BankProducer<E> create(){factories++;return new BankProducer<E>();}BankProducer<E> register(E v){value=v;return this;}BankProducer<E> step(){steps++;return this;}BankProduct<E> build(){return new BankProduct<E>(value);}}
`)
	}
	owner.WriteString("public class ConditionalBankOwner{\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&owner, "private final BankLookup<BankEntry> r%d;public BankLookup<BankEntry> registry%d(){return r%d;}public BankEntry get%d(){return r%d.lookup();}\n", i, i, i, i, i)
	}
	owner.WriteString("public ConditionalBankOwner(boolean a,boolean b,BankLookup<BankEntry> provided,int seed){\n")
	for i := 0; i < 100; i++ {
		factory := fmt.Sprintf("BankProducer.<BankEntry>create().register(new BankEntry(seed+%d))%s.build()", i, strings.Repeat(".step()", i/10+1))
		var expression, providedSelected, factorySelected string
		switch i % 10 {
		case 0:
			expression, providedSelected, factorySelected = "provided!=null?provided:"+factory, "provided!=null", "provided==null"
		case 1:
			expression, providedSelected, factorySelected = "provided==null?"+factory+":provided", "provided!=null", "provided==null"
		case 2:
			expression, providedSelected, factorySelected = "a?(provided!=null?provided:"+factory+"):"+factory, "a&&provided!=null", "!a||provided==null"
		case 3:
			expression, providedSelected, factorySelected = "a?provided:"+factory, "a", "!a"
		case 4:
			expression, providedSelected, factorySelected = "a?"+factory+":provided", "!a", "a"
		case 5:
			expression, providedSelected, factorySelected = "a?(b?provided:"+factory+"):provided", "!a||b", "a&&!b"
		case 6:
			expression, providedSelected, factorySelected = "a?provided:(b?"+factory+":provided)", "a||!b", "!a&&b"
		case 7:
			expression, providedSelected, factorySelected = "provided!=null?provided:(a?"+factory+":null)", "provided!=null", "provided==null&&a"
		case 8:
			expression, providedSelected, factorySelected = "a?null:(provided!=null?provided:"+factory+")", "!a&&provided!=null", "!a&&provided==null"
		case 9:
			expression, providedSelected, factorySelected = "a?(provided!=null?provided:"+factory+"):null", "a&&provided!=null", "a&&provided==null"
		}
		fmt.Fprintf(&owner, "r%d=%s;\n", i, expression)
		fmt.Fprintf(&checks, `{BankLookup<BankEntry> actual=owner.registry%d();boolean selected=(%s),made=(%s);if(made){expectedFactories++;expectedSteps+=%d;if(actual==null||actual==provided||owner.get%d().value!=seed+%d)throw new AssertionError("factory value/identity %d");}else if(selected){if(actual!=provided)throw new AssertionError("provided identity %d");if(payload==2){try{owner.get%d();throw new AssertionError("lost payload check %d");}catch(ClassCastException expected){}}else if(provided!=null&&owner.get%d()!=token)throw new AssertionError("token identity %d");}else if(actual!=null)throw new AssertionError("null branch %d");if(actual==null){try{owner.get%d();throw new AssertionError("lost null failure %d");}catch(NullPointerException expected){}}rows++;}
`, i, providedSelected, factorySelected, i/10+1, i, i, i, i, i, i, i, i, i, i, i)
	}
	owner.WriteString("}}\nclass ConditionalBankDriver{public static void main(String[]args){int rows=0;BankEntry token=new BankEntry(19);for(int mode=0;mode<4;mode++)for(int payload=0;payload<3;payload++){boolean a=(mode&1)!=0,b=(mode&2)!=0;int seed=mode==0?Integer.MAX_VALUE:Integer.MIN_VALUE+mode;BankLookup<BankEntry> provided=payload==0?null:payload==1?new BankProduct<BankEntry>(token):(BankLookup)new BankProduct<String>(\"polluted\");BankProducer.factories=0;BankProducer.steps=0;ConditionalBankOwner owner=new ConditionalBankOwner(a,b,provided,seed);int expectedFactories=0,expectedSteps=0;\n")
	owner.WriteString(checks.String())
	owner.WriteString(`if(BankProducer.factories!=expectedFactories||BankProducer.steps!=expectedSteps)throw new AssertionError("factory/receiver evaluation count:"+BankProducer.factories+":"+BankProducer.steps);}System.out.println(rows+":conditional:100-structures:lazy:identity:null:payload:overflow");}}`)
	testNativePrivateSetterSourceFixture(t, map[string]string{"ConditionalBankOwner.java": owner.String()}, "ConditionalBankOwner", "ConditionalBankDriver", "1200:conditional:100-structures:lazy:identity:null:payload:overflow\n")
}

package javaclassparser

import (
	"strings"
	"testing"
)

const conditionalPacketFixture = `class ChoiceTarget{final int value;ChoiceTarget(int value){this.value=value;}ChoiceTarget(long value){throw new AssertionError("wrong widened target");}}
class ChoiceOwner{class Part extends ChoiceTarget{Part(int word){super(word>0?7:-3);}Object owner(){return ChoiceOwner.this;}}Part build(int word){return new Part(word);}}
class ChoiceDriver{public static void main(String[]args){ChoiceOwner owner=new ChoiceOwner();int rows=0;for(int word:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){ChoiceOwner.Part part=owner.build(word);if(part.value!=(word<=0?-3:7)||part.owner()!=owner)throw new AssertionError("conditional packet/binding/outer identity");rows++;}System.out.println(rows+":conditional:constructor:identity");}}`

func TestAdversarialConstructorConditionalPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, conditionalPacketFixture, []string{"ChoiceOwner"}, "ChoiceDriver", "5:conditional:constructor:identity\n", nativeLexicalExactSignatures)
}
func TestAdversarialConstructorConditionalPacketRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(conditionalPacketFixture, "Choice", "SeparateDecision")
	testNativeIndependentFamilyFixture(t, f, []string{"SeparateDecisionOwner"}, "SeparateDecisionDriver", "5:conditional:constructor:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorNestedConditionalPacketRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(conditionalPacketFixture, "super(word>0?7:-3)", "super(word>0?(word>1?7:9):(word==0?-3:-8))")
	f = strings.ReplaceAll(f, "word<=0?-3:7", "word<0?-8:word==0?-3:word==1?9:7")
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "5:conditional:constructor:identity\n", nativeLexicalExactSignatures)
}

const conditionalEffectsPacketFixture = `class ConditionalEvents{static String trace="";static final Exception shared=new Exception("kept identity");static int positive(int word)throws Exception{trace+="P";return 7;}static int other(int word)throws Exception{trace+="N";if(word==Integer.MIN_VALUE)throw shared;return -3;}}
class ChoiceTarget{final int value;ChoiceTarget(int value){this.value=value;}ChoiceTarget(long value){throw new AssertionError("wrong widened target");}}
class ChoiceOwner{class Part extends ChoiceTarget{Part(int word)throws Exception{super(word>0?ConditionalEvents.positive(word):ConditionalEvents.other(word));}Object owner(){return ChoiceOwner.this;}}Part build(int word)throws Exception{return new Part(word);}}
class ChoiceDriver{public static void main(String[]args)throws Exception{ChoiceOwner owner=new ChoiceOwner();int rows=0;for(int word:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){ConditionalEvents.trace="";try{ChoiceOwner.Part part=owner.build(word);if(word==Integer.MIN_VALUE||part.value!=(word<=0?-3:7)||part.owner()!=owner)throw new AssertionError("branch/binding/outer identity");}catch(Exception failure){if(word!=Integer.MIN_VALUE||failure!=ConditionalEvents.shared)throw failure;}if(!ConditionalEvents.trace.equals(word<=0?"N":"P"))throw new AssertionError("branch effect once and before delegate");rows++;}System.out.println(rows+":conditional:effects:identity");}}`

func TestAdversarialConstructorConditionalEffectsPacketRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, conditionalEffectsPacketFixture, []string{"ChoiceOwner"}, "ChoiceDriver", "5:conditional:effects:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorConditionalBooleanPacketRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(conditionalPacketFixture, "ChoiceTarget(int value){this.value=value;}", "ChoiceTarget(boolean selected,int value){this.value=selected?7:-3;}")
	f = strings.ReplaceAll(f, "super(word>0?7:-3)", "super(word>0?true:false,word)")
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "5:conditional:constructor:identity\n", nativeLexicalExactSignatures)
}

func TestAdversarialConstructorConditionalWidePacketRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(conditionalPacketFixture, "final int value;ChoiceTarget(int value)", "final long value;ChoiceTarget(long value)")
	f = strings.ReplaceAll(f, "ChoiceTarget(long value){throw", "ChoiceTarget(double value){throw")
	f = strings.ReplaceAll(f, "super(word>0?7:-3)", "super(word>0?70000000001L:-30000000001L)")
	f = strings.ReplaceAll(f, "word<=0?-3:7", "word<=0?-30000000001L:70000000001L")
	testNativeIndependentFamilyFixture(t, f, []string{"ChoiceOwner"}, "ChoiceDriver", "5:conditional:constructor:identity\n", nativeLexicalExactSignatures)
}

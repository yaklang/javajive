package javaclassparser

import (
	"strings"
	"testing"
)

const constructorForwardHierarchyFixture = `
interface SuffixFactory {int value();}
class SuffixEffects {static String trace="";static boolean fail;static final java.io.IOException error=new java.io.IOException("identity");}
class SuffixMaker implements SuffixFactory {
 static final SuffixMaker INSTANCE=new SuffixMaker(0);
 SuffixMaker(int ignored){}SuffixMaker()throws java.io.IOException{SuffixEffects.trace+="N";if(SuffixEffects.fail)throw SuffixEffects.error;}
 public int value(){return 17;}}
class SuffixOps {static SuffixMaker make()throws java.io.IOException{SuffixEffects.trace+="F";if(SuffixEffects.fail)throw SuffixEffects.error;return SuffixMaker.INSTANCE;}}
class SuffixOwner<U> {
 final U token;SuffixOwner(U token){this.token=token;}
 abstract static class Base<T>{abstract T value();}
 class Member extends Base<U>{
  final java.util.List<String> input;final SuffixFactory factory;
  Member(java.util.List<String> input)throws java.io.IOException{this(OPERAND,input);}
  Member(SuffixFactory factory,java.util.List<String> input){SuffixEffects.trace+="T";this.input=input;this.factory=factory;}
  U value(){return token;}}
}
class SuffixDriver {public static void main(String[]args)throws Exception{
 SuffixFactory initial=SuffixMaker.INSTANCE;int rows=0;
 java.lang.reflect.Type[] ports=SuffixOwner.Member.class.getDeclaredConstructor(SuffixOwner.class,java.util.List.class).getGenericParameterTypes();
 if(ports.length!=1||!(ports[0] instanceof java.lang.reflect.ParameterizedType)||((java.lang.reflect.ParameterizedType)ports[0]).getRawType()!=java.util.List.class||((java.lang.reflect.ParameterizedType)ports[0]).getActualTypeArguments()[0]!=String.class)throw new AssertionError("declared constructor ports");
 for(Object token:new Object[]{null,new Object()})for(java.util.List<String> input:new java.util.List[]{null,java.util.Arrays.asList("a",null,"字𝄞")})for(boolean fail:new boolean[]{false,true}){
  SuffixEffects.trace="";SuffixEffects.fail=fail;java.io.IOException error=null;SuffixOwner<Object> owner=new SuffixOwner<Object>(token);SuffixOwner<Object>.Member member=null;
  try{member=owner.new Member(input);}catch(java.io.IOException e){error=e;}
  boolean failed=fail&&CAN_FAIL;String expected=PREFIX+(failed?"":"T");
  if(error!=(failed?SuffixEffects.error:null)||!SuffixEffects.trace.equals(expected))throw new AssertionError("operand effects/exception identity: "+SuffixEffects.trace,error);
  if(!failed&&(member.value()!=token||((SuffixOwner.Base<Object>)member).value()!=token||member.input!=input||member.factory.value()!=17))throw new AssertionError("constructor binding/dispatch identity");rows++;
 }System.out.println(rows+":forward:hierarchy:ports:effects:identity");}}
`

func TestAdversarialForwardingConstructorsKeepDeclaredGenericPortsAcrossWidening(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, operand := range []struct{ label, expression, prefix, canFail string }{
		{"field", "SuffixMaker.INSTANCE", `""`, "false"},
		{"factory", "SuffixOps.make()", `"F"`, "true"},
		{"allocation", "new SuffixMaker()", `"N"`, "true"},
	} {
		for _, owner := range []string{"SuffixOwner", "RenamedForwardOwner"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(operand.label+"/"+owner+"/"+debug, func(t *testing.T) {
					fixture := strings.NewReplacer("OPERAND", operand.expression, "PREFIX", operand.prefix, "CAN_FAIL", operand.canFail, "SuffixOwner", owner).Replace(constructorForwardHierarchyFixture)
					constructorSuffixFamilyRoundTrip(t, fixture, debug, []string{owner, owner + "$Base", owner + "$Member"}, "8:forward:hierarchy:ports:effects:identity\n")
				})
			}
		}
	}
}

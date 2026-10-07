package javaclassparser

import (
	"strings"
	"testing"
)

// The literal arms return early; the inner default and the outer conditional's
// false edge share one terminal expression. Neither edge may become a throw or
// inherit a sibling outer case. The driver separately checks each effect.
const sharedSwitchReturnFixture = `class UnarySharedTail {
 static String trace="";
 static int consume(int op){trace+="C";return op;}
 static int peek(int token){trace+="P";return token;}
 static String recurse(int token){trace+="R";return "child:"+token;}
 static String make(int op,String child){trace+="M";return "expr:"+op+":"+child;}
 static String parse(int op,int token){switch(op){case 33:case 43:case 45:case 126:case 362:case 363:
  int consumed=consume(op);if(consumed==45){int look=peek(token);switch(look){case 401:case 402:case 403:trace+="I";return "int:"+look;case 404:case 405:trace+="D";return "double:"+look;}}
  return make(consumed,recurse(token));
 case 40:trace+="A";return "cast:"+token;
 default:trace+="F";return "post:"+token;}}
}
class UnarySharedDriver {public static void main(String[]args){int rows=0;for(int op:new int[]{33,43,45,126,362,363,40,0,Integer.MIN_VALUE})for(int token:new int[]{400,401,402,403,404,405,406,Integer.MIN_VALUE}){UnarySharedTail.trace="";String value=UnarySharedTail.parse(op,token);boolean unary=op==33||op==43||op==45||op==126||op==362||op==363;String expected,effects;if(unary){if(op==45&&token>=401&&token<=405){expected=(token<=403?"int:":"double:")+token;effects=token<=403?"CPI":"CPD";}else{expected="expr:"+op+":child:"+token;effects=op==45?"CPRM":"CRM";}}else{expected=(op==40?"cast:":"post:")+token;effects=op==40?"A":"F";}if(!value.equals(expected)||!UnarySharedTail.trace.equals(effects))throw new AssertionError(op+":"+token+":"+value+":"+UnarySharedTail.trace);rows++;}System.out.println(rows+":shared-switch:terminal-return:effects");}}
`

func TestAdversarialConditionalSwitchSharedTerminalReturnRoundTrip(t *testing.T) {
	for _, owner := range []string{"UnarySharedTail", "RenamedUnaryTail"} {
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterFixture(t, strings.ReplaceAll(sharedSwitchReturnFixture, "UnarySharedTail", owner), owner, "UnarySharedDriver", "72:shared-switch:terminal-return:effects\n")
		})
	}
}

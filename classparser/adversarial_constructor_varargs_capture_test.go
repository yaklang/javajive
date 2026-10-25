package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// javac's anonymous constructor can carry ACC_VARARGS while captured locals
// follow the superclass's array argument in its physical descriptor. A plain
// source constructor must preserve that reflection contract after proving its
// exact capture schedule and delegation, without pretending a capture is an array.
func TestAdversarialConstructorVarargsCaptureKeepsPhysicalPortsAndMarker(t *testing.T) {
	for _, rename := range []bool{false, true} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(fmt.Sprintf("renamed%v/%s", rename, debug), func(t *testing.T) {
				fixture := constructorVarargsCaptureFixture()
				if rename {
					fixture = strings.ReplaceAll(strings.ReplaceAll(fixture, "SuffixBase", "AlternateBase"), "token", "payload")
				}
				constructorSuffixRoundTrip(t, fixture, debug, "SuffixOwner$1", "72:constructor:closed:suffix:oracle\n")
			})
		}
	}
}

func constructorVarargsCaptureFixture() string {
	fixture := strings.ReplaceAll(strings.Replace(constructorSuffixFixture, "BODY", `result=seed*31L+mode;SuffixWitness.visits++;`, 1), "ROUNDS", "1")
	fixture = strings.Replace(fixture, "SuffixBase(Object input)", "SuffixBase(Object... inputs)", 1)
	fixture = strings.Replace(fixture, "if(input==null)", "if(inputs[0]==null)", 1)
	fixture = strings.Replace(fixture, "int rows=0;", `try{java.lang.reflect.Constructor<?>[] cs=Class.forName("SuffixOwner$1").getDeclaredConstructors();if(cs.length!=1||!cs[0].isVarArgs()||cs[0].getParameterTypes()[cs[0].getParameterCount()-1].isArray())throw new AssertionError("physical varargs capture ports");}catch(ClassNotFoundException e){throw new AssertionError(e);}int rows=0;`, 1)
	return fixture
}

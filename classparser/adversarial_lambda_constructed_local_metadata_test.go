package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLambdaConstructedLocalRetainsStructuredAssignmentAndCaptureNames(t *testing.T) {
	source := forwardedLambdaCaptureMetadataFixture
	for _, change := range [][2]string{
		{"public class UncapturedLambdaNames", "class LambdaConstructedHolder {static LambdaConstructedHolder seen;final Callable<String> source;LambdaConstructedHolder(Callable<String> source){this.source=source;}void first(){seen=this;}String value()throws Exception{if(seen!=this)throw new AssertionError(\"allocation identity\");return source.call();}}\npublic class UncapturedLambdaNames"},
		{"return source.call();};", "LambdaConstructedHolder holder=new LambdaConstructedHolder(source);holder.first();return holder.value();};"},
	} {
		if !strings.Contains(source, change[0]) {
			t.Fatal("missing independent constructed-local variation", change[0])
		}
		source = strings.Replace(source, change[0], change[1], 1)
	}
	testUncapturedLambdaParameterMetadataRoundTrip(t, source, 2, "37")
}

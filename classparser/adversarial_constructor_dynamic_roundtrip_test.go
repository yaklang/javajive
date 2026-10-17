package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorDynamicInvocationsPreserveCaptureAndFailure(t *testing.T) {
	for _, shape := range []string{"postinitialized captured factory", "preinitialized delegation"} {
		t.Run(shape, func(t *testing.T) {
			f := constructorUnknownAbruptCaptureFixture
			if shape == "postinitialized captured factory" {
				f = strings.Replace(f, "trace=trace*31+1;", "java.util.function.IntSupplier source=()->n;if(source.getAsInt()!=n)throw new AssertionError(\"dynamic producer\");trace=trace*31+1;", 1)
			} else {
				f = strings.Replace(f, "AbruptParent(int n)throws java.io.IOException {", "AbruptParent(int n)throws java.io.IOException {this(n,()->n);}AbruptParent(int n,java.util.function.IntSupplier source)throws java.io.IOException {if(source.getAsInt()!=n)throw new AssertionError(\"dynamic producer\");", 1)
			}
			for _, prefix := range []string{"Abrupt", "Kappa"} {
				t.Run(prefix, func(t *testing.T) {
					testConstructorReceiverTypeViewFixture(t, strings.ReplaceAll(f, "Abrupt", prefix), prefix, "-3:null:false\n-3:null:true\n-2:fresh:false\n-2:fresh:true\n-1:shared:false\n-1:shared:true\n0:ok:false\n0:ok:true\n7:ok:false\n7:ok:true\n70000:ok:false\n70000:ok:true\n")
				})
			}
		})
	}
}

package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialForeignSuperWithOwnPrivateBridgeRoundTrip(t *testing.T) {
	for _, chain := range []bool{false, true} {
		for _, profile := range []string{"ordinary", "renamed", "generic", "private member"} {
			fixture := crossFamilyEnclosingFixture
			if chain {
				fixture = strings.Replace(fixture, "Leaf(int n)throws java.io.IOException{super(n);}", "private Leaf(int n)throws java.io.IOException{this(n,0L);}private Leaf(int n,long ignored)throws java.io.IOException{super(n);}", 1)
			} else {
				fixture = strings.Replace(fixture, "Leaf(int n)throws java.io.IOException{super(n);}", "private Leaf(int n)throws java.io.IOException{super(n);}", 1)
			}
			owners := []string{"BindingBase", "BindingCurrent"}
			driver := "BindingDriver"
			switch profile {
			case "renamed":
				fixture = strings.NewReplacer("Binding", "Independent", "Member", "Ancestor", "Leaf", "Descendant").Replace(fixture)
				owners = []string{"IndependentBase", "IndependentCurrent"}
				driver = "IndependentDriver"
			case "generic":
				fixture = strings.ReplaceAll(fixture, "class BindingBase{", "class BindingBase<A,B>{")
				fixture = strings.ReplaceAll(fixture, "class Member{", "class Member{A left;B right;")
				fixture = strings.ReplaceAll(fixture, "class BindingCurrent extends BindingBase{", "class BindingCurrent<T> extends BindingBase<T,Long>{")
			case "private member":
				fixture = strings.ReplaceAll(fixture, "class Leaf extends", "private class Leaf extends")
				fixture = strings.ReplaceAll(fixture, "Leaf build(int n)", "Member build(int n)")
				fixture = strings.ReplaceAll(fixture, "BindingCurrent.Leaf", "BindingBase.Member")
			}
			t.Run(profile+"/"+map[bool]string{false: "private", true: "private-this"}[chain], func(t *testing.T) {
				testNativeIndependentFamilyFixture(t, fixture, owners, driver, "6:cross-family:outer:identity:callback\n", nativeLexicalExactSignatures)
			})
		}
	}
}

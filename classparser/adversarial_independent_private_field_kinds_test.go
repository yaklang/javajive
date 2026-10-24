package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialIndependentPrivateFieldsRetainWideStaticAndAccessorSemantics(t *testing.T) {
	variants := []struct{ name, source string }{
		{"private getter", strings.Replace(independentPrivateScopeFixture, "final int base;", "private final int base;", 1)},
		{"wide value", strings.NewReplacer("PRIVATE final int internal", "PRIVATE final long internal", "int base,int hidden", "int base,long hidden", "int saved()", "long saved()", "for(int hidden:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})", "for(long hidden:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})").Replace(independentPrivateScopeFixture)},
		{"static storage", strings.NewReplacer("PRIVATE final int internal", "PRIVATE static int internal", "this.internal=hidden", "Capsule.internal=hidden").Replace(independentPrivateScopeFixture)},
		{"equal object identity", strings.NewReplacer("PRIVATE final int internal", "PRIVATE final Object internal", "int base,int hidden", "int base,Object hidden", "int saved()", "Object saved()", "for(int hidden:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})", "for(Object hidden:new Object[]{null,PrivateScopeDriver.class,\"same\",new String(\"same\"),new Object()})").Replace(independentPrivateScopeFixture)},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			testIndependentPrivateScopeComponent(t, variant.source, "25:independent:private:component\n")
		})
	}
}

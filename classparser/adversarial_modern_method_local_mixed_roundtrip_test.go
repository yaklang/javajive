package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialModernMethodLocalAndAnonymousCaptureRoundTrip(t *testing.T) {
	fixture := strings.Replace(modernLocalCaptureFixture, "return new Entry();}\n}", "return new Entry();}\n ModernLocalBase anonymous(final long seed,final Object token){return new ModernLocalBase(){long word(long delta){return seed+delta;}Object token(){return token;}};}\n}", 1)
	fixture = strings.Replace(fixture, "for(boolean fail:new boolean[]{false,true}){", "for(boolean fail:new boolean[]{false,true})for(boolean anon:new boolean[]{false,true}){", 1)
	fixture = strings.Replace(fixture, "value=owner.make(seed,token)", "value=anon?owner.anonymous(seed,token):owner.make(seed,token)", 1)
	fixture = strings.Replace(fixture, "!c.isLocalClass()", "(anon?!c.isAnonymousClass():!c.isLocalClass())", 1)
	fixture = strings.Replace(fixture, "equals(\"make\")", "equals(anon?\"anonymous\":\"make\")", 1)
	fixture = strings.Replace(fixture, "equals(\"ModernLocalOwner$1Entry\")", "equals(anon?\"ModernLocalOwner$1\":\"ModernLocalOwner$1Entry\")", 1)
	testSourceTargetReleaseFamilyFixture(t, fixture, "ModernLocalOwner", "ModernLocalDriver", "200:modern-local:wide:identity:pre-super:exception:owner\n", "11", []int{11, 16})
}

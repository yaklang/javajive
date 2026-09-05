package javaclassparser

import "testing"

func TestHttpclientBrowserCompatSpecSuperArrayIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/BrowserCompatSpec.class", "JDEC_HTTPCLIENT_REMAINING_OFF",
		"super(new CommonCookieAttributeHandler[]{",
		"super(var3);")
}

func TestHttpclientAuthenticatorChallengeReturnIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/HttpAuthenticator.class", "JDEC_HTTPCLIENT_REMAINING_OFF",
		"return false;\n\t\t}catch(MalformedChallengeException var6_1){",
		"}\n\t\t}catch(MalformedChallengeException var6_1){")
}

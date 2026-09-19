package javaclassparser

import (
	"strings"
	"testing"
)

func TestLegacyRecoveryPreservesStructuredAssignmentsAndExits(t *testing.T) {
	t.Setenv("JDEC_NETTY_REMAINING_OFF", "")
	input := "class ReferenceCountedOpenSslContext {\nSSLContext.addCertificateCompressionAlgorithm(this.ctx,SSL.SSL_CERT_COMPRESSION_DIRECTION_DECOMPRESS,(CertificateCompressionAlgo)(var30));\ncontinue;\n}"
	if got := fixNettyRemainingReconstructs(input); got != input {
		t.Fatalf("structured continue changed: %s", got)
	}
	t.Setenv("JDEC_ZXING_REMAINING_OFF", "")
	input = "package com.google.zxing.oned;\nclass Code39Reader {void f(){int var8 = 0; if (((var8 = var4[var6]) < (var3)) && ((var8) > (var2))){use(var8);}}}"
	if got := fixZxingRemainingReconstructs(input); got != input {
		t.Fatalf("declared assignment target changed: %s", got)
	}
}

func TestGroupJoinIteratorCastFollowsRightsField(t *testing.T) {
	t.Setenv("JDEC_RXJAVA_REMAINING_OFF", "")
	for _, name := range []string{"var14", "var13_1"} {
		for _, prefix := range []string{"Iterator ", ""} {
			body := "class FlowableGroupJoin$GroupJoinSubscription<TRight> { void drain(){" + prefix + name + " = this.rights.values().iterator(); var10.onNext(" + name + ".next()); var10.onNext(var99.next());} }"
			got := fixRxjavaRemainingReconstructs(body)
			if !strings.Contains(got, "var10.onNext((TRight)("+name+".next()))") || !strings.Contains(got, "var10.onNext(var99.next())") {
				t.Fatalf("iterator provenance lost: %s", got)
			}
		}
	}
}

package javaclassparser

import "testing"

func TestPicocliConcatKeyCastIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AutoComplete.class", "JDEC_PICOCLI_REMAINING_OFF",
		"concat(\"_\",var1,(String)(var7.getKey()),",
		"concat(\"_\",var1,var7.getKey(),")
}

func TestPicocliQuoteElementsIntSlotIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/CommandLine$UnmatchedArgumentException.class", "JDEC_PICOCLI_REMAINING_OFF",
		"int var6 = 0;",
		"Object var6 = null;")
}

package javaclassparser

import "testing"

func TestAdversarialRetiredArrayArgumentTextRewritesKeepLiteralPayloads(t *testing.T) {
	// These are application data, even when they look exactly like an old
	// source workaround's trigger. The typed consumer owns real array views.
	const source = `class Payload { /* org.assertj ELEMENT extends */
 String array="IterableUtil.toArray((Iterable)(var1))";
 String pair="this.compareElementsOf(((Object[])(var1)),((Object[])(var2)))";
 String atomic="this.compareElementsOf(((AtomicReferenceArray)(var1)),((Object[])(var2)))";
}`
	if got := fixAssertjRemainingReconstructs(source); got != source {
		t.Fatalf("array text substitution changed literal payloads:\n%s", got)
	}
}

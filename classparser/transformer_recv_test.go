package javaclassparser

// 承重测试: Transformer<? super E, ? extends E>.transform(x) 的接收者需 raw
// `((Transformer)(recv))` 造型, 否则 javac 把 ? super E 捕获为 CAP#1 拒 Object 实参。
// 镜像 commons-collections4 TransformedCollection.transformedCollection。
// kill-switch: JDEC_WILDCARD_CONSUMER_RECV_OFF。

import (
	"os"
	"strings"
	"testing"
)

func TestTransformerWildcardReceiverRawCastIsLoadBearing(t *testing.T) {
	seed, err := os.ReadFile("testdata/regression/TransformedCollection.class")
	if err != nil {
		t.Fatalf("read TransformedCollection: %v", err)
	}
	base, err := os.ReadFile("testdata/regression/AbstractCollectionDecorator.class")
	if err != nil {
		t.Fatalf("read AbstractCollectionDecorator: %v", err)
	}
	tf, err := os.ReadFile("testdata/regression/Transformer.class")
	if err != nil {
		t.Fatalf("read Transformer: %v", err)
	}
	resolver := func(internalName string) ([]byte, bool) {
		switch {
		case strings.HasSuffix(internalName, "TransformedCollection"):
			return seed, true
		case strings.HasSuffix(internalName, "AbstractCollectionDecorator"):
			return base, true
		case strings.HasSuffix(internalName, "Transformer"):
			return tf, true
		default:
			return nil, false
		}
	}

	os.Unsetenv("JDEC_WILDCARD_CONSUMER_RECV_OFF")
	on, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "((Transformer)") {
		t.Errorf("fix ON: expected raw `((Transformer)(recv)).transform`, got:\n%s", on)
	}

	t.Setenv("JDEC_WILDCARD_CONSUMER_RECV_OFF", "1")
	off, err := DecompileWithResolver(seed, resolver)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if strings.Contains(off, "((Transformer)") {
		t.Errorf("fix OFF: expected no raw Transformer receiver cast, got:\n%s", off)
	}
}

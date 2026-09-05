package javaclassparser

// 承重测试: IpSubnetFilter(IpSubnetFilterRule... rules) 委派
// `this(true, Arrays.asList(checkNotNull(rules)))`。checkNotNull 擦成 Object,
// arrayParamRefArgCast 会补 `(Object[])` 使 asList 推断 List<Object>, 绑不到
// IpSubnetFilter(boolean, List<IpSubnetFilterRule>)。
// 治法(JDEC_NETTY_REMAINING_OFF): dump 重构把 `(Object[])` 换成 `(IpSubnetFilterRule[])`。

import (
	"os"
	"strings"
	"testing"
)

func TestIpSubnetFilterAsListCastIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/IpSubnetFilter.class")
	if err != nil {
		t.Fatalf("read IpSubnetFilter: %v", err)
	}

	os.Unsetenv("JDEC_NETTY_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "asList((IpSubnetFilterRule[])") &&
		!strings.Contains(on, "asList(((IpSubnetFilterRule[])") {
		t.Errorf("fix ON: expected asList((IpSubnetFilterRule[])(checkNotNull...)), got:\n%s", on)
	}
	if strings.Contains(on, "asList(((Object[])") || strings.Contains(on, "asList((Object[])") {
		t.Errorf("fix ON: did not expect asList((Object[])...), got:\n%s", on)
	}

	t.Setenv("JDEC_NETTY_REMAINING_OFF", "1")
	off, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile OFF: %v", err)
	}
	if !strings.Contains(off, "asList(((Object[])") && !strings.Contains(off, "asList((Object[])") {
		t.Errorf("fix OFF: expected asList((Object[])...) (kill-switch load-bearing), got:\n%s", off)
	}
}

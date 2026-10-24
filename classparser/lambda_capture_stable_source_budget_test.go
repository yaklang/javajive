package javaclassparser

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialStableLambdaDiscoveryWithoutCapturedBindingsSkipsSource(t *testing.T) {
	for _, scenario := range []string{"no factories", "absent receipt", "ordinary modern body", "uncaptured binding"} {
		t.Run(scenario, func(t *testing.T) {
			d := &ClassObjectDumper{obj: &ClassObject{}, lambdaMethodSourceBodies: map[*MemberInfo]*lambdaMethodSourceReceipt{}}
			switch scenario {
			case "absent receipt":
				d.lambdaMethodSourceBodies[&MemberInfo{}] = nil
			case "ordinary modern body":
				d.lambdaMethodSourceBodies[&MemberInfo{}] = &lambdaMethodSourceReceipt{body: &dumpedMethods{}}
			case "uncaptured binding":
				d.lambdaMethodSourceBodies[&MemberInfo{}] = &lambdaMethodSourceReceipt{body: &dumpedMethods{lambdaParameterBinding: &lambdaParameterBinding{metadata: &lambdaParameterMetadata{}}}}
			}
			d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 2, MaxOutputBytes: 1})
			source := strings.Repeat("/* not an original captured factory: -> */\n", 4096)
			if got := d.completedLambdaStableSourceBindings(source, nil); len(got) != 0 || d.Work.Err() != nil || d.Work.Used(workbudget.CounterGraphScans) != int64(len(d.lambdaMethodSourceBodies)) {
				t.Fatal("inapplicable capture protection scanned source or reserved storage", got, d.Work.Err(), d.Work.Used(workbudget.CounterGraphScans))
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &ClassObjectDumper{obj: &ClassObject{}, Work: workbudget.New(ctx, workbudget.Limits{}), lambdaMethodSourceBodies: map[*MemberInfo]*lambdaMethodSourceReceipt{nil: nil}}
	if got := d.completedLambdaStableSourceBindings("class C{}", nil); len(got) != 0 || d.Work.Err() == nil {
		t.Fatal("inventory concealed cancellation")
	}
}

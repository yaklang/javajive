package javaclassparser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeSerializableEmissionRollbackKeepsFactoryButNotFailedRender(t *testing.T) {
	// Exercise the actual retained writer across rollback. The original
	// protocol/target decision is tested independently with executable fixtures.
	object, method, code := &ClassObject{MajorVersion: 52}, &MemberInfo{}, &CodeAttribute{}
	plan := &serializableReferenceProtocol{object: object, sites: []serializableReferenceSite{{method: method, code: code, pc: 17, tuple: serializableReferenceTuple{name: "apply", descriptor: "()Ljava/util/function/Function;", instantiated: "(Ljava/lang/String;)Ljava/lang/String;"}}}}
	c := &ClassObjectDumper{obj: object, serializableReferenceReady: true, serializableReferencePlan: plan, serializableReferencePackets: map[int]*serializableReferencePacket{}, FuncCtx: &class_context.ClassContext{InvocationMetadata: func(string) (callbinding.Class, bool) { return callbinding.Class{}, false }}}
	request := core.CallSiteRequest{Identity: core.BootstrapIdentity{Owner: "java.lang.invoke.LambdaMetafactory", Name: "altMetafactory", Descriptor: "(Ljava/lang/invoke/MethodHandles$Lookup;Ljava/lang/String;Ljava/lang/invoke/MethodType;[Ljava/lang/Object;)Ljava/lang/invoke/CallSite;", RefKind: 6}, CallSiteName: "apply", CallSiteDescriptor: "()Ljava/util/function/Function;", OriginPC: 17, ClassMajor: 52}
	packet, known := c.serializableReferenceFactorySource(method, code, request)
	if !known {
		t.Fatal("original physical factory packet missing")
	}
	spelling := "Links::Aa"
	value := values.NewStreamingCustomValue(func(_ *class_context.ClassContext, out *workbudget.Writer) error { return out.WriteString(spelling) }, func() types.JavaType { return types.NewJavaClass("java.util.function.Function") })
	value.Flag, value.IsMethodRef, value.NoOuterCapture, value.CapturesKnown = "lambda", true, true, true
	value.OriginPC, value.HasOriginPC, value.InstantiatedMtdDesc = 17, true, "(Ljava/lang/String;)Ljava/lang/String;"
	value.MarkOriginalLambdaFactory(17)
	retained, known := packet.Retain(value)
	if !known || retained == nil {
		t.Fatal("original method reference not retained")
	}
	write := func() {
		t.Helper()
		out := workbudget.NewWriter(nil)
		if err := retained.WriteFunc(c.FuncCtx, out); err != nil || out.String() != spelling {
			t.Fatal("actual retained writer", err, out.String())
		}
	}
	before, known := c.snapshotLambdaSourceConsumption()
	if !known {
		t.Fatal("empty emission snapshot")
	}
	write()
	token := c.serializableReferencePackets[0]
	old := c.serializableReferenceEmissions[token]
	if len(old.fingerprints) != 1 {
		t.Fatal("first actual render not observed")
	}
	c.restoreLambdaSourceConsumption(before)
	if c.serializableReferenceFingerprintCount != 0 || len(c.serializableReferenceEmissions[token].fingerprints) != 0 {
		t.Fatal("failed render survived rollback")
	}
	spelling = "Links::BB"
	write()
	current := c.serializableReferenceEmissions[token]
	fingerprint, _ := serializableReferenceFingerprintOf(spelling)
	if current == old || len(current.fingerprints) != 1 || current.fingerprints[0] != fingerprint || len(old.fingerprints) != 1 || c.serializableReferenceFingerprintCount != 1 {
		t.Fatal("late writer retained an obsolete transaction")
	}
	sealed, known := c.snapshotLambdaSourceConsumption()
	if !known {
		t.Fatal("completed emission snapshot")
	}
	// Alter the existing cached observation before append can reallocate its
	// backing array. Otherwise a shallow-copy mutant escapes this assertion.
	current.fingerprints[0] = serializableReferenceFingerprint{}
	spelling = "Links::Aa"
	write()
	c.restoreLambdaSourceConsumption(sealed)
	if len(c.serializableReferenceEmissions[token].fingerprints) != 1 || c.serializableReferenceEmissions[token].fingerprints[0] != fingerprint || c.serializableReferenceFingerprintCount != 1 {
		t.Fatal("snapshot aliased mutable emission observations")
	}
}

func TestNativeSerializableFingerprintsRequireActualCodeAndBoundedObservation(t *testing.T) {
	text := "((java.util.function.Function<String,String> & java.io.Serializable)(Links::Aa))"
	fingerprint, known := serializableReferenceFingerprintOf(text)
	if !known {
		t.Fatal("direct reference not fingerprinted")
	}
	for _, tc := range []struct {
		name, source string
		count        int
		known        bool
	}{
		{"single", "return " + text + ";", 1, true},
		{"two", text + ";" + text, 2, true},
		{"quoted", fmt.Sprintf("String s=%q;", text), 0, true},
		{"comments", "/*" + text + "*/ //" + text + "\n", 0, true},
		{"changed target", strings.ReplaceAll(text, "::Aa", "::BB"), 0, true},
		{"text block", `"""` + text + `"""`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, known := serializableReferenceFingerprintCount(tc.source, fingerprint, func(int64) bool { return true })
			if count != tc.count || known != tc.known {
				t.Fatal("source occurrence", count, known, tc.count, tc.known)
			}
		})
	}
	if _, known := serializableReferenceFingerprintCount(text, fingerprint, func(int64) bool { return false }); known {
		t.Fatal("exhausted publication work accepted")
	}
	for _, invalid := range []string{"null", `"Links::Aa"`, "/* Links::Aa */", "Links::Aa;Links::BB"} {
		if _, known := serializableReferenceFingerprintOf(invalid); known {
			t.Fatal("non-reference writer packet accepted", invalid)
		}
	}
	object := &ClassObject{}
	plan := &serializableReferenceProtocol{object: object, sites: []serializableReferenceSite{{}}}
	token := &serializableReferencePacket{plan: plan, index: 0}
	c := &ClassObjectDumper{obj: object, serializableReferencePlan: plan, serializableReferencePackets: map[int]*serializableReferencePacket{0: token}}
	for i := 0; i < 16; i++ {
		if err := c.recordSerializableReferenceEmission(token, fmt.Sprintf("Links::method%d", i)); err != nil {
			t.Fatal("bounded distinct render", i, err)
		}
	}
	if err := c.recordSerializableReferenceEmission(token, "Links::method0"); err != nil {
		t.Fatal("repeat observation consumed distinct-render capacity", err)
	}
	if err := c.recordSerializableReferenceEmission(token, "Links::method16"); err == nil {
		t.Fatal("distinct-render bound not enforced")
	}
	foreign := *token
	if err := c.recordSerializableReferenceEmission(&foreign, "Links::method0"); err == nil {
		t.Fatal("equal labels borrowed the original factory identity")
	}
}

func TestNativeSerializableSnapshotsPreserveExistingAuthorityLimits(t *testing.T) {
	for _, test := range []struct {
		name                 string
		ordinary, serialized int
		accepted             bool
	}{
		{"existing boundary", 12288, 0, true},
		{"existing overflow", 12289, 0, false},
		{"independent domains at boundary", 12288, 256, true},
		{"ordinary overflow cannot borrow serial capacity", 12289, 1, false},
		{"serial overflow cannot borrow ordinary capacity", 0, 257, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := &ClassObjectDumper{lambdaMethodSourceBodies: map[*MemberInfo]*lambdaMethodSourceReceipt{}, nativeLambdaFactorySources: map[*nativeLambdaLocalCaptureSite]*nativeLambdaLocalCaptureSource{}, nativeLambdaLocalSources: map[string]*nativeLambdaLocalCaptureSource{}, serializableReferenceEmissions: map[*serializableReferencePacket]*serializableReferenceEmission{}}
			for i := 0; i < test.ordinary; i++ {
				switch i % 3 {
				case 0:
					c.lambdaMethodSourceBodies[&MemberInfo{}] = nil
				case 1:
					c.nativeLambdaFactorySources[&nativeLambdaLocalCaptureSite{}] = nil
				case 2:
					c.nativeLambdaLocalSources[fmt.Sprintf("local%d", i)] = nil
				}
			}
			for i := 0; i < test.serialized; i++ {
				c.serializableReferenceEmissions[&serializableReferencePacket{index: i}] = &serializableReferenceEmission{}
			}
			if _, accepted := c.snapshotLambdaSourceConsumption(); accepted != test.accepted {
				t.Fatal("one source authority domain borrowed another domain's capacity", accepted, test.accepted)
			}
		})
	}
}

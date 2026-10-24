package javaclassparser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialConstructorRecoveryEvidencePreservesTheExistingDecision(t *testing.T) {
	for _, namespace := range []string{"CaptureMotion", "CtorEvidence"} {
		t.Run(namespace, func(t *testing.T) {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(captureMotionFixture, "public class ", "class ")
					fixture = strings.Replace(fixture, "class CaptureMotionOwner {", "class CaptureMotionPassing {CaptureMotionPassing(){accept(this);}static void accept(Object value){}Object capture(){return null;}}\nclass CaptureMotionAllocating {final Object local=new Object(); Object capture(){return null;}}\nclass CaptureMotionOwner {", 1)
					fixture = strings.Replace(fixture, "CaptureMotionSilent plain(final Object token){", "CaptureMotionPassing passing(final Object token){return new CaptureMotionPassing(){Object capture(){return token;}};}\n CaptureMotionAllocating allocating(final Object token){return new CaptureMotionAllocating(){Object capture(){return token;}};}\n CaptureMotionSilent plain(final Object token){", 1)
					files := nativeCompileDebugClasses(t, strings.ReplaceAll(fixture, "CaptureMotion", namespace), debug)
					for _, kind := range []string{"silent", "observing", "no-argument", "missing parent", "allocating", "passing"} {
						t.Run(kind, func(t *testing.T) {
							parentName := namespace + map[string]string{"silent": "Base", "observing": "Observer", "no-argument": "Silent", "missing parent": "Base", "allocating": "Allocating", "passing": "Passing"}[kind]
							name := ""
							for candidate, data := range files {
								if !strings.HasPrefix(candidate, namespace+"Owner$") {
									continue
								}
								obj, err := Parse(data)
								if err != nil {
									t.Fatal(err)
								}
								if obj.GetSupperClassName() == parentName {
									if name != "" {
										t.Fatal("ambiguous authored constructor owner")
									}
									name = candidate
								}
							}
							raw := files[name]
							if len(raw) == 0 {
								t.Fatal("missing authored input")
							}
							requests := map[string]int{}
							options := DecompileOptions{Mode: Precision, TargetSourceVersion: 8, Resolve: func(name string) ([]byte, bool) {
								requests[name]++
								if kind == "missing parent" && name == namespace+"Base" {
									return nil, false
								}
								value, ok := files[name+".class"]
								return value, ok
							}}
							// Warm platform metadata independently of the observation.
							if _, err := DecompileWithOptions(raw, options); err != nil {
								t.Fatal(err)
							}
							clear(requests)
							off, err := DecompileWithOptions(raw, options)
							if err != nil {
								t.Fatal(err)
							}
							before := map[string]int{}
							for name, calls := range requests {
								before[name] = calls
							}
							clear(requests)
							options.EnableRecoveryEvidence = true
							on, err := DecompileWithOptions(raw, options)
							if err != nil {
								t.Fatal(err)
							}
							if !reflect.DeepEqual(before, requests) {
								t.Fatalf("observer made additional resolution queries: %v -> %v", before, requests)
							}
							if len(off.RecoveryEvidence) != 0 || off.RecoveryEvidenceTruncated || len(on.RecoveryEvidence) != 1 || on.RecoveryEvidenceTruncated {
								t.Fatalf("unexpected coverage: off=%v on=%v", off.RecoveryEvidence, on.RecoveryEvidence)
							}
							observation := on.RecoveryEvidence[0]
							want := RecoveryRuleProved
							gate := "motion.capture_store_motion"
							if kind == "observing" || kind == "missing parent" || kind == "allocating" || kind == "passing" {
								want, gate = RecoveryRuleUnproved, "motion.constructor_chain_observation"
							}
							if observation.State != want || observation.Obligation != gate || observation.InputHash != fmt.Sprintf("%x", sha256.Sum256(raw)) || observation.Owner != strings.TrimSuffix(name, ".class") || observation.Method != "<init>" || len(observation.CodeHash) != 64 || observation.DelegationPC < 0 || observation.TargetSource != 8 || observation.SourceCompiler != ModernJavac {
								t.Fatalf("wrong original identity or sufficient-rule result: %+v", observation)
							}
							if kind == "passing" || kind == "observing" {
								r := observation.BlockedBy
								expected := "motion.receiver_argument_confinement"
								if kind == "observing" {
									expected = "motion.initialized_receiver_dispatch_coverage"
								}
								if r == nil || r.Obligation != expected || r.Owner != parentName || r.Method != "<init>" || r.Descriptor != "()V" || r.PC < 0 || r.ClassHash != fmt.Sprintf("%x", sha256.Sum256(files[parentName+".class"])) {
									t.Fatalf("lost receiver observation boundary: %+v", r)
								}
							}
							if kind == "allocating" {
								r := observation.BlockedBy
								if r == nil || r.Obligation != "motion.exceptional_exit_finalizer_coverage" || r.Owner != namespace+"Allocating" || r.Method != "<init>" || r.Descriptor != "()V" || r.PC < 0 || len(r.CodeHash) != 64 || r.ClassHash != fmt.Sprintf("%x", sha256.Sum256(files[namespace+"Allocating.class"])) || r.RuntimeProfile != 8 {
									t.Fatalf("lost the actual parent NEW/finalizer-coverage dependency: %+v", r)
								}
								parent, e := Parse(files[namespace+"Allocating.class"])
								if e != nil {
									t.Fatal(e)
								}
								found := false
								for _, m := range parent.Methods {
									n, _ := parent.getUtf8(m.NameIndex)
									d, _ := parent.getUtf8(m.DescriptorIndex)
									if n != r.Method || d != r.Descriptor {
										continue
									}
									for _, attribute := range m.Attributes {
										if code, ok := attribute.(*CodeAttribute); ok {
											if r.PC >= len(code.Code) || code.Code[r.PC] != core.OP_NEW || r.CodeHash != fmt.Sprintf("%x", sha256.Sum256(code.Code)) {
												t.Fatal("refusal does not identify the original NEW event")
											}
											found = true
										}
									}
								}
								if !found {
									t.Fatal("missing original parent declaration/code")
								}

							}
							on.RecoveryEvidence = nil
							if !on.RecoveryEvidenceEnabled || off.RecoveryEvidenceEnabled {
								t.Fatal("unobserved requests are not distinguishable from an empty observation")
							}
							on.RecoveryEvidenceEnabled = false
							if !reflect.DeepEqual(off, on) {
								t.Fatal("shadow evidence changed source, status, member completeness or diagnostics")
							}
						})
					}
				})
			}
		})
	}
}

func TestAdversarialRecoveryObservationCapDoesNotGrantAProofOrLeakRequests(t *testing.T) {
	files := nativeCompileDebugClasses(t, "class ObservationRoot {}", "none")
	obj, err := Parse(files["ObservationRoot.class"])
	if err != nil {
		t.Fatal(err)
	}
	result := &DecompileResult{InputHash: "test input identity", Status: "unsupported"}
	dumper := &ClassObjectDumper{obj: obj, report: result, options: DecompileOptions{EnableRecoveryEvidence: true}}
	decision := unprovedConstructorMotion("motion.constructor_chain_observation")
	for i := 0; i < maxRecoveryObservations+2; i++ {
		dumper.observeConstructorMotion(decision, nil, nil, nil)
	}
	if len(result.RecoveryEvidence) != maxRecoveryObservations || !result.RecoveryEvidenceTruncated || result.Status != "unsupported" {
		t.Fatal("evidence limit changed a recovery decision")
	}
	for _, observation := range result.RecoveryEvidence {
		if observation.State != RecoveryRuleUnproved || observation.DelegationPC != -1 || observation.CodeHash != "" {
			t.Fatal("missing witnesses were replaced by a proved result")
		}
	}
	fresh := &DecompileResult{}
	dumper.report = fresh
	dumper.options.EnableRecoveryEvidence = false
	dumper.observeConstructorMotion(decision, nil, nil, nil)
	if len(fresh.RecoveryEvidence) != 0 || fresh.RecoveryEvidenceTruncated {
		t.Fatal("request-local evidence leaked into an unobserved request")
	}
}

func TestAdversarialRecoveryObservationOnlyReadsExistingBudgetFailure(t *testing.T) {
	files := nativeCompileDebugClasses(t, "class ObservationBudget {}", "none")
	obj, err := Parse(files["ObservationBudget.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		budget := workbudget.New(ctx, workbudget.Limits{MaxGraphScans: 1})
		want := RecoveryResourceLimit
		if canceled {
			cancel()
			want = RecoveryCanceled
		}
		if err := budget.Charge(workbudget.CounterGraphScans, 2); err == nil {
			t.Fatal("missing deliberate budget/cancel refusal")
		}
		cancel()
		before, failure := budget.Snapshot(), budget.Err()
		result := &DecompileResult{Status: "unsupported"}
		dumper := &ClassObjectDumper{obj: obj, report: result, Work: budget, options: DecompileOptions{EnableRecoveryEvidence: true}}
		dumper.observeConstructorMotion(unprovedConstructorMotion("motion.exception_domain"), nil, nil, nil)
		if len(result.RecoveryEvidence) != 1 || result.RecoveryEvidence[0].State != want || result.RecoveryEvidence[0].Budget == nil || !reflect.DeepEqual(before, budget.Snapshot()) || budget.Err() != failure || result.Status != "unsupported" {
			t.Fatal("observer charged work, lost the failure category, or changed recovery")
		}
		if !canceled {
			observed := result.RecoveryEvidence[0].Budget
			if observed.Counter != workbudget.CounterGraphScans || observed.Limit != 1 || observed.Used != 0 {
				t.Fatalf("wrong recorded budget witness: %+v", observed)
			}
		}
	}
}

func TestAdversarialRecoveryObservationIdentifiesContainingMethodAtDynamicEvent(t *testing.T) {
	files := nativeCompileClasses(t, receiverDynamicEvidenceFixture)
	obj, code, ops, index := receiverDynamicEvidence(t, files, "DynamicEvidenceInput")
	d := NewClassObjectDumper(obj)
	d.options.TargetSourceVersion = 8
	d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	d.FuncCtx = &class_context.ClassContext{}
	d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
	var refusal *RecoveryMotionRefusal
	d.constructorMotionRefusal = &refusal
	remaining := 512
	if d.constructorReceiverEffects(obj, code, ops, "(Ljava/lang/Object;)V", map[string]bool{"DynamicEvidenceInput\x00unseenCapture\x00Ljava/lang/Object;": true}, map[string]bool{}, &remaining, 0) {
		t.Fatal("open finalizer was incorrectly proved")
	}
	if refusal == nil || refusal.Method != "<init>" || refusal.Descriptor != "(Ljava/lang/Object;)V" || refusal.PC != int(ops[index].CurrentOffset) {
		t.Fatalf("dynamic call type replaced the containing method identity: %+v", refusal)
	}
}

func TestAdversarialRecoveryObservationCodeHashHasABoundedIdentityDomain(t *testing.T) {
	for _, length := range []int{0, 1, 65535, 65536, 1000000} {
		code := &CodeAttribute{Code: make([]byte, length)}
		got := recoveryObservationCodeHash(code)
		if length == 0 || length > 65535 {
			if got != "" {
				t.Fatalf("invented a valid Code hash for length %d", length)
			}
		} else if got != fmt.Sprintf("%x", sha256.Sum256(code.Code)) {
			t.Fatalf("lost original Code identity for length %d", length)
		}
	}
	if recoveryObservationCodeHash(nil) != "" {
		t.Fatal("missing Code has an identity")
	}
}

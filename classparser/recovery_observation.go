package javaclassparser

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// RecoveryObligationObservation describes one evaluation of an existing rule,
// not a certificate for the whole class or a count of distinct failed inputs.
// An unproved sufficient rule does not establish source unexpressibility.
// This initial observer covers the ordinary constructor capture-motion
// candidate only; other lexical candidates and their ALL/ANY dependencies have
// not yet been observed. Consumers must retain that coverage boundary.
type RecoveryObligationObservation struct {
	Schema         string                     `json:"schema"`
	Candidate      string                     `json:"candidate"`
	State          RecoveryProofState         `json:"state"`
	Obligation     string                     `json:"obligation"`
	Scope          string                     `json:"scope"`
	InputHash      string                     `json:"input_hash"`
	Owner          string                     `json:"owner"`
	Method         string                     `json:"method,omitempty"`
	Descriptor     string                     `json:"descriptor,omitempty"`
	CodeHash       string                     `json:"code_hash,omitempty"`
	DelegationPC   int                        `json:"delegation_pc"`
	TargetSource   int                        `json:"target_source"`
	SourceCompiler SourceCompilerProfile      `json:"source_compiler"`
	Budget         *RecoveryBudgetObservation `json:"budget,omitempty"`
	BlockedBy      *RecoveryMotionRefusal     `json:"blocked_by,omitempty"`
}

// This is a dependency of the observed candidate, not a complete effect
// summary. In particular an uncovered possible finalizer is UNPROVED, not a
// demonstrated observer or an assertion that the source has no legal layout.
type RecoveryMotionRefusal struct {
	Obligation     string `json:"obligation"`
	Owner          string `json:"owner"`
	Method         string `json:"method,omitempty"`
	Descriptor     string `json:"descriptor"`
	PC             int    `json:"pc"`
	CodeHash       string `json:"code_hash"`
	ClassHash      string `json:"class_hash,omitempty"`
	RuntimeProfile int    `json:"runtime_profile"`
}

type RecoveryBudgetObservation struct {
	Kind    string             `json:"kind"`
	Counter workbudget.Counter `json:"counter"`
	Used    int64              `json:"used"`
	Limit   int64              `json:"limit"`
}

type RecoveryProofState string

const (
	RecoveryRuleProved    RecoveryProofState = "PROVED_BY_EXISTING_RULE"
	RecoveryNotApplicable RecoveryProofState = "NOT_APPLICABLE"
	RecoveryRuleUnproved  RecoveryProofState = "UNPROVED"
	RecoveryResourceLimit RecoveryProofState = "RESOURCE_LIMIT"
	RecoveryCanceled      RecoveryProofState = "CANCELED"
)

type constructorMotionDecision struct {
	state      RecoveryProofState
	obligation string
	refusal    *RecoveryMotionRefusal
}

func unprovedConstructorMotion(obligation string) constructorMotionDecision {
	return constructorMotionDecision{state: RecoveryRuleUnproved, obligation: obligation}
}

// This separate observation cap does not spend the analysis graph/update
// budget, invoke a resolver, re-run a proof, or turn truncated evidence into a
// successful result. Absence of an observation means NOT_OBSERVED, never PASS.
const maxRecoveryObservations = 512

// The observer never hashes more than the JVM Code length domain. Oversized
// or missing Code remains without a hash; this does not change parser policy
// or turn absent identity evidence into a proof.
func recoveryObservationCodeHash(code *CodeAttribute) string {
	if code == nil || len(code.Code) == 0 || len(code.Code) > 65535 {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(code.Code))
}

func (c *ClassObjectDumper) observeConstructorMotion(decision constructorMotionDecision, p *constructorSourceBoundary, code *CodeAttribute, method *MemberInfo) {
	if c == nil || !c.options.EnableRecoveryEvidence || c.report == nil || c.obj == nil {
		return
	}
	if len(c.report.RecoveryEvidence) >= maxRecoveryObservations {
		c.report.RecoveryEvidenceTruncated = true
		return
	}
	observation := RecoveryObligationObservation{
		Schema: "javajive.recovery-observation/v1", Candidate: "constructor.capture_store_motion/v1",
		State: decision.state, Obligation: decision.obligation,
		Scope:     "existing_constructor_rule_in_request_symbolic_resolution_domain",
		InputHash: c.report.InputHash, Owner: c.obj.GetClassName(), DelegationPC: -1,
		TargetSource: c.options.TargetSourceVersion, SourceCompiler: c.options.SourceCompiler,
		BlockedBy: decision.refusal,
	}
	if observation.TargetSource == 0 {
		observation.TargetSource = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	if observation.SourceCompiler == "" {
		observation.SourceCompiler = ModernJavac
	}
	if p != nil {
		observation.DelegationPC = p.pc
	}
	if code != nil {
		observation.CodeHash = recoveryObservationCodeHash(code)
	}
	if method != nil {
		observation.Method, _ = c.obj.getUtf8(method.NameIndex)
		observation.Descriptor, _ = c.obj.getUtf8(method.DescriptorIndex)
	}
	// Err reads only a previously recorded failure. Do not check a deadline or
	// charge a counter here: the observer cannot change the next rule's budget.
	var limit *workbudget.Error
	if decision.state == RecoveryRuleUnproved && errors.As(c.Work.Err(), &limit) {
		observation.State = RecoveryResourceLimit
		if limit.Kind == workbudget.KindCanceled {
			observation.State = RecoveryCanceled
		}
		observation.Budget = &RecoveryBudgetObservation{limit.Kind, limit.Counter, limit.Used, limit.Limit}
	}
	c.report.RecoveryEvidence = append(c.report.RecoveryEvidence, observation)
}

// Preserve exactly the original guard. Observing a refusal grants no effect
// permission and does not requery a resolver. The optional sink is shared only
// by the copied dumpers participating in this one existing constructor proof.
func (c *ClassObjectDumper) constructorExitObservationProved(initialized bool, obj *ClassObject, code *CodeAttribute, descriptor string, pc int) bool {
	if !initialized || c.constructorReceiverFinalizerSilent {
		return true
	}
	if c.constructorMotionRefusal != nil && *c.constructorMotionRefusal == nil && obj != nil && code != nil {
		r := &RecoveryMotionRefusal{Obligation: "motion.exceptional_exit_finalizer_coverage", Owner: obj.GetClassName(), Descriptor: descriptor, PC: pc, CodeHash: recoveryObservationCodeHash(code), RuntimeProfile: c.options.TargetSourceVersion}
		// Read the original member/code identity. A descriptor by itself must
		// never choose among different method declarations.
		matches, remaining := 0, 1024
		for _, method := range obj.Methods {
			remaining--
			if remaining < 0 {
				matches = 0
				break
			}
			desc, err := obj.getUtf8(method.DescriptorIndex)
			if err != nil || desc != descriptor {
				continue
			}
			for _, attribute := range method.Attributes {
				remaining--
				if remaining < 0 {
					matches = 0
					break
				}
				if attribute == code {
					r.Method, _ = obj.getUtf8(method.NameIndex)
					matches++
				}
			}
		}
		if matches != 1 {
			r.Method = ""
		}
		if evidence := c.constructorProfileEvidence; evidence != nil {
			if hash, known := evidence.originals[r.Owner]; known {
				r.ClassHash = fmt.Sprintf("%x", hash)
			}
		}
		*c.constructorMotionRefusal = r
	}
	return false
}

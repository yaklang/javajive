package javaclassparser

import (
	"crypto/sha256"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

// An effect certificate can be independent of the runtime profile: it used
// only immutable caller-supplied class bytes and the already modeled bootstrap
// Object initialization/finalization facts. Other platform bodies or metadata
// keep the existing full per-profile analysis. The certificate is local to one
// movement request, so its arguments and moved-storage identities cannot drift.
type constructorProfileObservation struct {
	className, absentRootMethod, methodDescriptor string
}

type constructorProfileEvidence struct {
	originals                       map[string][32]byte
	transcript                      []constructorProfileObservation
	eligible, bootstrapUsed, inBody bool
	inconsistent                    bool
	rootName, rootSuper             string
	rootFlags                       uint16
	finalizerSilent                 bool
	observationEpoch, providerEpoch uint64
	parsedOriginals                 map[*ClassObject]bool
	closedBodies                    map[constructorClosedBodyKey]constructorClosedBodyMemo
	parsedRetention, bodyRetention  int64
}

// A local path proof can be reused without replay only while no external
// observation has intervened. Count attempts as well as successful reads:
// even a missing-class provider may change caller-owned metadata.
func (e *constructorProfileEvidence) observed() {
	e.observationEpoch++
	if e.observationEpoch == 0 {
		e.eligible, e.inconsistent = false, true
	}
}

// Root-absence queries are internal reads whose binding is checked on every
// call. Provider/metadata callbacks can also mutate parsed metadata borrowed
// from their original bytes, so body certificates have a separate epoch.
func (e *constructorProfileEvidence) providerObserved() {
	e.observed()
	e.providerEpoch++
	if e.providerEpoch == 0 {
		e.eligible, e.inconsistent = false, true
	}
}

func (e *constructorProfileEvidence) original(c *ClassObjectDumper, name string, raw []byte) {
	if e == nil {
		return
	}
	if !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.Charge(workbudget.CounterReadBytes, int64(len(raw))) != nil {
		e.eligible = false
		return
	}
	fingerprint := sha256.Sum256(raw)
	if e.inBody {
		if !e.record(c, constructorProfileObservation{className: name}) {
			return
		}
	}
	if previous, ok := e.originals[name]; ok {
		if previous != fingerprint {
			e.eligible = false
			e.inconsistent = true
		}
		return
	}
	// Every class comes from an already bounded field/constructor ancestry or
	// instruction visit. Retained evidence still has its own allocation guard.
	if len(e.originals) >= 512 || c.Work != nil && c.Work.CheckAlloc(int64((len(e.originals)+1)*96+len(name))) != nil {
		e.eligible = false
		return
	}
	e.originals[name] = fingerprint
}

// Preserve the relative order of resolver reads and root method-table queries.
// A provider can change either observation; a name/super/flags-only certificate
// cannot stand for a mutable caller method-table observation.
func (e *constructorProfileEvidence) record(c *ClassObjectDumper, observation constructorProfileObservation) bool {
	e.observed()
	if len(e.transcript) >= 512 || !nativeProofWork(c.Work, 1) || c.Work != nil && c.Work.CheckAlloc(int64((len(e.transcript)+1)*64+len(e.originals)*96+len(observation.className)+len(observation.absentRootMethod)+len(observation.methodDescriptor))) != nil {
		e.eligible = false
		return false
	}
	e.transcript = append(e.transcript, observation)
	return true
}

func (e *constructorProfileEvidence) metadata(provider callbinding.Provider) callbinding.Provider {
	return func(name string) (callbinding.Class, bool) {
		e.providerObserved()
		if name == "java/lang/Object" {
			e.bootstrapUsed = true
		} else {
			e.eligible = false
		}
		return provider(name)
	}
}

func (e *constructorProfileEvidence) revalidates(c *ClassObjectDumper, previous *constructorProfileEvidence, remaining *int) bool {
	if previous == nil || !previous.eligible || !previous.bootstrapUsed || !e.eligible || previous.rootName != e.rootName || previous.rootSuper != e.rootSuper || previous.rootFlags != e.rootFlags || previous.finalizerSilent != e.finalizerSilent {
		return false
	}
	count := len(previous.transcript)
	*remaining -= count + 1
	if *remaining < 0 || !nativeProofWork(c.Work, int64(count+1)) || c.Work != nil && c.Work.CheckAlloc(int64(count)*16) != nil {
		return false
	}
	// Replay the original provider observation order, including duplicate body
	// resolutions. The finalizer's earlier read cannot stand for a constructor
	// lookup that would observe different original bytes.
	e.inBody = true
	defer func() { e.inBody = false }()
	for _, observation := range previous.transcript {
		if observation.absentRootMethod != "" {
			if observation.className != "" || c.obj == nil || c.obj.GetClassName() != e.rootName || c.obj.GetSupperClassName() != e.rootSuper || c.obj.AccessFlags != e.rootFlags || !constructorReceiverRootMethodAbsent(c.obj, observation.absentRootMethod, observation.methodDescriptor, remaining, c.Work) || !e.record(c, observation) {
				e.inconsistent = true
				return false
			}
			continue
		}
		name := observation.className
		if _, known := c.foldSiblingResolver(name); !known {
			e.inconsistent = true
			return false
		}
		if fingerprint, known := e.originals[name]; !known || fingerprint != previous.originals[name] {
			e.inconsistent = true
			return false
		}
	}
	if !e.eligible {
		return false
	}
	exceptions, known := exactInvocationExceptions(c.FuncCtx.InvocationMetadata, "java/lang/Object", "<init>", "()V")
	return known && len(exceptions) == 0 && e.eligible
}

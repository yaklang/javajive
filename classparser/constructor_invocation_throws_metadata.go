package javaclassparser

import (
	"crypto/sha256"
	"encoding/json"
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// One inference uses one immutable view of each queried declaration. A later
// source consumer must re-prove against the same complete query transcript;
// a changing hierarchy cannot exchange a lower/upper/uncheckedness witness.
type constructorThrowsMetadata struct {
	c        *ClassObjectDumper
	provider callbinding.Provider
	classes  map[string]callbinding.Class
	hashes   map[string][32]byte
	retained int64
}

func (c *ClassObjectDumper) constructorThrowsProofContext() (*ClassObjectDumper, *constructorThrowsMetadata) {
	copy := *c
	ctx := *c.FuncCtx
	proof := &constructorThrowsMetadata{c: c, provider: ctx.InvocationMetadata, classes: map[string]callbinding.Class{}, hashes: map[string][32]byte{}}
	ctx.InvocationMetadata = proof.lookup
	copy.FuncCtx = &ctx
	return &copy, proof
}

func (p *constructorThrowsMetadata) lookup(name string) (callbinding.Class, bool) {
	if p == nil || p.provider == nil || !nativeProofWork(p.c.Work, 1) {
		return callbinding.Class{}, false
	}
	if result, present := p.classes[name]; present {
		return result, true
	}
	if len(p.classes) >= 64 || len(name) > 8192 {
		return callbinding.Class{}, false
	}
	original, known := p.provider(name)
	if !known || original.Name != name || len(original.Methods) > 256 || len(original.Parents) > 128 {
		return callbinding.Class{}, false
	}
	retained, entries := int64(0), int64(1)
	text := func(value string) bool {
		retained += int64(len(value))
		return len(value) <= 8192 && retained <= 65536 && nativeProofWork(p.c.Work, int64(len(value))+1)
	}
	if !text(original.Name) || !text(original.Signature) {
		return callbinding.Class{}, false
	}
	for _, parent := range original.Parents {
		entries++
		if !text(parent) {
			return callbinding.Class{}, false
		}
	}
	for _, method := range original.Methods {
		entries++
		if len(method.Exceptions) > 128 || !text(method.Name) || !text(method.Desc) || !text(method.Signature) {
			return callbinding.Class{}, false
		}
		for _, exception := range method.Exceptions {
			entries++
			if !text(exception) {
				return callbinding.Class{}, false
			}
		}
	}
	// JSON escaping is at most six bytes per source byte. Include conservative
	// field/list overhead and all retained immutable class copies before allocation.
	allocation := retained*8 + entries*256 + 4096
	if p.c.Work != nil && p.c.Work.CheckAlloc(p.retained+allocation) != nil {
		return callbinding.Class{}, false
	}
	copy := original
	copy.Parents = slices.Clone(original.Parents)
	copy.Methods = slices.Clone(original.Methods)
	for i := range copy.Methods {
		if !nativeProofWork(p.c.Work, 1) {
			return callbinding.Class{}, false
		}
		copy.Methods[i].Exceptions = slices.Clone(original.Methods[i].Exceptions)
	}
	encoded, err := json.Marshal(copy)
	if err != nil || !nativeProofWork(p.c.Work, int64(len(encoded))+1) {
		return callbinding.Class{}, false
	}
	p.retained += allocation
	p.classes[name] = copy
	p.hashes[name] = sha256.Sum256(encoded)
	return copy, true
}

func constructorThrowsMetadataEqual(a, b map[string][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, hash := range a {
		if other, known := b[name]; !known || other != hash {
			return false
		}
	}
	return true
}

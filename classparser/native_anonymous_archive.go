package javaclassparser

import (
	"encoding/json"
	"github.com/yaklang/javajive/internal/jdecenv"
	"strings"
	"sync"
)

type nativeAnonymousOutput struct {
	source string
	folded map[string]bool
}
type nativeAnonymousCacheEntry struct {
	once   sync.Once
	output nativeAnonymousOutput
}

func (z *JarFS) nativeAnonymousSource(cf *ClassObject) ([]byte, bool) {
	owner, _, anon := originalAnonymousOwner(cf)
	if !anon {
		owner = cf.GetClassName()
		candidate := false
		for _, a := range cf.Attributes {
			if inner, ok := a.(*InnerClassesAttribute); ok && inner != nil {
				for _, row := range inner.Classes {
					if row != nil && row.InnerNameIndex == 0 {
						candidate = true
					}
				}
			}
		}
		if !candidate {
			return nil, false
		}
	}
	if z.archive != nil && z.archive.budget != nil && z.archive.budget.Work().CheckAlloc(0) != nil {
		return nil, false
	}
	snap, bound := jdecenv.Current()
	if !bound || snap == nil {
		snap = snapshotJDECEnv()
	}
	policy, _ := json.Marshal(snap)
	key := owner + "\x00" + string(policy)
	// Bounds include failed proofs and policy variants. Never evict an accepted
	// family while this filesystem is live: its child suppression depends on it.
	z.nativeAnonymousMu.Lock()
	entry := z.nativeAnonymousCache[key]
	if entry == nil {
		if len(z.nativeAnonymousCache) >= 512 {
			z.nativeAnonymousMu.Unlock()
			return nil, false
		}
		if z.nativeAnonymousCache == nil {
			z.nativeAnonymousCache = map[string]*nativeAnonymousCacheEntry{}
		}
		entry = &nativeAnonymousCacheEntry{}
		z.nativeAnonymousCache[key] = entry
	}
	z.nativeAnonymousMu.Unlock()
	entry.once.Do(func() {
		object := cf
		if anon {
			raw, known := z.enumSiblingResolver()(owner)
			if !known {
				return
			}
			var e error
			reader := NewClassObjectDumper(cf)
			if z.archive != nil && z.archive.budget != nil {
				reader.Work = z.archive.budget.Work()
				reader.options.Context = z.archive.ctx
			}
			object, e = reader.parseResolved(raw)
			if e != nil || object.GetClassName() != owner {
				return
			}
		}
		d := NewClassObjectDumper(object)
		d.foldSiblingResolver = z.enumSiblingResolver()
		d.declarationResolver = z.declarationResolver
		if z.archive != nil && z.archive.budget != nil {
			d.Work = z.archive.budget.Work()
			d.options.Context = z.archive.ctx
			d.options.TargetRelease = z.archive.targetRelease
		}
		d.options.EnvSnapshot = snap
		p := d.planNativeAnonymousFamily()
		if p == nil {
			return
		}
		d.nativeAnonymousRoot = p
		var source string
		var err error
		jdecenv.Run(snap, func() error { source, err = d.DumpClass(); return err })
		if err != nil || !p.completeSource(source) {
			return
		}
		z.nativeAnonymousMu.Lock()
		if int64(len(source)) > (16<<20)-z.nativeAnonymousBytes {
			z.nativeAnonymousMu.Unlock()
			return
		}
		z.nativeAnonymousBytes += int64(len(source))
		z.nativeAnonymousMu.Unlock()
		entry.output.source = source
		entry.output.folded = map[string]bool{}
		for name := range p.children {
			entry.output.folded[name] = true
		}
	})
	out := entry.output
	if out.source == "" {
		return nil, false
	}
	if z.archive != nil && z.archive.budget != nil {
		work := z.archive.budget.Work()
		if work.CheckAlloc(int64(len(out.source))) != nil || work.CheckOutput(int64(len(out.source))) != nil {
			return nil, false
		}
	}
	if anon && out.folded[cf.GetClassName()] {
		return []byte("// " + strings.ReplaceAll(cf.GetClassName(), "/", ".") + ": original anonymous body owned by " + strings.ReplaceAll(owner, "/", ".") + "; javac regenerates its binary class\n"), true
	}
	if !anon {
		return []byte(out.source), true
	}
	return nil, false
}

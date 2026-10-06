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
	if anon {
		var known bool
		owner, known = z.nativeAnonymousOutermostOwner(owner, z.nativeMemberReader(cf))
		if !known {
			return nil, false
		}
	}

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
		d.nativeMemberLookup = z.nativeMemberLookup
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
		// A partial source transaction changes the prefix's type names while
		// leaving independent terminal binaries flat. Close physical archive
		// users before publishing either the prefix or its child suppression.
		if len(p.standalone) != 0 && !nativeAnonymousPrefixArchiveClosed(p, z.originalMemberIndex(), d.Work) {
			return
		}
		if p.forest != nil && !nativeAnonymousForestArchiveClosed(p.forest, z.originalMemberIndex(), d.Work) {
			return
		}
		d.nativeAnonymousRoot = p
		var source string
		var err error
		jdecenv.Run(snap, func() error { source, err = d.DumpClass(); return err })
		if err != nil || !p.completeSource(source) {
			return
		}
		if !z.reserveOwnershipSource(int64(len(source))) {
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
		if p.forest != nil {
			for name := range p.forest.units {
				entry.output.folded[name] = true
			}
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

// Parent ownership is an original EnclosingMethod/self-row fact. A cached
// anonymous leaf and its complete source root must share the same commit key.
func (z *JarFS) nativeAnonymousOutermostOwner(owner string, reader *ClassObjectDumper) (string, bool) {
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if seen[owner] || !nativeProofWork(reader.Work, 1) {
			return "", false
		}
		seen[owner] = true
		raw, known := z.enumSiblingResolver()(owner)
		if !known {
			return "", false
		}
		object, err := reader.parseResolved(raw)
		if err != nil || object.GetClassName() != owner {
			return "", false
		}
		next, _, anonymous := originalAnonymousOwner(object)
		if !anonymous {
			return owner, true
		}
		owner = next
	}
	return "", false
}

package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

// This graph is the JVM's reciprocal original nest, never a '$' name family.
// Only a closed, owned modern nest containing proved source access adapters
// participates in executable completion. Ordinary source-only use is unchanged.
type privateNestCompletionFamily struct {
	host    string
	names   []string
	objects map[string]*ClassObject
	raw     map[string][]byte
	plans   map[string]*privateNestPlan
}

type sourcePrivateNestOrigin struct {
	owner        string
	originalHash [32]byte
}

func (c *ClassObjectDumper) privateNestCompletionAuthority() *sourceConstructorLoweringAuthority {
	if c.options.sourceConstructorLowering != nil {
		return c.options.sourceConstructorLowering
	}
	// Native readers deliberately carry no flat constructor-lowering authority.
	// The explicit artifact can nevertheless record this separate source proof.
	if a := c.options.sourceLambdaMetadata; a != nil && a.jar != nil {
		return a.jar.sourceConstructorLowering
	}
	return nil
}

func (c *ClassObjectDumper) privateNestCompletionFamily(read func(string) ([]byte, error)) *privateNestCompletionFamily {
	if c.obj == nil || c.obj.MajorVersion < 55 || c.obj.MajorVersion > 65 || c.obj.MinorVersion != 0 || read == nil {
		return nil
	}
	f := &privateNestCompletionFamily{objects: map[string]*ClassObject{}, raw: map[string][]byte{}, plans: map[string]*privateNestPlan{}}
	total := int64(0)
	load := func(name string) (*ClassObject, bool) {
		if o := f.objects[name]; o != nil {
			return o, true
		}
		if len(f.objects) >= 129 || !nativeProofWork(c.Work, 1) {
			return nil, false
		}
		b, e := read(name + ".class")
		total += int64(len(b))
		if e != nil || total > 32<<20 || !nativeProofWork(c.Work, int64(len(b))+1) {
			return nil, false
		}
		o, e := ParseWithBudget(b, c.Work)
		if e != nil || o == nil || o.GetClassName() != name || o.MajorVersion < 55 || o.MajorVersion > 65 || o.MinorVersion != 0 {
			return nil, false
		}
		f.objects[name], f.raw[name] = o, b
		return o, true
	}
	owner := c.obj.GetClassName()
	hostName := owner
	if hasOriginalNestAttribute(c.obj, "NestHost") {
		hosts, ok := originalNestAttribute(c.obj, "NestHost")
		if !ok || len(hosts) != 1 || hosts[0] == owner {
			return nil
		}
		hostName = hosts[0]
	}
	host, ok := load(hostName)
	if !ok || hasOriginalNestAttribute(host, "NestHost") {
		return nil
	}
	members, ok := originalNestAttribute(host, "NestMembers")
	if !ok || len(members) == 0 {
		return nil
	}
	f.host = hostName
	f.names = append([]string{hostName}, members...)
	seen := map[string]bool{hostName: true}
	for _, name := range members {
		if seen[name] || nestPackage(name) != nestPackage(hostName) {
			return nil
		}
		seen[name] = true
		o, ok := load(name)
		if !ok || hasOriginalNestAttribute(o, "NestMembers") {
			return nil
		}
		hs, ok := originalNestAttribute(o, "NestHost")
		if !ok || len(hs) != 1 || hs[0] != hostName {
			return nil
		}
	}
	if !seen[owner] || !bytes.Equal(f.raw[owner], c.obj.Bytes()) {
		return nil
	}
	sort.Strings(f.names)
	bridges := 0
	for _, name := range f.names {
		p, e := c.buildPrivateNestPlan(f.objects[name], load)
		if e != nil {
			return nil
		}
		if p == nil || len(p.bridges) == 0 {
			continue
		}
		for site := range p.sites {
			if site.kind != uint8(values.InvokeVirtual) {
				return nil
			}
		}
		f.plans[name] = p
		bridges += len(p.bridges)
	}
	if bridges == 0 || bridges > 512 {
		return nil
	}
	return f
}

func (c *ClassObjectDumper) publishPrivateNestCompletion(source string, methods []*dumpedMethods) bool {
	a := c.privateNestCompletionAuthority()
	if a == nil || a.jar == nil || a.jar.targetSourceVersion < 11 || c.obj.MajorVersion < 55 || !hasOriginalNestAttribute(c.obj, "NestHost") && !hasOriginalNestAttribute(c.obj, "NestMembers") {
		return true
	}
	family := c.privateNestCompletionFamily(a.jar.readJarEntry)
	if family == nil {
		return c.Work.Check() == nil
	}
	origin := constructorNativeSourceOrigin{}
	if !c.sourceDegraded && !legacySourceHasFailureMarker(source) {
		plan := family.plans[c.obj.GetClassName()]
		count := 0
		for _, m := range methods {
			if m == nil || m.privateNestTarget == nil {
				continue
			}
			key := *m.privateNestTarget
			if plan == nil || plan.bridges[key] != m.methodName || m.descriptor != key.descriptor || m.code == "" || m.code != m.privateNestCode || m.isStub || strings.Count(source, m.code) != 1 {
				return false
			}
			count++
		}
		if plan != nil && count != len(plan.bridges) {
			return false
		}
		raw := family.raw[c.obj.GetClassName()]
		if !nativeProofWork(c.Work, int64(len(source)+len(raw))+1) {
			return false
		}
		origin = constructorNativeSourceOrigin{sha256.Sum256(raw), sha256.Sum256([]byte(source))}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Work.CheckAlloc(int64(len(a.nestPending)+1)*96) != nil {
		return false
	}
	if a.nestPending == nil {
		a.nestPending = map[string]constructorNativeSourceOrigin{}
	}
	a.nestPending[c.obj.GetClassName()] = origin
	return true
}

func (a *sourceConstructorLoweringAuthority) privateNestSourceReturned(owner string, original []byte, work *workbudget.Budget) bool {
	if a == nil || !nativeProofWork(work, 1) {
		return false
	}
	a.mu.Lock()
	origin, active := a.nestActive[owner]
	_, pending := a.nestPending[owner]
	if !active {
		origin, active = a.nestNativeActive[owner]
	}
	a.mu.Unlock()
	if !active && !pending && a.parent != nil && a.jar != nil && a.parent.jar != nil && a.jar.sourceCompiler.effective() == a.parent.jar.sourceCompiler.effective() && a.jar.targetSourceVersion == a.parent.jar.targetSourceVersion {
		a.parent.mu.Lock()
		origin, active = a.parent.nestActive[owner]
		if !active {
			origin, active = a.parent.nestNativeActive[owner]
		}
		a.parent.mu.Unlock()
	}
	return active && origin.sourceHash != ([32]byte{}) && origin.originalHash == sha256.Sum256(original)
}

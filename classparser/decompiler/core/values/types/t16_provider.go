package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ClassHierarchyIdentity is content-addressed class metadata. It is built from
// class bytes (never Class.forName / <clinit>).
type ClassHierarchyIdentity struct {
	BinaryName  string
	SuperClass  string
	Interfaces  []string
	ContentHash string
}

// MetadataResolver returns hierarchy identity for a JVM internal name.
// ok=false is a resolver miss, not a guess.
type MetadataResolver func(internalName string) (ClassHierarchyIdentity, bool)

type hierarchyCacheEntry struct {
	ident   ClassHierarchyIdentity
	ok      bool
	unknown bool
	reason  string
}

// MetadataHierarchyProvider is a per-classpath, content-addressed super-type
// provider. Negative cache entries are not shared across provider instances.
type MetadataHierarchyProvider struct {
	resolver        MetadataResolver
	classpathDigest string
	targetVersion   string
	optionsKey      string
	cancel          <-chan struct{}
	cache           map[string]*hierarchyCacheEntry
	walkCap         int
}

// NewMetadataHierarchyProvider constructs an isolated provider. cache keys
// include classpath content digest, class identity, target version and options.
func NewMetadataHierarchyProvider(resolver MetadataResolver, classpathDigest, targetVersion, optionsKey string, cancel <-chan struct{}) *MetadataHierarchyProvider {
	if classpathDigest == "" {
		classpathDigest = "empty"
	}
	return &MetadataHierarchyProvider{
		resolver:        resolver,
		classpathDigest: classpathDigest,
		targetVersion:   targetVersion,
		optionsKey:      optionsKey,
		cancel:          cancel,
		cache:           map[string]*hierarchyCacheEntry{},
		walkCap:         crossClassSubtypeWalkCap,
	}
}

func ClasspathDigest(blobs ...[]byte) string {
	h := sha256.New()
	for _, b := range blobs {
		sum := sha256.Sum256(b)
		h.Write(sum[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (p *MetadataHierarchyProvider) cacheKey(internal string, ident ClassHierarchyIdentity, miss bool) string {
	id := ident.ContentHash
	if miss || id == "" {
		id = "MISS"
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s", p.classpathDigest, ident.BinaryName+internal, id, p.targetVersion, p.optionsKey)
}

func (p *MetadataHierarchyProvider) cancelled() bool {
	if p == nil || p.cancel == nil {
		return false
	}
	select {
	case <-p.cancel:
		return true
	default:
		return false
	}
}

// Lookup returns identity metadata. A miss, cycle, or cancel is explicit unknown.
func (p *MetadataHierarchyProvider) Lookup(internalName string) (ClassHierarchyIdentity, bool, string) {
	if p == nil {
		return ClassHierarchyIdentity{}, false, "no provider"
	}
	internalName = strings.ReplaceAll(internalName, ".", "/")
	if p.cancelled() {
		return ClassHierarchyIdentity{}, false, "cancel"
	}
	if p.resolver == nil {
		return ClassHierarchyIdentity{}, false, "resolver miss"
	}
	ident, ok := p.resolver(internalName)
	if !ok {
		key := p.cacheKey(internalName, ClassHierarchyIdentity{BinaryName: internalName}, true)
		p.cache[key] = &hierarchyCacheEntry{ok: false, unknown: true, reason: "resolver miss"}
		return ClassHierarchyIdentity{BinaryName: internalName}, false, "resolver miss"
	}
	if ident.BinaryName == "" {
		ident.BinaryName = internalName
	}
	key := p.cacheKey(internalName, ident, false)
	if e, hit := p.cache[key]; hit {
		return e.ident, e.ok && !e.unknown, e.reason
	}
	e := &hierarchyCacheEntry{ident: ident, ok: true}
	p.cache[key] = e
	return ident, true, ""
}

// AsSuperTypeProvider adapts to ClassContext.SiblingSuperTypes.
func (p *MetadataHierarchyProvider) AsSuperTypeProvider() SuperTypeProvider {
	if p == nil {
		return nil
	}
	return func(internalName string) ([]string, bool) {
		ident, ok, reason := p.Lookup(internalName)
		if !ok {
			_ = reason
			return nil, false
		}
		var supers []string
		if ident.SuperClass != "" {
			supers = append(supers, ident.SuperClass)
		}
		supers = append(supers, ident.Interfaces...)
		return supers, true
	}
}

// HierarchyWalk is a bounded, cancellable super-type walk.
type HierarchyWalk struct {
	Ancestors map[string]int
	Unknown   bool
	Cyclic    bool
	Cancelled bool
	Reason    string
}

// Walk starts at a binary/internal name and never executes target <clinit>.
func (p *MetadataHierarchyProvider) Walk(start string) HierarchyWalk {
	out := HierarchyWalk{Ancestors: map[string]int{}}
	if p == nil {
		out.Unknown = true
		out.Reason = "no provider"
		return out
	}
	if p.cancelled() {
		out.Unknown = true
		out.Cancelled = true
		out.Reason = "cancel"
		return out
	}
	startI := strings.ReplaceAll(start, ".", "/")
	onPath := map[string]bool{}
	visited := map[string]bool{}
	capN := p.walkCap
	if capN <= 0 {
		capN = crossClassSubtypeWalkCap
	}
	var walk func(cur string, depth int) bool
	walk = func(cur string, depth int) bool {
		if p.cancelled() {
			out.Unknown = true
			out.Cancelled = true
			out.Reason = "cancel"
			return false
		}
		if cur == "" {
			return true
		}
		if onPath[cur] {
			out.Unknown = true
			out.Cyclic = true
			out.Reason = "cyclic parents"
			return false
		}
		if visited[cur] {
			return true
		}
		if len(visited) >= capN {
			out.Unknown = true
			out.Reason = "walk cap"
			return false
		}
		visited[cur] = true
		if _, seen := out.Ancestors[cur]; !seen {
			out.Ancestors[internalToDot(cur)] = depth
		}
		if cur == "java/lang/Object" {
			return true
		}
		if _, jdk := jdkSuperEdges[internalToDot(cur)]; jdk {
			for _, s := range jdkSuperEdges[internalToDot(cur)] {
				if !walk(dotToInternal(s), depth+1) {
					return false
				}
			}
			return true
		}
		ident, ok, reason := p.Lookup(cur)
		if !ok {
			out.Unknown = true
			out.Reason = reason
			return false
		}
		onPath[cur] = true
		if ident.SuperClass != "" {
			if !walk(strings.ReplaceAll(ident.SuperClass, ".", "/"), depth+1) {
				onPath[cur] = false
				return false
			}
		}
		for _, iface := range ident.Interfaces {
			if iface == "" {
				continue
			}
			if !walk(strings.ReplaceAll(iface, ".", "/"), depth+1) {
				onPath[cur] = false
				return false
			}
		}
		onPath[cur] = false
		return true
	}
	walk(startI, 0)
	return out
}

// JoinNames joins two binary names using this provider. Unknown is explicit.
func (p *MetadataHierarchyProvider) JoinNames(a, b string) LubResult {
	if p == nil {
		return LubResult{Source: UnknownType(), Unknown: true, Reason: "no provider"}
	}
	wa, wb := p.Walk(a), p.Walk(b)
	if wa.Unknown {
		return LubResult{Source: UnknownType(), Unknown: true, Reason: wa.Reason}
	}
	if wb.Unknown {
		return LubResult{Source: UnknownType(), Unknown: true, Reason: wb.Reason}
	}
	return JoinTypes(NewJavaClass(internalToDot(strings.ReplaceAll(a, "/", "."))), NewJavaClass(internalToDot(strings.ReplaceAll(b, "/", "."))), p.AsSuperTypeProvider())
}

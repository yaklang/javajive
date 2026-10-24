package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

const jarManifestByteLimit = 1 << 20
const jarManifestEntryLimit = 65535
const jarCatalogNameByteLimit = 16 << 20

type jarReleaseRole uint8

const (
	jarReleaseRuntime jarReleaseRole = iota
	jarReleasePhysicalSource
)

// A receipt belongs to one original archive, independent of requested release
// or source role. Its catalog indexes exact original ZIP entries; a path-tree
// alias cannot supply metadata or declarations for a different physical name.
type jarReleaseVariant struct {
	release  int
	physical string
}

type jarReleaseSelection struct {
	request     *archiveFSState
	root        *filesys.ZipFS
	legacyWork  *workbudget.Budget
	catalogOnce sync.Once
	entries     []string
	exact       map[string]int
	versions    map[string][]jarReleaseVariant
	catalogErr  error
	once        sync.Once
	multi       bool
	err         error
}

func newJarReleaseSelection(root *filesys.ZipFS, request *archiveFSState) *jarReleaseSelection {
	p := &jarReleaseSelection{root: root, request: request}
	if root != nil && root.ArchiveBudget() == nil && (request == nil || request.budget == nil) {
		// Legacy unbounded callers still own one metered selection request.
		// Its physical reads and physical source views use this same Work;
		// never create a fresh allowance for each ZIP entry or release.
		p.legacyWork = workbudget.New(root.ArchiveContext(), workbudget.Limits{})
	}
	return p
}

func (p *jarReleaseSelection) work(n int64) error {
	if p == nil || p.root == nil {
		return fmt.Errorf("unknown JAR release selection archive")
	}
	rootBudget := p.root.ArchiveBudget()
	if ctx := p.root.ArchiveContext(); ctx != nil {
		if rootBudget != nil && rootBudget.Work() != nil {
			if err := rootBudget.Work().CheckContext(ctx); err != nil {
				return err
			}
		} else if err := ctx.Err(); err != nil {
			return err
		}
	}
	if rootBudget != nil && rootBudget.Work() != nil {
		if err := rootBudget.Work().Charge(workbudget.CounterGraphScans, n); err != nil {
			return err
		}
	}
	if p.request != nil {
		if err := p.request.checkJarSelectionContext(); err != nil {
			return err
		}
		if budget := p.request.budget; budget != nil && budget.Work() != nil && (rootBudget == nil || budget.Work() != rootBudget.Work()) {
			return budget.Work().Charge(workbudget.CounterGraphScans, n)
		}
	}
	if p.legacyWork != nil {
		return p.legacyWork.Charge(workbudget.CounterGraphScans, n)
	}
	return nil
}

func (p *jarReleaseSelection) checkAlloc(n int64) error {
	rootBudget := p.root.ArchiveBudget()
	if rootBudget != nil && rootBudget.Work() != nil {
		if err := rootBudget.Work().CheckAlloc(n); err != nil {
			return err
		}
	}
	if p.request != nil && p.request.budget != nil && p.request.budget.Work() != nil && (rootBudget == nil || rootBudget.Work() != p.request.budget.Work()) {
		return p.request.budget.Work().CheckAlloc(n)
	}
	if p.legacyWork != nil {
		return p.legacyWork.CheckAlloc(n)
	}
	return nil
}

func (s *archiveFSState) checkJarSelectionContext() error {
	if s == nil {
		return nil
	}
	if s.budget != nil && s.budget.Work() != nil {
		return s.budget.Work().CheckContext(s.ctx)
	}
	if s.ctx != nil {
		return s.ctx.Err()
	}
	return nil
}
func (s *archiveFSState) checkJarSelectionWork() error {
	if err := s.checkJarSelectionContext(); err != nil {
		return err
	}
	if s != nil && s.budget != nil && s.budget.Work() != nil {
		return s.budget.Work().Charge(workbudget.CounterGraphScans, 1)
	}
	return nil
}

func (p *jarReleaseSelection) catalog() ([]string, error) {
	if err := p.work(1); err != nil {
		return nil, err
	}
	p.catalogOnce.Do(func() {
		count := p.root.EntryCount()
		if count > jarManifestEntryLimit {
			p.catalogErr = fmt.Errorf("unknown JAR release selection: archive entry bound")
			return
		}
		if err := p.work(int64(count)); err != nil {
			p.catalogErr = err
			return
		}
		if err := p.checkAlloc(int64(count) * 192); err != nil {
			p.catalogErr = err
			return
		}
		entries := p.root.EntryNames()
		exact := make(map[string]int, len(entries))
		versions := make(map[string][]jarReleaseVariant)
		total := 0
		for i, name := range entries {
			if len(name) > jarCatalogNameByteLimit-total {
				p.catalogErr = fmt.Errorf("unknown JAR release selection: archive name bound")
				return
			}
			total += len(name)
			if err := p.work(int64(len(name)) + 1); err != nil {
				p.catalogErr = err
				return
			}
			if _, duplicate := exact[name]; duplicate {
				exact[name] = -1
			} else {
				exact[name] = i
			}
			rest, versioned := strings.CutPrefix(name, "META-INF/versions/")
			if versioned && jarCanonicalEntry(name) {
				part, suffix, found := strings.Cut(rest, "/")
				version, parseErr := strconv.Atoi(part)
				if found && parseErr == nil && version >= 9 && strconv.Itoa(version) == part && suffix != "" && !strings.HasPrefix(suffix, "META-INF/") {
					versions[suffix] = append(versions[suffix], jarReleaseVariant{release: version, physical: name})
				}
			}
		}
		for _, variants := range versions {
			// At most count entries, with bounded comparison work charged before sort.
			levels := int64(1)
			for n := len(variants); n > 1; n = (n + 1) / 2 {
				levels++
			}
			if err := p.work(int64(len(variants)) * levels); err != nil {
				p.catalogErr = err
				return
			}
			sort.Slice(variants, func(i, j int) bool { return variants[i].release < variants[j].release })
		}
		p.entries, p.exact, p.versions = entries, exact, versions
	})
	if err := p.work(1); err != nil {
		return nil, err
	}
	return p.entries, p.catalogErr
}

func (p *jarReleaseSelection) readPhysical(name string, hardLimit int64) ([]byte, error) {
	if _, err := p.catalog(); err != nil {
		return nil, err
	}
	if err := p.work(int64(len(name)) + 1); err != nil {
		return nil, err
	}
	index, found := p.exact[name]
	if !found {
		return nil, os.ErrNotExist
	}
	if index < 0 {
		return nil, fmt.Errorf("unknown JAR physical entry: duplicate name")
	}
	if p.request != nil {
		opts := filesys.ArchiveOptions{Limits: p.request.limits, Budget: p.request.budget, Context: p.request.ctx}
		if opts.Budget == nil && p.legacyWork != nil {
			opts.Work = p.legacyWork
		}
		return p.root.ReadPhysicalEntry(index, name, hardLimit, opts)
	}
	if p.legacyWork != nil {
		return p.root.ReadPhysicalEntry(index, name, hardLimit, filesys.ArchiveOptions{Work: p.legacyWork})
	}
	return p.root.ReadPhysicalEntry(index, name, hardLimit)
}

func (p *jarReleaseSelection) runtimeMultiRelease() (bool, error) {
	if err := p.work(1); err != nil {
		return false, err
	}
	p.once.Do(func() {
		entries, err := p.catalog()
		if err != nil {
			p.err = err
			return
		}
		if err := p.work(int64(len(entries))); err != nil {
			p.err = err
			return
		}
		manifest := ""
		for _, name := range entries {
			if !jarASCIIEqual(name, "META-INF/MANIFEST.MF") {
				continue
			}
			if manifest != "" {
				p.err = fmt.Errorf("unknown JAR release selection: duplicate physical manifests")
				return
			}
			manifest = name
		}
		if manifest == "" {
			return
		}
		raw, err := p.readPhysical(manifest, jarManifestByteLimit)
		if err != nil {
			p.err = err
			return
		}
		if err = p.work(int64(len(raw))*2 + 1); err != nil {
			p.err = err
			return
		}
		p.multi, p.err = jarMainMultiRelease(raw)
	})
	if err := p.work(1); err != nil {
		return false, err
	}
	return p.multi, p.err
}

// RuntimeMultiRelease reports only the original physical root's MR metadata.
// It does not select a runtime version or confer source ownership. Unknown is
// an error, distinct from an original archive that is known not to be MR.
func (z *JarFS) RuntimeMultiRelease() (bool, error) {
	p, err := z.boundJarReleaseSelection()
	if err != nil {
		return false, err
	}
	return p.runtimeMultiRelease()
}

func (z *JarFS) boundJarReleaseSelection() (*jarReleaseSelection, error) {
	if z == nil {
		return nil, fmt.Errorf("unknown JAR release selection archive")
	}
	if err := z.archive.checkJarSelectionWork(); err != nil {
		return nil, err
	}
	return z.bindJarReleaseSelection()
}

// Binding alone creates no metadata/read permission. Namespace construction
// may retain the binding while canceled; every actual read checks current Work.
func (z *JarFS) bindJarReleaseSelection() (*jarReleaseSelection, error) {
	if z == nil || z.ZipFS == nil {
		return nil, fmt.Errorf("unknown JAR release selection archive")
	}
	z.jarReleaseMu.Lock()
	defer z.jarReleaseMu.Unlock()
	if z.jarReleaseSelection == nil {
		z.jarReleaseBinding = z.ZipFS
		z.jarReleaseSelection = newJarReleaseSelection(z.ZipFS, z.archive)
	}
	if z.jarReleaseBinding != z.ZipFS {
		return nil, fmt.Errorf("JAR release selection archive changed")
	}
	return z.jarReleaseSelection, nil
}

func (z *JarFS) effectiveJarRelease() (int, error) {
	p, err := z.boundJarReleaseSelection()
	if err != nil {
		return 0, err
	}
	if z.jarReleaseRole == jarReleasePhysicalSource {
		if z.jarPhysicalRelease < 9 || z.ZipFS.TargetRelease() != z.jarPhysicalRelease {
			return 0, fmt.Errorf("unknown physical source namespace")
		}
		if err := p.work(1); err != nil {
			return 0, err
		}
		return z.jarPhysicalRelease, nil
	}
	if z.jarReleaseRole != jarReleaseRuntime {
		return 0, fmt.Errorf("unknown JAR release role")
	}
	target := z.ZipFS.TargetRelease()
	if z.archive != nil {
		target = z.archive.targetRelease
	}
	return p.effectiveRelease(target, jarReleaseRuntime)
}

func (p *jarReleaseSelection) effectiveRelease(target int, role jarReleaseRole) (int, error) {
	if err := p.work(1); err != nil {
		return 0, err
	}
	if role == jarReleasePhysicalSource {
		if target < 9 {
			return 0, fmt.Errorf("unknown physical source namespace")
		}
		return target, nil
	}
	if role != jarReleaseRuntime {
		return 0, fmt.Errorf("unknown JAR release role")
	}
	if target < 9 {
		return 0, nil
	}
	multi, err := p.runtimeMultiRelease()
	if err != nil {
		return 0, err
	}
	if !multi {
		return 0, nil
	}
	return target, nil
}

func jarLogicalPath(name string) string { return strings.TrimPrefix(name, "./") }

func (p *jarReleaseSelection) selectPath(name string, target int, role jarReleaseRole) (string, error) {
	if err := p.work(1); err != nil {
		return "", err
	}
	name = jarLogicalPath(name)
	// Explicit physical requests never borrow runtime metadata permission.
	if strings.HasPrefix(name, "META-INF/") {
		return name, nil
	}
	release, err := p.effectiveRelease(target, role)
	if err != nil {
		return "", err
	}
	// The JVM never versions logical resources under META-INF.
	if release < 9 || strings.HasPrefix(name, "META-INF/") {
		return name, nil
	}
	if _, err := p.catalog(); err != nil {
		return "", err
	}
	if err := p.work(int64(len(name)) + 1); err != nil {
		return "", err
	}
	variants := p.versions[name]
	lo, hi := 0, len(variants)
	for lo < hi {
		if err := p.work(1); err != nil {
			return "", err
		}
		mid := lo + (hi-lo)/2
		if variants[mid].release <= release {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	chosen := name
	if lo > 0 {
		chosen = variants[lo-1].physical
	}
	return chosen, nil
}

func (z *JarFS) readJarEntry(name string) ([]byte, error) {
	p, err := z.boundJarReleaseSelection()
	if err != nil {
		return nil, err
	}
	name = jarLogicalPath(name)
	if strings.HasPrefix(name, "META-INF/") {
		return p.readPhysical(name, 0)
	}
	target, err := z.effectiveJarRelease()
	if err != nil {
		return nil, err
	}
	chosen, err := p.selectPath(name, target, z.jarReleaseRole)
	if err != nil {
		return nil, err
	}
	return p.readPhysical(chosen, 0)
}

func (e *ExpandedZipFS) boundJarReleaseSelection() (*jarReleaseSelection, error) {
	if e == nil || e.zipFS == nil {
		return nil, fmt.Errorf("unknown JAR release selection archive")
	}
	if err := e.archive.checkJarSelectionWork(); err != nil {
		return nil, err
	}
	e.jarReleaseMu.Lock()
	defer e.jarReleaseMu.Unlock()
	if e.jarReleaseSelection == nil {
		e.jarReleaseBinding = e.zipFS
		e.jarReleaseSelection = newJarReleaseSelection(e.zipFS, e.archive)
	}
	if e.jarReleaseBinding != e.zipFS {
		return nil, fmt.Errorf("JAR release selection archive changed")
	}
	return e.jarReleaseSelection, nil
}

func (e *ExpandedZipFS) readSelectedJarEntry(name string) ([]byte, error) {
	p, err := e.boundJarReleaseSelection()
	if err != nil {
		return nil, err
	}
	return p.readPhysical(name, 0)
}

// A nested physical entry is selected by its outer archive's semantic role.
// Errors never fall through to the raw force view or another namespace.
func (e *ExpandedZipFS) readSemanticArchiveBytes(name string) ([]byte, error) {
	if isArchivePath(name) {
		return e.readFileFromArchive(name)
	}
	if e.zipFS == nil {
		return e.readArchiveBytes(name)
	}
	return e.ReadFile(name)
}

func jarCanonicalEntry(name string) bool {
	return name != "" && !strings.Contains(name, "\\") && path.Clean(name) == name && !strings.HasPrefix(name, "/")
}

func jarASCIIFold(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
func jarASCIIEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if jarASCIIFold(a[i]) != jarASCIIFold(b[i]) {
			return false
		}
	}
	return true
}

// jarManifestLine reproduces the JDK detector's bounded FastInputStream:
// 512-byte physical lines over an 8192-byte ByteArrayInputStream buffer.
// EOF discards an unterminated partial line. At a buffer boundary, a trailing
// CR may consume the next LF even when the caller's line buffer is full.
// These are byte-stream rules, independent of attribute names and values.
func jarManifestLine(raw []byte, offset *int, storage *[512]byte) ([]byte, bool) {
	total := 0
	for total < len(storage) {
		if *offset >= len(raw) {
			return nil, false
		}
		bufferEnd := (*offset/8192 + 1) * 8192
		if bufferEnd > len(raw) {
			bufferEnd = len(raw)
		}
		end := *offset + len(storage) - total
		if end > bufferEnd {
			end = bufferEnd
		}
		start := *offset
		cursor := start
		for cursor < end {
			c := raw[cursor]
			cursor++
			if c == '\n' || c == '\r' {
				break
			}
		}
		if raw[cursor-1] == '\r' && cursor < end && raw[cursor] == '\n' {
			cursor++
		}
		n := copy(storage[total:], raw[start:cursor])
		total += n
		*offset = cursor
		switch storage[total-1] {
		case '\n':
			return storage[:total], true
		case '\r':
			if cursor == bufferEnd && cursor < len(raw) && raw[cursor] == '\n' {
				if total < len(storage) {
					storage[total] = '\n'
					total++
				} else {
					storage[total-1] = '\n'
				}
				*offset++
			}
			return storage[:total], true
		}
	}
	return storage[:total], true
}

// JDK21 first searches the original bytes for a contiguous, case-insensitive
// marker. Only then does it parse the main attributes. Continuations commit
// together, duplicate main attributes use the last completed value, and an
// entry section can supply the raw witness but cannot authorize the JAR.
// Malformed main attributes and duplicate physical manifests remain explicit
// unknowns; they never silently authorize a version or fall back to the base.
func jarMainMultiRelease(raw []byte) (bool, error) {
	if len(raw) > jarManifestByteLimit {
		return false, fmt.Errorf("unknown JAR manifest byte bound")
	}
	witness := []byte("multi-release: true")
	hasWitness := false
	for i := 0; i+len(witness) <= len(raw); i++ {
		match := true
		for j, c := range witness {
			if jarASCIIFold(raw[i+j]) != c {
				match = false
				break
			}
		}
		if match {
			hasWitness = true
			break
		}
	}
	if !hasWitness {
		return false, nil
	}
	lastMR, hasName, currentMR, validName := false, false, false, false
	var value []byte
	finish := func() error {
		if !validName {
			return fmt.Errorf("unknown JAR manifest attribute name")
		}
		if currentMR {
			lastMR = jarASCIIEqual(string(value), "true")
		}
		return nil
	}
	var storage [512]byte
	for offset := 0; offset < len(raw); {
		physical, ok := jarManifestLine(raw, &offset, &storage)
		if !ok {
			break
		}
		n := len(physical) - 1
		if physical[n] != '\n' && physical[n] != '\r' {
			return false, fmt.Errorf("unknown JAR manifest physical line bound")
		}
		if n > 0 && physical[n-1] == '\r' {
			n--
		}
		line := physical[:n]
		if len(line) == 0 {
			break
		}
		if line[0] == ' ' {
			if !hasName {
				return false, fmt.Errorf("unknown JAR manifest misplaced continuation")
			}
			if currentMR {
				value = append(value, line[1:]...)
			}
		} else {
			colon := bytes.IndexByte(line, ':')
			if colon < 1 || colon+1 >= len(line) || line[colon+1] != ' ' {
				return false, fmt.Errorf("unknown JAR manifest header")
			}
			hasName = true
			validName = colon <= 70
			for _, c := range line[:colon] {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
					validName = false
				}
			}
			currentMR = jarASCIIEqual(string(line[:colon]), "Multi-Release")
			value = value[:0]
			if currentMR {
				value = append(value, line[colon+2:]...)
			}
		}
		// Like Attributes.read, postpone committing the entire attribute while
		// the next physical line starts with a continuation. An EOF-discarded
		// continuation must not publish a partially assembled value.
		if offset < len(raw) && raw[offset] == ' ' {
			continue
		}
		if err := finish(); err != nil {
			return false, err
		}
	}
	return lastMR, nil
}

package javaclassparser

import (
	"bytes"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/javajive/internal/filesys"
	fi "github.com/yaklang/javajive/internal/filesys/filesys_interface"
)

// Declaration indexes contain entry locations, not parsed classes or decompiled
// bodies. Each outer WAR owns a separate namespace. A conflicting duplicate is
// unknown rather than depending on traversal order.
type archiveDeclarationLocation struct {
	fs   fi.FileSystem
	name string
}

type archiveDeclarationScope struct {
	once    sync.Once
	entries map[string][]archiveDeclarationLocation
	valid   bool
}

func (s *archiveDeclarationScope) index(root fi.FileSystem, rootDir string) {
	s.entries = make(map[string][]archiveDeclarationLocation)
	var walk func(fi.FileSystem, string, int) bool
	count := 0
	walk = func(fsys fi.FileSystem, dir string, depth int) bool {
		if depth > 64 {
			return false
		}
		entries, err := fsys.ReadDir(dir)
		if err != nil {
			return false
		}
		// Stable traversal is useful for reproducible diagnostics, even though
		// conflicts are never resolved by selecting the first entry.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			name := fsys.Join(dir, entry.Name())
			if entry.IsDir() {
				if !walk(fsys, name, depth+1) {
					return false
				}
				continue
			}
			if strings.EqualFold(path.Ext(name), ".jar") {
				var z *filesys.ZipFS
				if parent, ok := fsys.(*filesys.ZipFS); ok {
					// Use the same nesting/budget admission as normal archive reads.
					view, err := NewJarFS(parent).getNestedJarFS(name)
					if err != nil {
						return false
					}
					z = view.GetFileSystem().(*JarFS).ZipFS
				} else {
					raw, err := fsys.ReadFile(name)
					if err != nil {
						return false
					}
					var errOpen error
					z, errOpen = filesys.NewZipFSRaw(bytes.NewReader(raw), int64(len(raw)))
					if errOpen != nil {
						return false
					}
				}
				if !walk(z, ".", depth+1) {
					return false
				}
				continue
			}
			// Do not enter adjacent WAR/EAR/ZIP applications. Nested JARs are
			// declarations in this scope; other deployments are not.
			if !strings.HasSuffix(name, ".class") {
				continue
			}
			logical := strings.TrimPrefix(path.Clean(name), "./")
			if fsys == root && path.Clean(rootDir) != "." {
				logical = strings.TrimPrefix(logical, strings.TrimPrefix(path.Clean(rootDir), "./")+"/")
			}
			logical = strings.TrimPrefix(logical, "WEB-INF/classes/")
			logical = strings.TrimPrefix(logical, "BOOT-INF/classes/")
			logical = strings.TrimPrefix(logical, "classes/")
			if strings.HasPrefix(logical, "META-INF/") {
				continue
			}
			logical = strings.TrimSuffix(logical, ".class")
			count++
			if count > 262144 {
				return false
			}
			s.entries[logical] = append(s.entries[logical], archiveDeclarationLocation{fsys, name})
		}
		return true
	}
	s.valid = walk(root, rootDir, 0)
	if !s.valid {
		s.entries = nil
	}
}

func (s *archiveDeclarationScope) resolve(root fi.FileSystem, rootDir, name string) ([]byte, bool) {
	s.once.Do(func() { s.index(root, rootDir) })
	if !s.valid {
		return nil, false
	}
	entries := s.entries[name]
	var result []byte
	for _, entry := range entries {
		raw, err := entry.fs.ReadFile(entry.name)
		if err != nil {
			return nil, false
		}
		if result == nil {
			result = raw
			continue
		}
		if !bytes.Equal(result, raw) {
			return nil, false
		}
	}
	return result, result != nil
}

// declarationScopeRoot selects the deployment container before considering a
// nested lib JAR. For loose libraries it uses their directory, not the process
// working directory. This prevents classpath leakage between independent WARs.
func (e *ExpandedZipFS) declarationScopeRoot(jarPath string) (string, fi.FileSystem, string) {
	name := normalizeArchivePath(jarPath)
	lower := strings.ToLower(name)
	for i := 0; i < len(lower); i++ {
		for _, ext := range []string{".war/", ".war!", ".ear/", ".ear!"} {
			if strings.HasPrefix(lower[i:], ext) {
				container := name[:i+len(ext)-1]
				z, err := e.getZipFS(container)
				if err != nil {
					return "", nil, ""
				}
				return container, z, "."
			}
		}
	}
	// The archive itself can be a deployment container.
	if strings.HasSuffix(lower, ".war") || strings.HasSuffix(lower, ".ear") {
		z, err := e.getZipFS(name)
		if err != nil {
			return "", nil, ""
		}
		return name, z, "."
	}
	if e.zipFS != nil {
		return "<archive-root>", e.zipFS, "."
	}
	// Keep loose lookup inside this library directory and its classes sibling.
	dir := path.Dir(name)
	if path.Base(dir) == "lib" {
		dir = path.Dir(dir)
	}
	return "directory:" + dir, e.underlying, dir
}

func (e *ExpandedZipFS) archiveDeclarationResolver(jarPath string) func(string) ([]byte, bool) {
	return func(name string) ([]byte, bool) {
		key, root, dir := e.declarationScopeRoot(jarPath)
		if root == nil {
			return nil, false
		}
		return e.resolveDeclarationScope(key, root, dir, name)
	}
}

func (e *ExpandedZipFS) resolveDeclarationScope(key string, root fi.FileSystem, dir, name string) ([]byte, bool) {
	e.declarationsMu.Lock()
	if e.declarations == nil {
		e.declarations = map[string]*archiveDeclarationScope{}
	}
	scope := e.declarations[key]
	if scope == nil {
		scope = &archiveDeclarationScope{}
		e.declarations[key] = scope
	}
	e.declarationsMu.Unlock()
	return scope.resolve(root, dir, strings.ReplaceAll(name, ".", "/"))
}

var _ fs.ReadDirFS = (*filesys.ZipFS)(nil)

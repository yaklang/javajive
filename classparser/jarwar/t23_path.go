package jarwar

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/javajive/internal/filesys"
)

// SafeJoin confines name under root. It rejects NUL, absolute names, zip-slip `..`,
// and existing symlink components in the destination path. It does not follow
// input-controlled symlinks.
func SafeJoin(root, name string) (string, error) {
	if strings.IndexByte(root, 0) >= 0 || strings.IndexByte(name, 0) >= 0 {
		return "", pathRejected("NUL in path")
	}
	if strings.TrimSpace(root) == "" {
		return "", pathRejected("empty root")
	}

	n := strings.ReplaceAll(filepath.ToSlash(name), "\\", "/")
	n = strings.TrimSpace(n)
	n = strings.TrimPrefix(n, "./")
	if n == "" || n == "." {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		return filepath.Clean(rootAbs), nil
	}
	if path.IsAbs(n) || filepath.IsAbs(filepath.FromSlash(n)) {
		return "", pathRejected("absolute path")
	}
	if len(n) >= 2 && n[1] == ':' {
		return "", pathRejected("absolute path")
	}

	cleaned := path.Clean(n)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", pathRejected("zip-slip")
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return "", pathRejected("zip-slip")
		}
		if part == "" {
			continue
		}
		if strings.IndexByte(part, 0) >= 0 {
			return "", pathRejected("NUL in path")
		}
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootAbs = filepath.Clean(rootAbs)

	target := filepath.Join(rootAbs, filepath.FromSlash(cleaned))
	target = filepath.Clean(target)
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil {
		return "", pathRejected("escape")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", pathRejected("escape")
	}

	current := rootAbs
	parts := strings.Split(filepath.FromSlash(cleaned), string(os.PathSeparator))
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", pathRejected("zip-slip")
		}
		next := filepath.Join(current, part)
		fi, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				rest := parts[i:]
				current = filepath.Join(append([]string{current}, rest...)...)
				break
			}
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", pathRejected("symlink")
		}
		current = next
	}
	current = filepath.Clean(current)
	rel, err = filepath.Rel(rootAbs, current)
	if err != nil {
		return "", pathRejected("escape")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", pathRejected("escape")
	}
	return current, nil
}

func pathRejected(reason string) error {
	return &filesys.ArchiveError{
		Kind: filesys.KindInvalid,
		Code: filesys.CodeArchivePathRejected,
		Err:  errString(reason),
	}
}

type errString string

func (e errString) Error() string { return string(e) }

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxArchiveBytes   = int64(32 << 20)
	maxExtractedBytes = int64(128 << 20)
	maxArchiveEntries = 4096
	maxEntryBytes     = int64(32 << 20)
)

type fetchRecord struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	ArchiveSHA256 string `json:"archive_sha256"`
	ArchivePath   string `json:"archive_path"`
	Distribution  string `json:"distribution_path"`
	Launcher      string `json:"launcher_path"`
}

type fetchReport struct {
	SchemaVersion  int           `json:"schema_version"`
	ManifestSHA256 string        `json:"manifest_sha256"`
	Oracles        []fetchRecord `json:"oracles"`
}

func defaultCacheDir() string {
	if value := os.Getenv("DEX_ORACLE_CACHE"); value != "" {
		return value
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "javajive-dex-oracle")
	}
	return filepath.Join(dir, "javajive", "dex-oracle")
}

func runFetch(args []string) error {
	fs := newFlagSet("fetch")
	cache := fs.String("cache", defaultCacheDir(), "cache directory")
	manifestPath := fs.String("manifest", "", "alternate manifest")
	versions := fs.String("versions", "all", "comma-separated ids or versions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("fetch accepts no positional arguments")
	}
	m, manifestHash, err := loadManifest(*manifestPath)
	if err != nil {
		return err
	}
	oracles, err := selectOracles(m, *versions)
	if err != nil {
		return err
	}
	records := make([]fetchRecord, 0, len(oracles))
	for _, oracle := range oracles {
		record, err := ensureOracle(context.Background(), *cache, oracle)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", oracle.ID, err)
		}
		records = append(records, record)
	}
	return writeJSON(os.Stdout, fetchReport{SchemaVersion: 1, ManifestSHA256: manifestHash, Oracles: records})
}

func ensureOracle(ctx context.Context, cache string, oracle Oracle) (fetchRecord, error) {
	archiveDir := filepath.Join(cache, "archives")
	distDir := filepath.Join(cache, "distributions", oracle.SHA256)
	archivePath := filepath.Join(archiveDir, oracle.SHA256+"-"+oracle.ArchiveName)
	launcher := filepath.Join(distDir, oracle.DistributionRoot, filepath.FromSlash(oracle.Launcher))
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fetchRecord{}, err
	}
	if err := verifyFile(archivePath, oracle.SHA256, oracle.ArchiveSize); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(archivePath)
		}
		if err := downloadAtomic(ctx, oracle, archivePath); err != nil {
			return fetchRecord{}, err
		}
	}
	distribution := filepath.Join(distDir, oracle.DistributionRoot)
	if err := verifyDistribution(archivePath, distribution, oracle); err != nil {
		if err := extractAtomic(archivePath, filepath.Dir(distDir), filepath.Base(distDir), oracle); err != nil {
			return fetchRecord{}, err
		}
		if err := verifyDistribution(archivePath, distribution, oracle); err != nil {
			return fetchRecord{}, fmt.Errorf("verify extracted distribution: %w", err)
		}
	}
	info, err := os.Lstat(launcher)
	if err != nil || !info.Mode().IsRegular() {
		return fetchRecord{}, fmt.Errorf("launcher discovery failed for %s", oracle.Launcher)
	}
	return fetchRecord{ID: oracle.ID, Version: oracle.Version, ArchiveSHA256: oracle.SHA256, ArchivePath: archivePath, Distribution: filepath.Join(distDir, oracle.DistributionRoot), Launcher: launcher}, nil
}

func downloadAtomic(ctx context.Context, oracle Oracle, destination string) error {
	client := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" && !(req.URL.Scheme == "http" && isLoopbackHost(req.URL.Hostname())) {
				return fmt.Errorf("redirected to non-HTTPS URL")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oracle.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %s", resp.Status)
	}
	if resp.ContentLength > maxArchiveBytes || (resp.ContentLength >= 0 && resp.ContentLength != oracle.ArchiveSize) {
		return fmt.Errorf("unexpected Content-Length %d", resp.ContentLength)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != oracle.ArchiveSize || n > maxArchiveBytes {
		return fmt.Errorf("archive size mismatch: got %d, expected %d", n, oracle.ArchiveSize)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != oracle.SHA256 {
		return fmt.Errorf("archive hash mismatch: got %s, expected %s", got, oracle.SHA256)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, destination)
}

func verifyFile(filename, expectedHash string, expectedSize int64) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize {
		return fmt.Errorf("cached archive has unexpected type or size")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expectedHash {
		return fmt.Errorf("cached archive hash mismatch")
	}
	return nil
}

func extractAtomic(archivePath, parent, finalName string, oracle Oracle) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	if len(r.File) == 0 || len(r.File) > maxArchiveEntries {
		return fmt.Errorf("archive entry count %d is outside limits", len(r.File))
	}
	var total int64
	launcherPath := oracle.DistributionRoot + "/" + oracle.Launcher
	launcherFound := false
	for _, f := range r.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !safeRelativePath(name) {
			return fmt.Errorf("unsafe zip path %q", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return fmt.Errorf("unsupported zip entry type %q", f.Name)
		}
		if !strings.HasPrefix(f.Name, oracle.DistributionRoot+"/") {
			return fmt.Errorf("zip entry outside distribution root: %q", f.Name)
		}
		if int64(f.UncompressedSize64) > maxEntryBytes {
			return fmt.Errorf("zip entry too large: %q", f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxExtractedBytes {
			return fmt.Errorf("archive expands beyond %d bytes", maxExtractedBytes)
		}
		launcherFound = launcherFound || f.Name == launcherPath
	}
	if !launcherFound {
		return fmt.Errorf("archive lacks expected launcher %q", launcherPath)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, f := range r.File {
		target := filepath.Join(tmp, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			src.Close()
			return err
		}
		n, copyErr := io.Copy(dst, io.LimitReader(src, int64(f.UncompressedSize64)+1))
		closeErr := dst.Close()
		srcErr := src.Close()
		if copyErr != nil || closeErr != nil || srcErr != nil {
			return errors.Join(copyErr, closeErr, srcErr)
		}
		if n != int64(f.UncompressedSize64) {
			return fmt.Errorf("zip entry size mismatch: %q", f.Name)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ".complete"), []byte(oracle.SHA256+"\n"), 0o644); err != nil {
		return err
	}
	final := filepath.Join(parent, finalName)
	if _, err := os.Stat(final); err == nil {
		old := final + ".stale-" + fmt.Sprint(time.Now().UnixNano())
		if err := os.Rename(final, old); err != nil {
			return err
		}
		defer os.RemoveAll(old)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, final)
}

func verifyDistribution(archivePath, distributionDir string, oracle Oracle) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) == 0 || len(r.File) > maxArchiveEntries {
		return fmt.Errorf("archive entry count is outside limits")
	}
	expected := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(f.Name, oracle.DistributionRoot+"/")
		if name == f.Name || !safeRelativePath(name) {
			return fmt.Errorf("archive entry outside distribution root: %q", f.Name)
		}
		expected[filepath.Clean(filepath.FromSlash(name))] = f
	}
	seen := make(map[string]bool, len(expected))
	err = filepath.WalkDir(distributionDir, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(distributionDir, name)
		if err != nil || rel == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("distribution contains symlink %q", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("distribution contains non-regular file %q", rel)
		}
		if rel == ".complete" {
			return nil
		}
		want, ok := expected[filepath.Clean(rel)]
		if !ok {
			return fmt.Errorf("distribution contains unexpected file %q", rel)
		}
		got, err := os.Open(name)
		if err != nil {
			return err
		}
		defer got.Close()
		wantReader, err := want.Open()
		if err != nil {
			return err
		}
		defer wantReader.Close()
		gotHash := sha256.New()
		wantHash := sha256.New()
		gotN, gotErr := io.Copy(gotHash, io.LimitReader(got, maxEntryBytes+1))
		wantN, wantErr := io.Copy(wantHash, io.LimitReader(wantReader, maxEntryBytes+1))
		if gotErr != nil || wantErr != nil {
			return errors.Join(gotErr, wantErr)
		}
		if gotN != wantN || gotN != int64(want.UncompressedSize64) || !bytes.Equal(gotHash.Sum(nil), wantHash.Sum(nil)) {
			return fmt.Errorf("distribution file differs from pinned archive: %q", rel)
		}
		seen[filepath.Clean(rel)] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("distribution is incomplete: got %d files, want %d", len(seen), len(expected))
	}
	return nil
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func sortedJars(distribution string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(distribution, "lib", "*.jar"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("distribution has no lib/*.jar files")
	}
	for _, value := range paths {
		info, err := os.Lstat(value)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe classpath entry %q", value)
		}
	}
	return paths, nil
}

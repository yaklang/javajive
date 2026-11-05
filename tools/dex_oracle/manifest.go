package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

//go:embed manifest.json
var embeddedManifest []byte

type Manifest struct {
	SchemaVersion int      `json:"schema_version"`
	Oracles       []Oracle `json:"oracles"`
}

type Oracle struct {
	ID               string `json:"id"`
	Family           string `json:"family"`
	Version          string `json:"version"`
	Repository       string `json:"repository"`
	ReleaseTag       string `json:"release_tag"`
	SourceCommit     string `json:"source_commit"`
	URL              string `json:"url"`
	ArchiveName      string `json:"archive_name"`
	ArchiveSize      int64  `json:"archive_size"`
	SHA256           string `json:"sha256"`
	License          string `json:"license"`
	DistributionRoot string `json:"distribution_root"`
	Launcher         string `json:"launcher"`
}

const maxManifestBytes = int64(1 << 20)

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
	idRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

func loadManifest(filename string) (Manifest, string, error) {
	b := embeddedManifest
	if filename != "" {
		f, err := os.Open(filename)
		if err != nil {
			return Manifest{}, "", err
		}
		b, err = io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return Manifest{}, "", errors.Join(err, closeErr)
		}
		if int64(len(b)) > maxManifestBytes {
			return Manifest{}, "", fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
		}
	}
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, "", fmt.Errorf("decode manifest: %w", err)
	}
	if err := validateManifest(m); err != nil {
		return Manifest{}, "", err
	}
	sum := sha256.Sum256(b)
	return m, hex.EncodeToString(sum[:]), nil
}

func validateManifest(m Manifest) error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported manifest schema_version %d", m.SchemaVersion)
	}
	if len(m.Oracles) == 0 {
		return fmt.Errorf("manifest contains no oracles")
	}
	seen := make(map[string]bool)
	for i, o := range m.Oracles {
		prefix := fmt.Sprintf("oracle[%d]", i)
		if !idRE.MatchString(o.ID) || seen[o.ID] {
			return fmt.Errorf("%s has invalid or duplicate id %q", prefix, o.ID)
		}
		seen[o.ID] = true
		if o.Version == "" || o.Family == "" || o.ReleaseTag == "" || o.License == "" {
			return fmt.Errorf("%s is missing version, family, release_tag, or license", prefix)
		}
		if !hex64.MatchString(o.SHA256) || !hex40.MatchString(o.SourceCommit) {
			return fmt.Errorf("%s has invalid sha256 or source_commit", prefix)
		}
		if o.ArchiveSize <= 0 || o.ArchiveSize > maxArchiveBytes {
			return fmt.Errorf("%s archive_size is outside limits", prefix)
		}
		if path.Base(o.ArchiveName) != o.ArchiveName || !strings.HasSuffix(strings.ToLower(o.ArchiveName), ".zip") {
			return fmt.Errorf("%s has unsafe archive_name", prefix)
		}
		if !safeRelativePath(o.DistributionRoot) || strings.Contains(o.DistributionRoot, "/") || !safeRelativePath(o.Launcher) {
			return fmt.Errorf("%s has unsafe distribution or launcher path", prefix)
		}
		u, err := url.Parse(o.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
			return fmt.Errorf("%s has non-HTTPS release URL", prefix)
		}
		if path.Base(u.Path) != o.ArchiveName {
			return fmt.Errorf("%s URL basename does not match archive_name", prefix)
		}
		r, err := url.Parse(o.Repository)
		if err != nil || r.Scheme != "https" || r.Host != "github.com" {
			return fmt.Errorf("%s repository is not an HTTPS GitHub URL", prefix)
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func safeRelativePath(name string) bool {
	if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, `\`) || strings.HasPrefix(name, "/") {
		return false
	}
	clean := path.Clean(name)
	return clean == name && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func selectOracles(m Manifest, selection string) ([]Oracle, error) {
	wanted := make(map[string]bool)
	if selection != "" && selection != "all" {
		for _, value := range strings.Split(selection, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				return nil, fmt.Errorf("empty version selection")
			}
			wanted[value] = true
		}
	}
	var out []Oracle
	for _, o := range m.Oracles {
		if len(wanted) == 0 || wanted[o.ID] || wanted[o.Version] {
			out = append(out, o)
			delete(wanted, o.ID)
			delete(wanted, o.Version)
		}
	}
	if len(wanted) != 0 {
		unknown := make([]string, 0, len(wanted))
		for value := range wanted {
			unknown = append(unknown, value)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown oracle selection: %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

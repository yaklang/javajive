package input

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"math"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
	"github.com/yaklang/javajive/internal/inputkind"
)

// DEXSource is one DEX file discovered in a raw DEX, APK, APKS/XAPK/APKM
// bundle, or Android App Bundle (AAB).
type DEXSource struct {
	Entry         string
	MultidexIndex int
	Canonical     bool
	Data          []byte
}

// ParseApplicationWithOptions classifies data and parses every DEX file it
// contains. Identical classes repeated across split APKs or AAB modules are
// collapsed; conflicting definitions are rejected.
func ParseApplicationWithOptions(data []byte, name string, options Options) (*model.Application, inputkind.Kind, error) {
	sources, kind, err := CollectDEXSources(data, name, options)
	if err != nil {
		return nil, kind, err
	}
	strict := kind == inputkind.APK
	app, err := parseDEXSources(sources, options, strict)
	return app, kind, err
}

// CollectDEXSources classifies data and returns every DEX file it contains
// without parsing the DEX payloads.
func CollectDEXSources(data []byte, name string, options Options) ([]DEXSource, inputkind.Kind, error) {
	kind := inputkind.DetectBytes(data, name)
	switch kind {
	case inputkind.DEX:
		if int64(len(data)) > options.maxDEXSize() {
			return nil, kind, report.New("classes.dex", "", "", 0, "input", fmt.Errorf("DEX size %d exceeds limit %d", len(data), options.maxDEXSize()))
		}
		return []DEXSource{{Entry: "classes.dex", MultidexIndex: 1, Canonical: false, Data: data}}, kind, nil
	case inputkind.APK:
		sources, err := collectAPK(data, "", options, false)
		return sources, kind, err
	case inputkind.APKS, inputkind.XAPK, inputkind.APKM:
		sources, err := collectAPKS(data, kind, options)
		return sources, kind, err
	case inputkind.AAB:
		sources, err := collectAAB(data, options)
		return sources, kind, err
	default:
		return nil, kind, report.New(name, "", "", -1, "input", fmt.Errorf("unsupported DEX input: detected %s", kind))
	}
}

func collectAPK(data []byte, prefix string, options Options, allowEmpty bool) ([]DEXSource, error) {
	if int64(len(data)) > options.maxAPKSize() {
		return nil, report.New(prefix, "", "", -1, "APK", fmt.Errorf("archive size %d exceeds limit %d", len(data), options.maxAPKSize()))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, report.New(prefix, "", "", -1, "APK", err)
	}
	type entry struct {
		index int
		file  *zip.File
	}
	entries := make([]entry, 0)
	seenIndex := make(map[int]string)
	for _, file := range reader.File {
		index, canonical, indexErr := dexEntryIndex(file.Name)
		if indexErr != nil {
			return nil, report.New(qualifiedEntry(prefix, file.Name), "", "", -1, "APK entry", indexErr)
		}
		if !canonical {
			continue
		}
		if previous, exists := seenIndex[index]; exists {
			return nil, report.New(qualifiedEntry(prefix, file.Name), "", "", -1, "APK entry", fmt.Errorf("duplicate multidex index %d also used by %q", index, previous))
		}
		seenIndex[index] = file.Name
		entries = append(entries, entry{index: index, file: file})
	}
	if len(entries) == 0 {
		if allowEmpty {
			return nil, nil
		}
		return nil, report.New(prefix, "", "", -1, "APK", errors.New("no classes*.dex entries"))
	}
	if len(entries) > options.maxDEXFiles() {
		return nil, report.New(prefix, "", "", -1, "APK", fmt.Errorf("DEX file count %d exceeds limit %d", len(entries), options.maxDEXFiles()))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].index < entries[j].index })
	sources := make([]DEXSource, 0, len(entries))
	var totalDEXSize uint64
	for _, item := range entries {
		if item.file.UncompressedSize64 > uint64(options.maxDEXSize()) || item.file.UncompressedSize64 > uint64(math.MaxInt) {
			return nil, report.New(qualifiedEntry(prefix, item.file.Name), "", "", -1, "APK entry", fmt.Errorf("uncompressed size %d exceeds limit", item.file.UncompressedSize64))
		}
		if item.file.UncompressedSize64 > uint64(options.maxTotalDEXSize())-totalDEXSize {
			return nil, report.New(qualifiedEntry(prefix, item.file.Name), "", "", -1, "APK entry", fmt.Errorf("total DEX size exceeds limit %d", options.maxTotalDEXSize()))
		}
		totalDEXSize += item.file.UncompressedSize64
		contents, readErr := readZipEntry(item.file, options.maxDEXSize())
		if readErr != nil {
			return nil, report.New(qualifiedEntry(prefix, item.file.Name), "", "", -1, "APK entry", readErr)
		}
		sources = append(sources, DEXSource{
			Entry:         qualifiedEntry(prefix, item.file.Name),
			MultidexIndex: item.index,
			Canonical:     true,
			Data:          contents,
		})
	}
	return sources, nil
}

func collectAPKS(data []byte, kind inputkind.Kind, options Options) ([]DEXSource, error) {
	if int64(len(data)) > options.maxAPKSize() {
		return nil, report.New("", "", "", -1, string(kind), fmt.Errorf("container size %d exceeds limit %d", len(data), options.maxAPKSize()))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, report.New("", "", "", -1, string(kind), err)
	}
	nested := make([]*zip.File, 0)
	hasUniversal := false
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".apk") {
			continue
		}
		nested = append(nested, file)
		if strings.EqualFold(path.Base(file.Name), "universal.apk") {
			hasUniversal = true
		}
	}
	if len(nested) == 0 {
		return nil, report.New("", "", "", -1, string(kind), errors.New("no nested .apk entries"))
	}
	if len(nested) > options.maxNestedAPKs() {
		return nil, report.New("", "", "", -1, string(kind), fmt.Errorf("nested APK count %d exceeds limit %d", len(nested), options.maxNestedAPKs()))
	}
	sort.SliceStable(nested, func(i, j int) bool { return nested[i].Name < nested[j].Name })

	// bundletool's system mode emits one complete APK per device variant under
	// standalones/. They carry the same code, so translate a single variant.
	hasStandalone := false
	for _, file := range nested {
		if strings.HasPrefix(strings.ToLower(path.Clean(file.Name)), "standalones/") {
			hasStandalone = true
			break
		}
	}
	selectedStandalone := ""

	sources := make([]DEXSource, 0)
	var totalAPKSize uint64
	var totalDEXSize uint64
	for _, file := range nested {
		if hasUniversal && !strings.EqualFold(path.Base(file.Name), "universal.apk") {
			continue
		}
		if !hasUniversal && hasStandalone {
			if !strings.HasPrefix(strings.ToLower(path.Clean(file.Name)), "standalones/") {
				continue
			}
			if selectedStandalone == "" {
				selectedStandalone = file.Name
			}
			if file.Name != selectedStandalone {
				continue
			}
		}
		if file.UncompressedSize64 > uint64(options.maxAPKSize()) || file.UncompressedSize64 > uint64(math.MaxInt) {
			return nil, report.New(file.Name, "", "", -1, string(kind)+" entry", fmt.Errorf("uncompressed APK size %d exceeds limit", file.UncompressedSize64))
		}
		if file.UncompressedSize64 > uint64(options.maxTotalAPKSize())-totalAPKSize {
			return nil, report.New(file.Name, "", "", -1, string(kind)+" entry", fmt.Errorf("total nested APK size exceeds limit %d", options.maxTotalAPKSize()))
		}
		totalAPKSize += file.UncompressedSize64
		raw, readErr := readZipEntry(file, options.maxAPKSize())
		if readErr != nil {
			return nil, report.New(file.Name, "", "", -1, string(kind)+" entry", readErr)
		}
		nestedSources, parseErr := collectAPK(raw, file.Name, options, true)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, source := range nestedSources {
			totalDEXSize += uint64(len(source.Data))
			if totalDEXSize > uint64(options.maxTotalDEXSize()) {
				return nil, report.New(source.Entry, "", "", -1, string(kind)+" entry", fmt.Errorf("total DEX size exceeds limit %d", options.maxTotalDEXSize()))
			}
		}
		sources = append(sources, nestedSources...)
		if len(sources) > options.maxDEXFiles() {
			return nil, report.New("", "", "", -1, string(kind), fmt.Errorf("DEX file count %d exceeds limit %d", len(sources), options.maxDEXFiles()))
		}
	}
	if len(sources) == 0 {
		return nil, report.New("", "", "", -1, string(kind), errors.New("no classes*.dex entries in nested APKs"))
	}
	return sources, nil
}

func collectAAB(data []byte, options Options) ([]DEXSource, error) {
	if int64(len(data)) > options.maxAPKSize() {
		return nil, report.New("", "", "", -1, "aab", fmt.Errorf("container size %d exceeds limit %d", len(data), options.maxAPKSize()))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, report.New("", "", "", -1, "aab", err)
	}
	type entry struct {
		module string
		index  int
		file   *zip.File
	}
	entries := make([]entry, 0)
	seen := make(map[string]string)
	for _, file := range reader.File {
		module, index, canonical := inputkind.AABDEX(file.Name)
		if !canonical {
			continue
		}
		key := fmt.Sprintf("%s#%d", module, index)
		if previous, exists := seen[key]; exists {
			return nil, report.New(file.Name, "", "", -1, "aab entry", fmt.Errorf("duplicate module DEX index %d also used by %q", index, previous))
		}
		seen[key] = file.Name
		entries = append(entries, entry{module: module, index: index, file: file})
	}
	if len(entries) == 0 {
		return nil, report.New("", "", "", -1, "aab", errors.New("no <module>/dex/classes*.dex entries"))
	}
	if len(entries) > options.maxDEXFiles() {
		return nil, report.New("", "", "", -1, "aab", fmt.Errorf("DEX file count %d exceeds limit %d", len(entries), options.maxDEXFiles()))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].module != entries[j].module {
			return entries[i].module < entries[j].module
		}
		return entries[i].index < entries[j].index
	})
	sources := make([]DEXSource, 0, len(entries))
	var totalDEXSize uint64
	for _, item := range entries {
		if item.file.UncompressedSize64 > uint64(options.maxDEXSize()) || item.file.UncompressedSize64 > uint64(math.MaxInt) {
			return nil, report.New(item.file.Name, "", "", -1, "aab entry", fmt.Errorf("uncompressed size %d exceeds limit", item.file.UncompressedSize64))
		}
		if item.file.UncompressedSize64 > uint64(options.maxTotalDEXSize())-totalDEXSize {
			return nil, report.New(item.file.Name, "", "", -1, "aab entry", fmt.Errorf("total DEX size exceeds limit %d", options.maxTotalDEXSize()))
		}
		totalDEXSize += item.file.UncompressedSize64
		contents, readErr := readZipEntry(item.file, options.maxDEXSize())
		if readErr != nil {
			return nil, report.New(item.file.Name, "", "", -1, "aab entry", readErr)
		}
		sources = append(sources, DEXSource{
			Entry:         item.module + "/dex/" + path.Base(item.file.Name),
			MultidexIndex: item.index,
			Canonical:     true,
			Data:          contents,
		})
	}
	return sources, nil
}

func parseDEXSources(sources []DEXSource, options Options, strictDuplicates bool) (*model.Application, error) {
	if len(sources) == 0 {
		return nil, report.New("", "", "", -1, "input", errors.New("no DEX sources"))
	}
	if len(sources) > options.maxDEXFiles() {
		return nil, report.New("", "", "", -1, "input", fmt.Errorf("DEX file count %d exceeds limit %d", len(sources), options.maxDEXFiles()))
	}
	app := &model.Application{Files: make([]*model.File, 0, len(sources))}
	seen := make(map[string]model.Class)
	seenEntry := make(map[string]string)
	var totalDEXSize uint64
	for _, source := range sources {
		if int64(len(source.Data)) > options.maxDEXSize() {
			return nil, report.New(source.Entry, "", "", 0, "input", fmt.Errorf("DEX size %d exceeds limit %d", len(source.Data), options.maxDEXSize()))
		}
		if uint64(len(source.Data)) > uint64(options.maxTotalDEXSize())-totalDEXSize {
			return nil, report.New(source.Entry, "", "", 0, "input", fmt.Errorf("total DEX size exceeds limit %d", options.maxTotalDEXSize()))
		}
		totalDEXSize += uint64(len(source.Data))
		file, err := ParseDEXWithOptions(source.Data, source.Entry, options)
		if err != nil {
			return nil, err
		}
		kept := file.Classes[:0]
		for index := range file.Classes {
			class := &file.Classes[index]
			if previous, exists := seen[class.Descriptor]; exists {
				if !strictDuplicates && reflect.DeepEqual(previous, *class) {
					continue
				}
				return nil, report.New(source.Entry, class.Descriptor, "", -1, "duplicate class", fmt.Errorf("descriptor already defined in %q", seenEntry[class.Descriptor]))
			}
			seen[class.Descriptor] = *class
			seenEntry[class.Descriptor] = source.Entry
			kept = append(kept, *class)
		}
		file.Classes = kept
		app.Files = append(app.Files, file)
	}
	return app, nil
}

// dexEntryIndex parses a root-level canonical classes*.dex name. The bool is
// true when the name matches the canonical pattern, even when the index itself
// is invalid (for example classes01.dex).
func dexEntryIndex(name string) (index int, canonical bool, err error) {
	match := dexEntryPattern.FindStringSubmatch(name)
	if match == nil {
		return 0, false, nil
	}
	index = 1
	if match[1] == "" {
		return index, true, nil
	}
	parsed, parseErr := strconv.ParseInt(match[1], 10, 32)
	if parseErr != nil || parsed < 2 || match[1][0] == '0' {
		if parseErr == nil {
			parseErr = errors.New("index must be canonical and at least 2")
		}
		return 0, true, fmt.Errorf("invalid multidex index: %w", parseErr)
	}
	return int(parsed), true, nil
}

func qualifiedEntry(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "!/" + name
}

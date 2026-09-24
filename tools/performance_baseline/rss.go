package performance_baseline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type childSpec struct {
	FamilyID string `json:"family_id"`
	Stage    string `json:"stage"`
	Repeats  int    `json:"repeats"`
}

type childOut struct {
	NS           int64      `json:"ns"`
	BOp          int64      `json:"b_op"`
	Allocs       int64      `json:"allocs_op"`
	PeakRSSBytes *int64     `json:"peak_rss_bytes"`
	RSSSource    string     `json:"rss_source"`
	OK           bool       `json:"ok"`
	Error        string     `json:"error,omitempty"`
	OutputSHA256 string     `json:"output_sha256,omitempty"`
	Work         WorkCounts `json:"work"`
}

// RunChild is invoked from TestMain when T32_CHILD=1.
func RunChild() error {
	specPath := os.Getenv("T32_CHILD_SPEC")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	var spec childSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return err
	}
	fam, err := familyByID(spec.FamilyID)
	if err != nil {
		return err
	}
	if spec.Repeats < 1 {
		spec.Repeats = 1
	}
	var last Sample
	var lastErr error
	for i := 0; i < spec.Repeats; i++ {
		last, lastErr = measureOnce(spec.Stage, fam.Bytes)
		if lastErr != nil {
			break
		}
	}
	rss, src, rssErr := peakRSSBytes()
	var measuredRSS *int64
	if rssErr == nil {
		measuredRSS = &rss
	}
	work, _ := harvestWork(spec.Stage, fam.Bytes)
	out := childOut{
		NS:           last.NS,
		BOp:          last.BOp,
		Allocs:       last.Allocs,
		PeakRSSBytes: measuredRSS,
		RSSSource:    src,
		OK:           lastErr == nil && last.OK && rssErr == nil,
		Work:         work,
		OutputSHA256: work.OutputSHA256,
	}
	if lastErr != nil {
		out.Error = lastErr.Error()
	}
	if rssErr != nil {
		out.Error = strings.TrimSpace(out.Error + " rss:" + rssErr.Error())
	}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(out)
}

func spawnColdSample(exe, familyID, stage, tmpDir string) (Sample, WorkCounts, error) {
	spec := childSpec{FamilyID: familyID, Stage: stage, Repeats: 1}
	b, _ := json.Marshal(spec)
	specPath := tmpDir + "/spec-" + familyIDSafe(familyID) + "-" + stage + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".json"
	if err := os.WriteFile(specPath, b, 0o644); err != nil {
		return Sample{}, WorkCounts{}, err
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"T32_CHILD=1",
		"T32_CHILD_SPEC="+specPath,
		"GOTOOLCHAIN=local",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var out childOut
	if uerr := json.Unmarshal(stdout.Bytes(), &out); uerr != nil {
		return Sample{}, WorkCounts{}, fmt.Errorf("child json: %v (run-err=%v stderr=%s stdout=%s)", uerr, err, stderr.String(), stdout.String())
	}
	s := Sample{
		Role:         "measured",
		NS:           out.NS,
		BOp:          out.BOp,
		Allocs:       out.Allocs,
		PeakRSSBytes: out.PeakRSSBytes,
		RSSSource:    out.RSSSource,
		OK:           out.OK,
		Error:        out.Error,
		OutputSHA256: out.OutputSHA256,
	}
	return s, out.Work, err
}

func familyIDSafe(id string) string {
	return strings.ReplaceAll(id, "/", "_")
}

func timeDashL(exe, familyID, stage, tmpDir string) (int64, string, error) {
	if runtime.GOOS != "darwin" {
		return 0, "time_-l_not_darwin", nil
	}
	spec := childSpec{FamilyID: familyID, Stage: stage, Repeats: 1}
	b, _ := json.Marshal(spec)
	specPath := tmpDir + "/time-spec.json"
	if err := os.WriteFile(specPath, b, 0o644); err != nil {
		return 0, "", err
	}
	cmd := exec.Command("/usr/bin/time", "-l", exe)
	cmd.Env = append(os.Environ(), "T32_CHILD=1", "T32_CHILD_SPEC="+specPath, "GOTOOLCHAIN=local")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	rss, ok := parseTimeL(stderr.String())
	if !ok {
		return 0, stderr.String(), fmt.Errorf("could not parse /usr/bin/time -l")
	}
	return rss, "/usr/bin/time -l maximum resident set size", nil
}

func parseTimeL(stderr string) (int64, bool) {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "maximum resident set size") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			v, err := strconv.ParseInt(fields[0], 10, 64)
			if err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

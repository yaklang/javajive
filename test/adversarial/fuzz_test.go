package adversarial

import (
	"encoding/binary"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mutateCases deterministically derives malformed classfiles from a
// repository-authored seed. Inputs are constructed here; no external bytes are
// executed.
func mutateCases(seed []byte) map[string][]byte {
	rng := rand.New(rand.NewSource(20261105))
	cases := map[string][]byte{}

	for length := 0; length < len(seed); length += 64 {
		cases["truncate-"+strconv.Itoa(length)] = append([]byte(nil), seed[:length]...)
	}
	for i := 0; i < 300; i++ {
		mutated := append([]byte(nil), seed...)
		pos := rng.Intn(len(mutated))
		if i%2 == 0 {
			mutated[pos] ^= 0x01
		} else {
			mutated[pos] ^= 0x80
		}
		cases["flip-"+strconv.Itoa(i)] = mutated
	}
	for i := 0; i < 100; i++ {
		mutated := append([]byte(nil), seed...)
		pos := rng.Intn(len(mutated) - 4)
		binary.BigEndian.PutUint32(mutated[pos:], rng.Uint32())
		cases["word-"+strconv.Itoa(i)] = mutated
	}
	for i := 0; i < 50; i++ {
		mutated := append([]byte(nil), seed...)
		pos := rng.Intn(len(mutated) - 4)
		n := 1 + rng.Intn(4)
		cases["delete-"+strconv.Itoa(i)] = append(mutated[:pos], mutated[pos+n:]...)
	}
	for i := 0; i < 100; i++ {
		length := 16 + rng.Intn(496)
		junk := make([]byte, length)
		rng.Read(junk)
		copy(junk, []byte{0xca, 0xfe, 0xba, 0xbe})
		cases["junk-"+strconv.Itoa(i)] = junk
	}
	for _, version := range []byte{0, 1, 44, 45, 99, 255} {
		mutated := append([]byte(nil), seed...)
		mutated[6] = 0
		mutated[7] = version
		cases["version-"+strconv.Itoa(int(version))] = mutated
	}
	return cases
}

func TestAdversarialMalformed(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	seedPath := "../cross/testdata/irreducible/original/IrreducibleTwoEntry.class"
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}

	counts := map[string]int{}
	acceptedByKind := map[string]int{}
	var slowest time.Duration
	var slowName string
	for name, raw := range mutateCases(seed) {
		_, decompileErr, panicked, elapsed := decompileWith(raw, 3*time.Second)
		kind := ""
		switch {
		case panicked != nil:
			kind = "panic"
			t.Errorf("%s panicked: %v", name, panicked)
		case elapsed > 2*time.Second:
			kind = "slow"
			if elapsed > slowest {
				slowest, slowName = elapsed, name
			}
		case decompileErr != nil:
			kind = "rejected"
		default:
			kind = "accepted"
		}
		counts[kind]++
		if kind == "accepted" {
			prefix := name
			if dash := strings.IndexByte(name, '-'); dash >= 0 {
				prefix = name[:dash]
			}
			acceptedByKind[prefix]++
		}
		if elapsed > slowest {
			slowest, slowName = elapsed, name
		}
	}
	t.Logf("malformed inputs: %d total, rejected=%d accepted=%d panics=%d slow=%d",
		len(counts), counts["rejected"], counts["accepted"], counts["panic"], counts["slow"])
	keys := make([]string, 0, len(acceptedByKind))
	for k := range acceptedByKind {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("accepted family %-10s %d", k, acceptedByKind[k])
	}
	t.Logf("slowest input: %s at %s", slowName, slowest.Round(time.Millisecond))
	if counts["panic"] != 0 {
		t.Fatalf("%d malformed inputs panicked", counts["panic"])
	}
	if counts["slow"] != 0 {
		t.Fatalf("%d malformed inputs exceeded the 2s bound (slowest %s at %s)", counts["slow"], slowName, slowest)
	}
}

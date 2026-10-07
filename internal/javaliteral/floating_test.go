package javaliteral

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Finite values are checked with an independent decimal parser; NaN runtime
// operands are checked as integer words. The compiled JVM banks separately
// establish that these words survive real invocation, boxing, storage and return.
func TestRuntimeFloatingLiteralFiniteWordModel(t *testing.T) {
	for _, width := range []int{32, 64} {
		count := 0
		exponents := []uint64{0, 1, 16, 126, 127, 128, 253, 254}
		mantissaBits := uint(23)
		if width == 64 {
			exponents = []uint64{0, 1, 16, 1022, 1023, 1024, 2019, 2045, 2046}
			mantissaBits = 52
		}
		mask := uint64(1)<<mantissaBits - 1
		for _, exponent := range exponents {
			for sign := uint64(0); sign < 2; sign++ {
				for i := uint64(0); i < 64; i++ {
					mantissa := (i * 0x123456781) & mask
					switch i {
					case 0:
						mantissa = 0
					case 1:
						mantissa = 1
					case 2:
						mantissa = mask
					case 3:
						mantissa = mask >> 1
					}
					word := sign<<uint(width-1) | exponent<<mantissaBits | mantissa
					calls := 0
					owner := func(name string) string { calls++; return "WrongOwner" }
					var text string
					if width == 32 {
						text = RuntimeFloat32(math.Float32frombits(uint32(word)), owner)
					} else {
						text = RuntimeFloat64(math.Float64frombits(word), owner)
					}
					if calls != 0 {
						t.Fatalf("finite word %#x introduced an owner", word)
					}
					value, err := strconv.ParseFloat(text[:len(text)-1], width)
					if err != nil {
						t.Fatalf("word %#x produced invalid finite literal %q: %v", word, text, err)
					}
					got := math.Float64bits(value)
					if width == 32 {
						got = uint64(math.Float32bits(float32(value)))
					}
					if got != word {
						t.Fatalf("width=%d word=%#x reparsed=%#x source=%q", width, word, got, text)
					}
					count++
				}
			}
		}
		if count < 1000 {
			t.Fatalf("finite width=%d model only %d words", width, count)
		}
	}
}

func TestRuntimeQuietNaNLiteralIntegerWordModel(t *testing.T) {
	for _, width := range []int{32, 64} {
		for sign := uint64(0); sign < 2; sign++ {
			for i := uint64(0); i < 1024; i++ {
				word := uint64(0x7fc00000) | (i*0x10531)&0x003fffff
				intrinsic := ".intBitsToFloat(0x"
				if width == 64 {
					word = 0x7ff8000000000000 | (i*0x123456781)&0x0007ffffffffffff
					intrinsic = ".longBitsToDouble(0x"
				}
				word |= sign << uint(width-1)
				canonical := sign == 0 && i == 0
				calls := 0
				owner := func(name string) string { calls++; return "BoundOwner" }
				var text string
				if width == 32 {
					text = RuntimeFloat32(math.Float32frombits(uint32(word)), owner)
				} else {
					text = RuntimeFloat64(math.Float64frombits(word), owner)
				}
				if canonical {
					if calls != 0 || !strings.Contains(text, "/0.0") {
						t.Fatalf("canonical word gained a dependency: %s", text)
					}
					continue
				}
				if calls != 1 || !strings.HasPrefix(text, "BoundOwner"+intrinsic) {
					t.Fatalf("raw word lacks one bound intrinsic: %s, calls=%d", text, calls)
				}
				hex := strings.TrimSuffix(strings.TrimPrefix(text, "BoundOwner"+intrinsic), ")")
				hex = strings.TrimSuffix(hex, "L")
				got, err := strconv.ParseUint(hex, 16, width)
				if err != nil || got != word {
					t.Fatalf("width=%d word=%#x rendered=%#x source=%q error=%v", width, word, got, text, err)
				}
			}
		}
	}
}

func TestRuntimeSpecialConstantsDoNotAcquireTypeDependencies(t *testing.T) {
	owner := func(string) string { t.Fatal("constant expression requested a class owner"); return "" }
	for _, value := range []float64{0, math.Copysign(0, -1), 1, -1, math.Inf(1), math.Inf(-1), math.Float64frombits(0x7ff8000000000000)} {
		if got, want := RuntimeFloat64(value, owner), Float64(value); got != want {
			t.Fatalf("constant %v got=%s want=%s", value, got, want)
		}
	}
	for _, word := range []uint32{0, 0x80000000, 0x3f800000, 0xbf800000, 0x7f800000, 0xff800000, 0x7fc00000} {
		value := math.Float32frombits(word)
		if got, want := RuntimeFloat32(value, owner), Float32(value); got != want {
			t.Fatalf("constant %#x got=%s want=%s", word, got, want)
		}
	}
	// A runtime call is forbidden in ConstantValue / annotation contexts even
	// for noncanonical payloads: this renderer preserves constant-expression status.
	for _, word := range []uint64{0x7ff8000000001234, 0xfff8000000001234} {
		if text := Float64(math.Float64frombits(word)); strings.Contains(text, "BitsTo") {
			t.Fatalf("metadata gained runtime initialization: %s", text)
		}
	}
}

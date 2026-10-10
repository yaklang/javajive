package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ordinary Java NaN constant expressions cannot promise these original payload
// bits. The general initializer fallback must not bypass the literal proof.
func TestNativeAnonymousInitializerRefusesNoncanonicalNaNPayload(t *testing.T) {
	_, java := t04Tools(t)
	for _, variant := range []string{"positive float", "negative float", "positive double", "negative double"} {
		t.Run(variant, func(t *testing.T) {
			floatBits := uint32(0x3fa00000)
			doubleBits := uint64(0x3ff4000000000000)
			switch variant {
			case "positive float":
				floatBits = 0x7fc01234
			case "negative float":
				floatBits = 0xffc01234
			case "positive double":
				doubleBits = 0x7ff8000000001234
			case "negative double":
				doubleBits = 0xfff8000000001234
			}
			fixture := strings.Replace(nativeAnonymousInitializerFixture, `Object ref=captured;`, `Object ref=captured;float payload=1.25f;double payloadDouble=1.25;`, 1)
			fixture = strings.Replace(fixture, `main(String[]args){`, `main(String[]args)throws Exception{`, 1)
			fixture = strings.Replace(fixture, `rows++;`, fmt.Sprintf(`java.lang.reflect.Field f=p.getClass().getDeclaredField("payload"),g=p.getClass().getDeclaredField("payloadDouble");f.setAccessible(true);g.setAccessible(true);if(Float.floatToRawIntBits(f.getFloat(p))!=(int)0x%08xL||Double.doubleToRawLongBits(g.getDouble(p))!=0x%016xL)throw new AssertionError("original NaN payload");rows++;`, floatBits, doubleBits), 1)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileDebugClasses(t, fixture, debug)
					raw := append([]byte(nil), files["AnonymousInitOwner$1.class"]...)
					for _, change := range []struct {
						original    []byte
						replacement []byte
					}{
						{[]byte{4, 0x3f, 0xa0, 0, 0}, binary.BigEndian.AppendUint32(nil, floatBits)},
						{[]byte{6, 0x3f, 0xf4, 0, 0, 0, 0, 0, 0}, binary.BigEndian.AppendUint64(nil, doubleBits)},
					} {
						if bytes.Count(raw, change.original) != 1 {
							t.Fatal("unique original constant bytes")
						}
						offset := bytes.Index(raw, change.original) + 1
						copy(raw[offset:offset+len(change.replacement)], change.replacement)
					}
					obj, e := Parse(raw)
					if e != nil {
						t.Fatal(e)
					}
					files["AnonymousInitOwner$1.class"] = raw
					out := t.TempDir()
					for name, raw := range files {
						if e := os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					if got := t04RunJava(t, java, out, "AnonymousInitDriver"); got != "6:anonymous:initializer:callback:identity:owner\n" {
						t.Fatalf("valid original payload=%q", got)
					}
					if nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make", nil) != nil {
						t.Fatal("general fallback accepted payload-changing NaN source")
					}
				})
			}
		})
	}
}

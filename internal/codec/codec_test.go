package codec

import "testing"

func TestEncodeDecodeHex(t *testing.T) {
	if got := EncodeToHex("AB"); got != "4142" {
		t.Fatalf("EncodeToHex = %q want 4142", got)
	}
	if got := EncodeToHex([]byte{0x01, 0xff}); got != "01ff" {
		t.Fatalf("EncodeToHex bytes = %q want 01ff", got)
	}
	b, err := DecodeHex("4142")
	if err != nil || string(b) != "AB" {
		t.Fatalf("DecodeHex = %q,%v want AB", b, err)
	}
}

func TestBase64(t *testing.T) {
	if got := EncodeBase64("hello"); got != "aGVsbG8=" {
		t.Fatalf("EncodeBase64 = %q", got)
	}
	b, err := DecodeBase64("aGVsbG8=")
	if err != nil || string(b) != "hello" {
		t.Fatalf("DecodeBase64 = %q,%v", b, err)
	}
}

func TestSha256(t *testing.T) {
	// echo -n "abc" | sha256sum
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := Sha256("abc"); got != want {
		t.Fatalf("Sha256 = %s want %s", got, want)
	}
}

func TestAnyConversions(t *testing.T) {
	if got := AnyToString([]byte("xy")); got != "xy" {
		t.Fatalf("AnyToString bytes = %q", got)
	}
	if got := AnyToString(42); got != "42" {
		t.Fatalf("AnyToString int = %q", got)
	}
	if got := string(AnyToBytes("z")); got != "z" {
		t.Fatalf("AnyToBytes = %q", got)
	}
}

func TestMatchMIMETypeStub(t *testing.T) {
	res, err := MatchMIMEType([]byte("anything"))
	if err == nil {
		t.Fatal("trimmed MatchMIMEType should return an error")
	}
	if res != nil {
		t.Fatal("trimmed MatchMIMEType should return nil result")
	}
}

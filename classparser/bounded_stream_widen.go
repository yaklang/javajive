package javaclassparser

import (
	"os"
	"strings"
)

// fixBoundedStreamWiden retypes a BoundedInputStream local that is later
// assigned CRC32VerifyingInputStream or CheckedInputStream (neither is a
// BoundedInputStream) to InputStream.
// Kill-switch: JDEC_BOUNDED_STREAM_WIDEN_OFF=1.
func fixBoundedStreamWiden(body string) string {
	if os.Getenv("JDEC_BOUNDED_STREAM_WIDEN_OFF") == "1" {
		return body
	}
	const prefix = "BoundedInputStream "
	from := 0
	for {
		rel := strings.Index(body[from:], prefix)
		if rel < 0 {
			return body
		}
		i := from + rel
		ident, ok, rest := readJavaIdent(body[i+len(prefix):])
		if !ok || !isDecompilerLocal(ident) || !strings.HasPrefix(rest, " = new BoundedInputStream") {
			from = i + 1
			continue
		}
		chunk := body[i:nextMemberStart(body, i)]
		if strings.Contains(chunk, ident+" = new CRC32VerifyingInputStream") ||
			strings.Contains(chunk, ident+" = new CheckedInputStream") {
			body = body[:i] + "InputStream " + body[i+len(prefix):]
			from = i + len("InputStream ")
			continue
		}
		from = i + 1
	}
}

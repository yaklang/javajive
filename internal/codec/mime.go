package codec

import "errors"

// MIMEResult is a minimal stand-in for the original yaklang MIME detection result.
//
// The original implementation performed full magic-byte sniffing plus charset
// recovery (GB18030/GBK -> UTF-8), which pulled in heavyweight third-party
// dependencies. javajive only used this on a best-effort optimization path for
// recovering mis-decoded Chinese string literals during decompilation; skipping it
// is behavior-preserving for the common case (the caller falls back to the normal
// quoting path). The type and methods are retained so callers compile unchanged.
type MIMEResult struct {
	MIMEType string
	Charset  string
	IsText   bool
}

// IsChineseCharset always reports false in this trimmed build, so the optional
// charset-recovery branch is skipped and callers use their default behavior.
func (m *MIMEResult) IsChineseCharset() bool { return false }

// TryUTF8Convertor is a no-op in this trimmed build.
func (m *MIMEResult) TryUTF8Convertor(raw []byte) ([]byte, bool) { return raw, false }

// MatchMIMEType is a trimmed stub: it never detects a charset, so callers always
// take their default (non-recovery) path.
func MatchMIMEType(raw any) (*MIMEResult, error) {
	return nil, errors.New("MatchMIMEType is not supported in the trimmed javajive codec build")
}

package javaclassparser

import (
	"errors"
	"fmt"
)

// Parse failure codes. These are diagnostic codes on ClassParseError, not a
// replacement for DecompileResult.Status strings.
const (
	ParseCodeTruncated          = "truncated"
	ParseCodeTrailingBytes      = "trailing_bytes"
	ParseCodeBadMagic           = "bad_magic"
	ParseCodeLengthOverflow     = "length_overflow"
	ParseCodeCPCount            = "cp_count"
	ParseCodeCPTag              = "cp_tag"
	ParseCodeCPIndex            = "cp_index"
	ParseCodeAttrLength         = "attr_length"
	ParseCodeAttrPlacement      = "attr_placement"
	ParseCodeAttrDuplicate      = "attr_duplicate"
	ParseCodeCodeLength         = "code_length"
	ParseCodeHandlerRange       = "handler_range"
	ParseCodeResourceLimit      = "resource_limit"
	ParseCodeInvalidInput       = "invalid_input"
	ParseCodeUnsupportedVersion = "unsupported_version"
	ParseCodeUnsupportedTarget  = "unsupported_target"
)

// ClassParseError is a structured class-file parse failure.
// Error() always includes offset, stage, and code (T05-M02).
type ClassParseError struct {
	Code   string
	Offset int
	Stage  string
	Field  string
	Msg    string
}

func (e *ClassParseError) Error() string {
	if e == nil {
		return "class parse error"
	}
	msg := e.Msg
	if msg == "" {
		msg = e.Code
	}
	return fmt.Sprintf("class parse error: code=%s offset=%d stage=%s field=%s: %s",
		e.Code, e.Offset, e.Stage, e.Field, msg)
}

// StatusClassifies maps a parse error onto DecompileResult.Status.
// Format errors are invalid_input. Version/capability codes are unsupported.
// Budget/partial statuses are produced after a successful Parse, not here.
func StatusClassifies(err error) string {
	if err == nil {
		return "complete"
	}
	var pe *ClassParseError
	if errors.As(err, &pe) && pe != nil {
		switch pe.Code {
		case ParseCodeUnsupportedVersion, ParseCodeUnsupportedTarget:
			return "unsupported"
		case ParseCodeResourceLimit:
			return "partial"
		}
	}
	return "invalid_input"
}

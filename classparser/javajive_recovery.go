package javaclassparser

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	javaJiveRecoveryAttributeName = "JavaJiveRecovery"
	javaJiveRecoveryVersion       = 1
	javaJiveRecoveryMaxPayload    = 64 << 10
	javaJiveRecoveryMaxRecords    = 128
	javaJiveRecoveryMaxFieldRunes = 240
)

// javaJiveRecoveryRecord is the version 1 payload record written by recovery
// producers. A JavaJiveRecovery attribute contains a JSON array of these.
type javaJiveRecoveryRecord struct {
	Version    int    `json:"version"`
	Code       string `json:"code"`
	Action     string `json:"action"`
	Entry      string `json:"entry,omitempty"`
	Class      string `json:"class,omitempty"`
	Member     string `json:"member,omitempty"`
	Descriptor string `json:"descriptor,omitempty"`
	Reason     string `json:"reason"`
}

func javaJiveRecoveryComments(obj *ClassObject) string {
	if obj == nil {
		return ""
	}
	lines := make([]string, 0)
	for _, attribute := range obj.Attributes {
		unparsed, ok := attribute.(*UnparsedAttribute)
		if !ok || unparsed == nil || unparsed.Name != javaJiveRecoveryAttributeName {
			continue
		}
		records, ok := parseJavaJiveRecoveryPayload(unparsed)
		if !ok {
			continue
		}
		if len(lines)+len(records) > javaJiveRecoveryMaxRecords {
			return ""
		}
		for _, record := range records {
			lines = append(lines, formatJavaJiveRecoveryRecord(record))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func parseJavaJiveRecoveryPayload(attribute *UnparsedAttribute) ([]javaJiveRecoveryRecord, bool) {
	if attribute == nil || attribute.Length != uint32(len(attribute.Info)) || len(attribute.Info) == 0 || len(attribute.Info) > javaJiveRecoveryMaxPayload {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(attribute.Info))
	decoder.DisallowUnknownFields()
	var records []javaJiveRecoveryRecord
	if err := decoder.Decode(&records); err != nil || len(records) == 0 || len(records) > javaJiveRecoveryMaxRecords {
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, false
	}
	for i := range records {
		record := &records[i]
		if record.Version != javaJiveRecoveryVersion || strings.TrimSpace(record.Code) == "" || strings.TrimSpace(record.Action) == "" || strings.TrimSpace(record.Reason) == "" {
			return nil, false
		}
		record.Code = sanitizeJavaJiveRecoveryField(record.Code)
		record.Action = sanitizeJavaJiveRecoveryField(record.Action)
		record.Entry = sanitizeJavaJiveRecoveryField(record.Entry)
		record.Class = sanitizeJavaJiveRecoveryField(record.Class)
		record.Member = sanitizeJavaJiveRecoveryField(record.Member)
		record.Descriptor = sanitizeJavaJiveRecoveryField(record.Descriptor)
		record.Reason = sanitizeJavaJiveRecoveryField(record.Reason)
		if record.Code == "" || record.Action == "" || record.Reason == "" {
			return nil, false
		}
	}
	return records, true
}

func sanitizeJavaJiveRecoveryField(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	value = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029':
			return ' '
		case unicode.IsControl(r):
			return ' '
		default:
			return r
		}
	}, value)
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= javaJiveRecoveryMaxFieldRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:javaJiveRecoveryMaxFieldRunes])
}

func formatJavaJiveRecoveryRecord(record javaJiveRecoveryRecord) string {
	var line strings.Builder
	line.WriteString("// javajive recovery:")
	appendField := func(name, value string) {
		if value == "" {
			return
		}
		line.WriteByte(' ')
		line.WriteString(name)
		line.WriteByte('=')
		line.WriteString(strconv.Quote(value))
	}
	appendField("code", record.Code)
	appendField("action", record.Action)
	appendField("entry", record.Entry)
	appendField("class", record.Class)
	appendField("member", record.Member)
	appendField("descriptor", record.Descriptor)
	appendField("reason", record.Reason)
	return line.String()
}

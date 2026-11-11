package dexparser

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func TestParseDebugStreamBoundsHeaderAndRegisters(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		registers  uint16
		parameters uint32
		want       string
	}{
		{name: "declared parameters", data: []byte{1, 2, 0, 0}, registers: 2, parameters: 1, want: "exceeds declared parameters"},
		{name: "register", data: []byte{1, 0, dbgStartLocal, 2, 0, 0, dbgEndSequence}, registers: 2, want: "local register v2 exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &parser{data: test.data, file: &model.File{}}
			_, err := p.parseDebugStream(0, 1, test.registers, test.parameters)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseDebugStream() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseDebugStreamRestartLocalScale(t *testing.T) {
	const locals = 4096
	data := []byte{1, 0}
	for register := 0; register < locals; register++ {
		data = append(data, dbgStartLocal)
		data = appendTestULEB(data, uint32(register))
		data = append(data, 0, 0, dbgEndLocal)
		data = appendTestULEB(data, uint32(register))
	}
	for register := 0; register < locals; register++ {
		data = append(data, dbgRestartLocal)
		data = appendTestULEB(data, uint32(register))
	}
	data = append(data, dbgEndSequence)

	p := &parser{data: data, file: &model.File{}}
	info, err := p.parseDebugStream(0, 1, locals, 0)
	if err != nil {
		t.Fatalf("parseDebugStream() error = %v", err)
	}
	if got, want := len(info.Locals), locals*2; got != want {
		t.Fatalf("local count = %d, want %d", got, want)
	}
}

func BenchmarkParseDebugRestartLocalScale(b *testing.B) {
	const locals = 8192
	data := []byte{1, 0}
	for register := 0; register < locals; register++ {
		data = append(data, dbgStartLocal)
		data = appendTestULEB(data, uint32(register))
		data = append(data, 0, 0, dbgEndLocal)
		data = appendTestULEB(data, uint32(register))
	}
	for register := 0; register < locals; register++ {
		data = append(data, dbgRestartLocal)
		data = appendTestULEB(data, uint32(register))
	}
	data = append(data, dbgEndSequence)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		p := &parser{data: data, file: &model.File{}}
		if _, err := p.parseDebugStream(0, 1, locals, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func appendTestULEB(data []byte, value uint32) []byte {
	for {
		part := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			part |= 0x80
		}
		data = append(data, part)
		if value == 0 {
			return data
		}
	}
}

package javaclassparser

import (
	"encoding/binary"
	"fmt"
)

/*
*
jvm中定义了u1，u2，u4来表示1，2，4字节的 无 符号整数
相同类型的多条数据一般按表的形式存储在class文件中，由表头和表项构成，表头是u2或者u4整数。
假设表头为10，后面就紧跟着10个表项数据
*/
// ClassReader is a bounded cursor over classfile bytes. Failed reads stick an
// error and never advance past the view bound.
//
// orig is the original buffer (absolute offsets). data is kept as the remaining
// window orig[pos:end] so existing remaining-slice snapshots (attribute_info)
// stay valid. Always 0 <= pos <= end <= len(orig).
type ClassReader struct {
	orig   []byte
	data   []byte // remaining view; always orig[pos:end]
	pos    int
	end    int
	err    error
	stage  string
	field  string
	parent *ClassReader
}

func NewClassReader(data []byte) *ClassReader {
	if data == nil {
		data = []byte{}
	}
	r := &ClassReader{
		orig: data,
		pos:  0,
		end:  len(data),
	}
	r.syncData()
	return r
}

func (r *ClassReader) syncData() {
	if r.pos < 0 {
		r.pos = 0
	}
	if r.end > len(r.orig) {
		r.end = len(r.orig)
	}
	if r.pos > r.end {
		r.pos = r.end
	}
	r.data = r.orig[r.pos:r.end]
}

// Offset is the absolute cursor in the original buffer.
func (r *ClassReader) Offset() int {
	if r == nil {
		return 0
	}
	return r.pos
}

// Remaining is bytes left in this view (never negative).
func (r *ClassReader) Remaining() int {
	if r == nil {
		return 0
	}
	if r.pos >= r.end {
		return 0
	}
	return r.end - r.pos
}

// Bound is the exclusive absolute end of this view.
func (r *ClassReader) Bound() int {
	if r == nil {
		return 0
	}
	return r.end
}

func (r *ClassReader) Err() error {
	if r == nil {
		return nil
	}
	return r.err
}

func (r *ClassReader) SetStage(stage, field string) {
	if r == nil {
		return
	}
	r.stage = stage
	r.field = field
}

func (r *ClassReader) fail(code, msg string) error {
	if r == nil {
		return &ClassParseError{Code: code, Msg: msg}
	}
	if r.err != nil {
		return r.err
	}
	e := &ClassParseError{
		Code:   code,
		Offset: r.pos,
		Stage:  r.stage,
		Field:  r.field,
		Msg:    msg,
	}
	r.err = e
	for p := r.parent; p != nil; p = p.parent {
		if p.err == nil {
			p.err = e
		}
	}
	return e
}

func (r *ClassReader) hasUint32(length uint32) bool {
	rem := r.Remaining()
	if rem < 0 {
		return false
	}
	return uint64(length) <= uint64(rem)
}

func (r *ClassReader) need(n int, field string) bool {
	if r.err != nil {
		return false
	}
	if field != "" {
		r.field = field
	}
	if n < 0 {
		r.fail(ParseCodeLengthOverflow, "negative length")
		return false
	}
	if r.Remaining() < n {
		r.fail(ParseCodeTruncated, fmt.Sprintf("need %d bytes, remaining %d", n, r.Remaining()))
		return false
	}
	return true
}

// Subreader reserves the next length bytes as a child view. The parent cursor
// advances by length immediately so a child cannot consume the next attribute.
// If length exceeds remaining, the parent fails without advancing past Bound.
func (r *ClassReader) Subreader(length uint32) *ClassReader {
	child := &ClassReader{
		orig:   r.orig,
		pos:    r.pos,
		end:    r.pos,
		stage:  r.stage,
		field:  r.field,
		parent: r,
		err:    r.err,
	}
	if r.err != nil {
		child.syncData()
		return child
	}
	if !r.hasUint32(length) {
		r.field = "subreader"
		code := ParseCodeTruncated
		if length == 0xffffffff {
			code = ParseCodeLengthOverflow
		}
		r.fail(code, fmt.Sprintf("subreader length %d exceeds remaining %d", length, r.Remaining()))
		child.err = r.err
		child.syncData()
		return child
	}
	n := int(length)
	child.end = r.pos + n
	child.err = nil
	child.syncData()
	r.pos += n
	r.syncData()
	return child
}

/*
*
相当于java的 byte 8位无符号整数
*/
func (this *ClassReader) readUint8() uint8 {
	if !this.need(1, "u1") {
		return 0
	}
	val := this.orig[this.pos]
	this.pos++
	this.syncData()
	return val
}

/*
*
相当于java的 short 16位无符号整数
这里class文件在文件系统中以大端法存储
*/
func (this *ClassReader) readUint16() uint16 {
	if !this.need(2, "u2") {
		return 0
	}
	val := binary.BigEndian.Uint16(this.orig[this.pos : this.pos+2])
	this.pos += 2
	this.syncData()
	return val
}

/*
*
相当于java的 int 32位无符号整数
*/
func (this *ClassReader) readUint32() uint32 {
	if !this.need(4, "u4") {
		return 0
	}
	val := binary.BigEndian.Uint32(this.orig[this.pos : this.pos+4])
	this.pos += 4
	this.syncData()
	return val
}

/*
*
相当于java的 long 64位无符号整数
*/
func (this *ClassReader) readUint64() uint64 {
	if !this.need(8, "u8") {
		return 0
	}
	val := binary.BigEndian.Uint64(this.orig[this.pos : this.pos+8])
	this.pos += 8
	this.syncData()
	return val
}

/*
*
读取uint16表，表的大小由开头的uint16数据指出
*/
func (this *ClassReader) readUint16s() []uint16 {
	n := this.readUint16()
	if this.err != nil {
		return nil
	}
	need := int(n) * 2
	if !this.need(need, "u2s") {
		return nil
	}
	s := make([]uint16, n)
	for i := range s {
		s[i] = this.readUint16()
		if this.err != nil {
			return nil
		}
	}
	return s
}

/*
*
读取制定length数量的字节
*/
func (this *ClassReader) readBytes(length uint32) []byte {
	if this.err != nil {
		return nil
	}
	this.field = "bytes"
	if !this.hasUint32(length) {
		code := ParseCodeTruncated
		if length == 0xffffffff {
			code = ParseCodeLengthOverflow
		}
		this.fail(code, fmt.Sprintf("readBytes length %d exceeds remaining %d", length, this.Remaining()))
		return nil
	}
	n := int(length)
	bytes := this.orig[this.pos : this.pos+n]
	this.pos += n
	this.syncData()
	return bytes
}

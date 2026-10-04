package performance_baseline

import (
	"encoding/binary"
	"fmt"
)

// Deterministic Java 8 class-file assembler for synthetic T32 families.
// It emits legal ClassFile bytes; it is not a general-purpose compiler.

type cpBuilder struct {
	items [][]byte
	utf8  map[string]uint16
}

func newCP() *cpBuilder {
	return &cpBuilder{utf8: map[string]uint16{}}
}

func (c *cpBuilder) add(raw []byte) uint16 {
	c.items = append(c.items, raw)
	return uint16(len(c.items))
}

func (c *cpBuilder) Utf8(s string) uint16 {
	if i, ok := c.utf8[s]; ok {
		return i
	}
	buf := make([]byte, 3+len(s))
	buf[0] = 1
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(s)))
	copy(buf[3:], s)
	i := c.add(buf)
	c.utf8[s] = i
	return i
}

func (c *cpBuilder) Class(name string) uint16 {
	n := c.Utf8(name)
	buf := []byte{7, 0, 0}
	binary.BigEndian.PutUint16(buf[1:], n)
	return c.add(buf)
}

func (c *cpBuilder) NameAndType(name, desc string) uint16 {
	n := c.Utf8(name)
	d := c.Utf8(desc)
	buf := []byte{12, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(buf[1:3], n)
	binary.BigEndian.PutUint16(buf[3:5], d)
	return c.add(buf)
}

func (c *cpBuilder) Methodref(class, name, desc string) uint16 {
	cl := c.Class(class)
	nat := c.NameAndType(name, desc)
	buf := []byte{10, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(buf[1:3], cl)
	binary.BigEndian.PutUint16(buf[3:5], nat)
	return c.add(buf)
}

type assembler struct {
	code   []byte
	labels map[string]int
	fixes  []branchFix
}

type branchFix struct {
	at, from int
	label    string
}

func newAsm() *assembler {
	return &assembler{labels: map[string]int{}}
}

func (a *assembler) emit(op ...byte) { a.code = append(a.code, op...) }

func (a *assembler) mark(name string) { a.labels[name] = len(a.code) }

func (a *assembler) pc() int { return len(a.code) }

func (a *assembler) branch(op byte, label string) {
	from := len(a.code)
	a.code = append(a.code, op, 0, 0)
	a.fixes = append(a.fixes, branchFix{at: from + 1, from: from, label: label})
}

func (a *assembler) finish() ([]byte, error) {
	for _, p := range a.fixes {
		target, ok := a.labels[p.label]
		if !ok {
			return nil, fmt.Errorf("unknown label %q", p.label)
		}
		off := target - p.from
		if off < -32768 || off > 32767 {
			return nil, fmt.Errorf("branch %q offset %d out of range", p.label, off)
		}
		binary.BigEndian.PutUint16(a.code[p.at:], uint16(int16(off)))
	}
	return a.code, nil
}

type methodSpec struct {
	name, desc string
	access     uint16
	maxStack   uint16
	maxLocals  uint16
	code       []byte
	exceptions []exEntry
}

type exEntry struct {
	start, end, handler, catch uint16
}

func buildClass(internalName string, thisClass, superClass, codeName uint16, cp *cpBuilder, methods []methodSpec) ([]byte, error) {
	// Intern names before freezing the constant pool.
	for i := range methods {
		_ = cp.Utf8(methods[i].name)
		_ = cp.Utf8(methods[i].desc)
	}
	_ = cp.Utf8("Code")
	var out []byte
	putU1 := func(v byte) { out = append(out, v) }
	putU2 := func(v uint16) {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], v)
		out = append(out, b[:]...)
	}
	putU4 := func(v uint32) {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], v)
		out = append(out, b[:]...)
	}
	putRaw := func(b []byte) { out = append(out, b...) }

	putU4(0xCAFEBABE)
	putU2(0)
	putU2(52) // Java 8
	putU2(uint16(len(cp.items) + 1))
	for _, it := range cp.items {
		putRaw(it)
	}
	putU2(0x0021) // public super
	putU2(thisClass)
	putU2(superClass)
	putU2(0) // interfaces
	putU2(0) // fields
	putU2(uint16(len(methods)))
	for _, m := range methods {
		putU2(m.access)
		putU2(cp.Utf8(m.name))
		putU2(cp.Utf8(m.desc))
		putU2(1) // Code attribute only
		putU2(codeName)
		// attribute_length = 2+2+4+code + 2 + 8*ex + 2
		attrLen := uint32(12 + len(m.code) + 8*len(m.exceptions))
		putU4(attrLen)
		putU2(m.maxStack)
		putU2(m.maxLocals)
		putU4(uint32(len(m.code)))
		putRaw(m.code)
		putU2(uint16(len(m.exceptions)))
		for _, e := range m.exceptions {
			putU2(e.start)
			putU2(e.end)
			putU2(e.handler)
			putU2(e.catch)
		}
		putU2(0) // code attributes
	}
	putU2(0) // class attributes
	_ = internalName
	_ = putU1
	return out, nil
}

func ctorCode(objectInit uint16) []byte {
	a := newAsm()
	a.emit(0x2a) // aload_0
	a.emit(0xb7, byte(objectInit>>8), byte(objectInit))
	a.emit(0xb1) // return
	code, _ := a.finish()
	return code
}

func loopRunCode(n int) ([]byte, error) {
	a := newAsm()
	a.emit(0x03) // iconst_0
	a.emit(0x3b) // istore_0 acc
	for i := 0; i < n; i++ {
		start := fmt.Sprintf("s%d", i)
		end := fmt.Sprintf("e%d", i)
		a.emit(0x03) // iconst_0
		a.emit(0x3c) // istore_1 i
		a.mark(start)
		a.emit(0x1b)       // iload_1
		a.emit(0x10, 0x0a) // bipush 10
		a.branch(0xa2, end)
		a.emit(0x1a) // iload_0
		a.emit(0x1b) // iload_1
		a.emit(0x60) // iadd
		a.emit(0x3b) // istore_0
		a.emit(0x84, 0x01, 0x01)
		a.branch(0xa7, start)
		a.mark(end)
	}
	a.emit(0x1a) // iload_0
	a.emit(0xac) // ireturn
	return a.finish()
}

func slotRunCode(n int) ([]byte, error) {
	if n < 2 {
		return nil, fmt.Errorf("slot family needs n>=2")
	}
	a := newAsm()
	for i := 0; i < n; i++ {
		a.emit(0x04) // iconst_1
		a.emit(0x36, byte(i))
	}
	a.emit(0x15, 0x00) // iload 0
	a.branch(0x99, "else")
	a.emit(0x15, 0x01) // iload 1
	a.emit(0x36, 0x00) // istore 0
	a.branch(0xa7, "merge")
	a.mark("else")
	a.emit(0x03)       // iconst_0
	a.emit(0x36, 0x00) // istore 0
	a.mark("merge")
	a.emit(0x15, 0x00) // iload 0
	a.emit(0xac)
	return a.finish()
}

func deepExprCode(n int) ([]byte, error) {
	if n < 1 {
		return nil, fmt.Errorf("deep expr n>=1")
	}
	a := newAsm()
	a.emit(0x04) // iconst_1
	for i := 0; i < n; i++ {
		a.emit(0x04) // iconst_1
		a.emit(0x60) // iadd
	}
	a.emit(0xac)
	return a.finish()
}

func handlerDenseCode(n int) (code []byte, ex []exEntry, err error) {
	a := newAsm()
	start := a.pc()
	a.emit(0x03) // iconst_0
	a.emit(0x3b) // istore_0
	a.emit(0x1a) // iload_0
	a.emit(0x57) // pop
	end := a.pc()
	a.branch(0xa7, "done")
	handlers := make([]int, n)
	for i := 0; i < n; i++ {
		handlers[i] = a.pc()
		a.emit(0x4c) // astore_1
		a.branch(0xa7, "done")
	}
	a.mark("done")
	a.emit(0xb1)
	code, err = a.finish()
	if err != nil {
		return nil, nil, err
	}
	ex = make([]exEntry, n)
	for i := 0; i < n; i++ {
		ex[i] = exEntry{start: uint16(start), end: uint16(end), handler: uint16(handlers[i])}
	}
	return code, ex, nil
}

func handlerSparseCode(n int) (code []byte, ex []exEntry, err error) {
	a := newAsm()
	ex = make([]exEntry, n)
	for i := 0; i < n; i++ {
		start := a.pc()
		a.emit(0x03)
		a.emit(0x3b)
		end := a.pc()
		after := fmt.Sprintf("after%d", i)
		a.branch(0xa7, after)
		handler := a.pc()
		a.emit(0x4c) // astore_1
		a.mark(after)
		ex[i] = exEntry{start: uint16(start), end: uint16(end), handler: uint16(handler)}
	}
	a.emit(0xb1)
	code, err = a.finish()
	return code, ex, err
}

func makeSyntheticClass(internalName, runDesc string, run methodSpec) ([]byte, error) {
	cp := newCP()
	thisClass := cp.Class(internalName)
	superClass := cp.Class("java/lang/Object")
	objectInit := cp.Methodref("java/lang/Object", "<init>", "()V")
	codeName := cp.Utf8("Code")
	excClass := cp.Class("java/lang/Exception")
	for i := range run.exceptions {
		if run.exceptions[i].catch == 0 {
			run.exceptions[i].catch = excClass
		}
	}
	_ = runDesc
	methods := []methodSpec{
		{
			name:      "<init>",
			desc:      "()V",
			access:    0x0001,
			maxStack:  1,
			maxLocals: 1,
			code:      ctorCode(objectInit),
		},
		run,
	}
	return buildClass(internalName, thisClass, superClass, codeName, cp, methods)
}

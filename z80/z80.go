package z80

import (
	"errors"
	"fmt"

	"github.com/marcopeereboom/toyz80/bus"
)

var (
	ErrHalt               = errors.New("halt")
	ErrInvalidInstruction = errors.New("invalid instruction")
)

type CPUMode int

const (
	ModeZ80 CPUMode = iota
	Mode8080
)

const (
	carry     uint16 = 1 << 0 // C Carry Flag
	addsub    uint16 = 1 << 1 // N Add/Subtract
	parity    uint16 = 1 << 2 // P/V Parity/Overflow Flag
	unused    uint16 = 1 << 3 // X Not Used
	halfCarry uint16 = 1 << 4 // H Half Carry Flag
	unused2   uint16 = 1 << 5 // X Not Used
	zero      uint16 = 1 << 6 // Z Zero Flag
	sign      uint16 = 1 << 7 // S Sign Flag
)

// z80 describes a z80/8080 CPU.
type z80 struct {
	af uint16 // A & Flags
	bc uint16 // B & C
	de uint16 // D & E
	hl uint16 // H & L
	ix uint16 // index register X
	iy uint16 // index register Y

	pc uint16 // program counter
	sp uint16 // stack pointer

	bus *bus.Bus // System bus

	totalCycles uint64 // Total cycles used

	mode CPUMode // Mode CPU is running
}

// DumpRegisters returns a dump of all registers.
func (z *z80) DumpRegisters() string {
	flags := ""
	if z.af&sign == sign {
		flags += "S"
	} else {
		flags += "-"
	}
	if z.af&zero == zero {
		flags += "Z"
	} else {
		flags += "-"
	}
	if z.af&unused2 == unused2 {
		flags += "1"
	} else {
		flags += "0"
	}
	if z.af&halfCarry == halfCarry {
		flags += "H"
	} else {
		flags += "-"
	}
	if z.af&unused == unused {
		flags += "1"
	} else {
		flags += "0"
	}
	if z.af&parity == parity {
		flags += "P"
	} else {
		flags += "-"
	}
	if z.af&addsub == addsub {
		flags += "N"
	} else {
		flags += "-"
	}
	if z.af&carry == carry {
		flags += "C"
	} else {
		flags += "-"
	}
	return fmt.Sprintf("af $%04x bc $%04x de $%04x hl $%04x ix $%04x "+
		"iy $%04x pc $%04x sp $%04x f %v ", uint16(z.af),
		uint16(z.bc), uint16(z.de), uint16(z.hl), uint16(z.ix),
		uint16(z.iy), uint16(z.pc), uint16(z.sp), flags)
}

// New returns a cold reset Z80 CPU struct.
func New(mode CPUMode, bus *bus.Bus) (*z80, error) {
	return &z80{
		mode: mode,
		bus:  bus,
	}, nil
}

// Reset resets the CPU.  If cold is true then memory is zeroed.
func (z *z80) Reset(cold bool) {
	if cold {
		// toss memory.
		//z.bus.Reset()
	}

	//The program counter is reset to 0000h
	z.pc = 0

	//Interrupt mode 0.

	//Interrupt are dissabled.

	//The register I = 00h
	//The register R = 00h
}

func (z *z80) evalZ(src byte) {
	if src == 0x00 {
		z.af |= zero
	} else {
		z.af &^= zero
	}
}

func (z *z80) evalS(src byte) {
	if src&0x80 == 0x80 {
		z.af |= sign
	} else {
		z.af &^= sign
	}
}

func (z *z80) evalH(src, increment byte) {
	h := (src&0x0f + increment&0x0f) & 0x10
	if h != 0 {
		z.af |= halfCarry
	} else {
		z.af &^= halfCarry
	}
}

func (z *z80) genericPostInstruction(o *opcode) {
	z.totalCycles += o.noCycles
	z.pc += uint16(o.noBytes)
}

// inc8H adds 1 to the low byte of *p and sets the flags.
func (z *z80) inc8L(p *uint16) {
	oldB := byte(*p & 0x00ff)
	newB := oldB + 1
	*p = uint16(newB) | *p&0xff00
	//Condition Bits Affected
	//S is set if result is negative; otherwise, it is reset.
	//Z is set if result is 0; otherwise, it is reset.
	//H is set if carry from bit 3; otherwise, it is reset.
	//P/V is set if r was 7Fh before operation; otherwise, it is reset.
	//N is reset.
	//C is not affected
	z.evalS(newB)
	z.evalZ(newB)
	if oldB == 0x7f {
		z.af |= parity
	} else {
		z.af &^= parity
	}
	z.evalH(oldB, 1)
	z.af &^= addsub
}

// inc8H adds 1 to the high byte of *p and sets the flags.
func (z *z80) inc8H(p *uint16) {
	oldB := byte(*p >> 8)
	newB := oldB + 1
	*p = uint16(newB)<<8 | *p&0x00ff
	//Condition Bits Affected
	//S is set if result is negative; otherwise, it is reset.
	//Z is set if result is 0; otherwise, it is reset.
	//H is set if carry from bit 3; otherwise, it is reset.
	//P/V is set if r was 7Fh before operation; otherwise, it is reset.
	//N is reset.
	//C is not affected
	z.evalS(newB)
	z.evalZ(newB)
	if oldB == 0x7f {
		z.af |= parity
	} else {
		z.af &^= parity
	}
	z.evalH(oldB, 1)
	z.af &^= addsub
}

// Step executes the instruction as pointed at by PC.
func (z *z80) Step() error {
	// This is a little messy because of multi-byte opcodes.  We assume the
	// opcode is one byte and we change in the switch statement to contain
	// the *actual* opcode in order to calculate cycles etc.
	//
	// Instructions that return early shall handle pc and noCycles.
	//
	// Reference used: http://zilog.com/docs/z80/um0080.pdf
	opc := z.bus.Read(z.pc)
	opcodeStruct := &opcodes[opc]
	pi := z.genericPostInstruction

	// Move all code into opcodes array for extra vroom vroom.
	switch opc {
	case 0x00: // nop
		// nothing to do
	case 0x01: // ld bc,nn
		z.bc = uint16(z.bus.Read(z.pc+1)) | uint16(z.bus.Read(z.pc+2))<<8
	case 0x02: // ld (bc),a
		z.bus.Write(z.bc, byte(z.af>>8))
	case 0x03: //inc bc
		z.bc += 1
	case 0x04: //inc b
		z.inc8H(&z.bc)
	case 0x06: // ld b,n
		z.bc = uint16(z.bus.Read(z.pc+1))<<8 | z.bc&0x00ff
	case 0x0a: // ld a,(bc)
		z.af = uint16(z.bus.Read(z.bc))<<8 | z.af&0x00ff
	case 0x0b: //dec bc
		z.bc -= 1
	case 0x0e: // ld c,n
		z.bc = uint16(z.bus.Read(z.pc+1)) | z.bc&0xff00
	case 0x11: // ld de,nn
		z.de = uint16(z.bus.Read(z.pc+1)) | uint16(z.bus.Read(z.pc+2))<<8
	case 0x12: // ld (de),a
		z.bus.Write(z.de, byte(z.af>>8))
	case 0x13: //inc de
		z.de += 1
	case 0x16: // ld d,n
		z.de = uint16(z.bus.Read(z.pc+1))<<8 | z.de&0x00ff
	case 0x18: // jr d
		z.pc = z.pc + 2 + uint16(int8(z.bus.Read(z.pc+1)))
		z.totalCycles += opcodeStruct.noCycles
		return nil
	case 0x1a: // ld a,(de)
		z.af = uint16(z.bus.Read(z.de))<<8 | z.af&0x00ff
	case 0x1b: //dec de
		z.de -= 1
	case 0x1e: // ld e,n
		z.de = uint16(z.bus.Read(z.pc+1)) | z.de&0xff00
	case 0x21: // ld hl,nn
		z.hl = uint16(z.bus.Read(z.pc+1)) | uint16(z.bus.Read(z.pc+2))<<8
	case 0x23: //inc hl
		z.hl += 1
	case 0x26: // ld h,n
		z.hl = uint16(z.bus.Read(z.pc+1))<<8 | z.hl&0x00ff
	case 0x28: // jr z,d
		if z.af&zero == zero {
			z.pc = z.pc + 2 + uint16(int8(z.bus.Read(z.pc+1)))
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
		// XXX make this generic
		z.totalCycles += 7
	case 0x2b: //dec hl
		z.hl -= 1
	case 0x2e: // ld l,n
		z.hl = uint16(z.bus.Read(z.pc+1)) | z.hl&0xff00
	case 0x2f: // cpl
		z.af = z.af&0x00ff | ^z.af&0xff00

		// Condition Bits Affected
		// S is not affected.
		// Z is not affected.
		// H is set.
		// P/V is not affected.
		// N is set.
		// C is not affected.
		z.af |= halfCarry
		z.af |= addsub
	case 0x31: // ld sp,nn
		z.sp = uint16(z.bus.Read(z.pc+1)) | uint16(z.bus.Read(z.pc+2))<<8
	case 0x32: // ld (nn),a
		z.bus.Write(uint16(z.bus.Read(z.pc+1))|
			uint16(z.bus.Read(z.pc+2))<<8, byte(z.af>>8))
	case 0x33: //inc sp
		z.sp += 1
	case 0x36: // ld (hl),n
		z.bus.Write(z.hl, z.bus.Read(z.pc+1))
	case 0x37: // scf
		// Condition Bits Affected
		// S is not affected.
		// Z is not affected.
		// H is reset.
		// P/V is not affected.
		// N is reset.
		// C is set.
		z.af &^= halfCarry
		z.af &^= addsub
		z.af |= carry
	case 0x3f: // ccf
		// Condition Bits Affected
		// S is not affected.
		// Z is not affected.
		// H, previous carry is copied.
		// P/V is not affected.
		// N is reset.
		// C is set if CY was 0 before operation; otherwise, it is reset.
		if z.af&carry == carry {
			z.af |= halfCarry
			z.af &^= carry // invert carry
		} else {
			z.af &^= halfCarry
			z.af |= carry // invert carry
		}
		z.af &^= addsub
	case 0x3a: // ld a,(nn)
		z.af = uint16(z.bus.Read(uint16(z.bus.Read(z.pc+1))|
			uint16(z.bus.Read(z.pc+2))<<8)) << 8
	case 0x3b: //dec sp
		z.sp -= 1
	case 0x3c: // inc a
		z.inc8L(&z.af)
	case 0x3e: // ld a,n
		z.af = uint16(z.bus.Read(z.pc+1))<<8 | z.af&0x00ff
	case 0x40: //ld b,b
		// nothing to do
	case 0x41: //ld b,c
		z.bc = z.bc&0x00ff | z.bc<<8
	case 0x42: //ld b,d
		z.bc = z.bc&0x00ff | z.de&0xff00
	case 0x43: //ld b,e
		z.bc = z.bc&0x00ff | z.de<<8
	case 0x44: //ld b,h
		z.bc = z.bc&0x00ff | z.hl&0xff00
	case 0x45: //ld b,l
		z.bc = z.bc&0x00ff | z.hl<<8
	case 0x46: //ld b,(hl)
		z.bc = uint16(z.bus.Read(z.hl))<<8 | z.bc&0x00ff
	case 0x47: // ld b,a
		z.bc = z.af&0xff00 | z.bc&0x00ff
	case 0x48: // ld c,b
		z.bc = z.bc>>8 | z.bc&0xff00
	case 0x49: //ld c,c
		// nothing to do
	case 0x4a: // ld c,d
		z.bc = z.bc&0xff00 | z.de>>8
	case 0x4b: // ld c,e
		z.bc = z.bc&0xff00 | z.de&0x00ff
	case 0x4c: // ld c,h
		z.bc = z.bc&0xff00 | z.hl>>8
	case 0x4d: // ld c,l
		z.bc = z.bc&0xff00 | z.hl&0x00ff
	case 0x4e: //ld c,(hl)
		z.bc = uint16(z.bus.Read(z.hl)) | z.bc&0xff00
	case 0x4f: // ld c,a
		z.bc = z.bc&0xff00 | z.af>>8
	case 0x50: // ld d,b
		z.de = z.bc&0xff00 | z.de&0x00ff
	case 0x51: // ld d,c
		z.de = z.bc<<8 | z.de&0x00ff
	case 0x52: // ld d,d
		// nothing to do
	case 0x53: // ld d,e
		z.de = z.de&0x00ff | z.de<<8
	case 0x54: // ld d,h
		z.de = z.hl&0xff00 | z.de&0x00ff
	case 0x55: // ld d,l
		z.de = z.hl<<8 | z.de&0x00ff
	case 0x56: // ld d,(hl)
		z.de = uint16(z.bus.Read(z.hl))<<8 | z.de&0x00ff
	case 0x57: // ld d,a
		z.de = z.af&0xff00 | z.de&0x00ff
	case 0x58: // ld e,b
		z.de = z.bc>>8 | z.de&0xff00
	case 0x59: // ld e,c
		z.de = z.bc&0x00ff | z.de&0xff00
	case 0x5a: // ld e,d
		z.de = z.de>>8 | z.de&0xff00
	case 0x5b: // ld e,e
		// nothing to do
	case 0x5c: // ld e,h
		z.de = z.hl>>8 | z.de&0xff00
	case 0x5d: // ld e,l
		z.de = z.hl&0x00ff | z.de&0xff00
	case 0x5e: // ld e,(hl)
		z.de = uint16(z.bus.Read(z.hl)) | z.de&0xff00
	case 0x5f: // ld e,a
		z.de = z.af>>8 | z.de&0xff00
	case 0x60: // ld h,b
		z.hl = z.hl&0x00ff | z.bc&0xff00
	case 0x61: // ld h,c
		z.hl = z.hl&0x00ff | z.bc<<8
	case 0x62: // ld h,d
		z.hl = z.hl&0x00ff | z.de&0xff00
	case 0x63: // ld h,e
		z.hl = z.hl&0x00ff | z.de<<8
	case 0x64: // ld h,h
		// nothing to do
	case 0x65: // ld h,l
		z.hl = z.hl&0x00ff | z.hl<<8
	case 0x66: // ld h,(hl)
		z.hl = uint16(z.bus.Read(z.hl))<<8 | z.hl&0x00ff
	case 0x67: // ld h,a
		z.hl = z.hl&0x00ff | z.af&0xff00
	case 0x68: // ld l,b
		z.hl = z.hl&0xff00 | z.bc>>8
	case 0x69: // ld l,c
		z.hl = z.hl&0xff00 | z.bc&0x00ff
	case 0x6a: // ld l,d
		z.hl = z.hl&0xff00 | z.de>>8
	case 0x6b: // ld l,e
		z.hl = z.hl&0xff00 | z.de&0x00ff
	case 0x6c: // ld l,h
		z.hl = z.hl>>8 | z.hl&0xff00
	case 0x6d: // ld l,l
		// nothing to do
	case 0x6e: // ld l,(hl)
		z.hl = uint16(z.bus.Read(z.hl)) | z.hl&0xff00
	case 0x6f: // ld l,a
		z.hl = z.hl&0xff00 | z.af>>8
	case 0x70: // ld (hl),b
		z.bus.Write(z.hl, byte(z.bc>>8))
	case 0x71: // ld (hl),c
		z.bus.Write(z.hl, byte(z.bc))
	case 0x72: // ld (hl),d
		z.bus.Write(z.hl, byte(z.de>>8))
	case 0x73: // ld (hl),e
		z.bus.Write(z.hl, byte(z.de))
	case 0x74: // ld (hl),h
		z.bus.Write(z.hl, byte(z.hl>>8))
	case 0x75: // ld (hl),l
		z.bus.Write(z.hl, byte(z.hl))
	case 0x76: // halt
		z.totalCycles += opcodeStruct.noCycles
		return ErrHalt
	case 0x77: // ld (hl),a
		z.bus.Write(z.hl, byte(z.af>>8))
	case 0x78: // ld a,b
		z.af = z.af&0x00ff | z.bc&0xff00
	case 0x79: // ld a,c
		z.af = z.af&0x00ff | z.bc<<8
	case 0x7a: // ld a,d
		z.af = z.af&0x00ff | z.de&0xff00
	case 0x7b: // ld a,e
		z.af = z.af&0x00ff | z.de<<8
	case 0x7c: // ld a,h
		z.af = z.af&0x00ff | z.hl&0xff00
	case 0x7d: // ld a,l
		z.af = z.af&0x00ff | z.hl<<8
	case 0x7e: // ld a,(hl)
		z.af = uint16(z.bus.Read(z.hl))<<8 | z.af&0x00ff
	case 0x7f: // ld a,a
		// nothing to do
	case 0xa7: // and a
		a := byte(z.af >> 8)
		z.evalS(a)
		z.evalZ(a)
		z.af |= halfCarry
		// PV
		z.af &^= addsub
		z.af &^= carry
	case 0xbf: // cp a
		// XXX this is all kinds of broken XXX
		// XXX the flags are not obvious from the doco at all.
		// Condition Bits Affected
		// S is set if result is negative; otherwise, it is reset.
		// Z is set if result is 0; otherwise, it is reset.
		// H is set if borrow from bit 4; otherwise, it is reset.
		// P/V is set if overflow; otherwise, it is reset.
		// N is set.
		// C is set if borrow; otherwise, it is reset.
		z.evalS(byte(z.af >> 8))
		z.evalZ(byte(z.af >> 8))
		// XXX figure out H
		// XXX figure out P
		z.af |= addsub
		// XXX figure out C

	case 0xc1: // pop bc
		z.bc = uint16(z.bus.Read(z.sp)) | z.bc&0xff00
		z.sp++
		z.bc = uint16(z.bus.Read(z.sp))<<8 | z.bc&0x00ff
		z.sp++
	case 0xc2: // jp nz,nn
		if z.af&zero == 0 {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xc3: // jp nn
		z.pc = uint16(z.bus.Read(z.pc+1)) |
			uint16(z.bus.Read(z.pc+2))<<8
		z.totalCycles += opcodeStruct.noCycles
		return nil
	case 0xc5: // push bc
		z.sp--
		z.bus.Write(z.sp, byte(z.bc>>8))
		z.sp--
		z.bus.Write(z.sp, byte(z.bc))
	case 0xc8: // ret z
		if z.af&zero == zero {
			pc := uint16(z.bus.Read(z.sp))
			z.sp++
			pc = uint16(z.bus.Read(z.sp))<<8 | pc&0x00ff
			z.sp++
			z.totalCycles += 11 // XXX
			z.pc = pc
			return nil
		}
	case 0xc9: // ret
		pc := uint16(z.bus.Read(z.sp))
		z.sp++
		pc = uint16(z.bus.Read(z.sp))<<8 | pc&0x00ff
		z.sp++
		z.totalCycles += opcodeStruct.noCycles
		z.pc = pc
		return nil
	case 0xca: // jp z,nn
		if z.af&zero == zero {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xcd: //call nn
		retPC := z.pc + opcodeStruct.noBytes
		z.sp--
		z.bus.Write(z.sp, byte(retPC>>8))
		z.sp--
		z.bus.Write(z.sp, byte(retPC))

		z.pc = uint16(z.bus.Read(z.pc+1)) |
			uint16(z.bus.Read(z.pc+2))<<8

		z.totalCycles += opcodeStruct.noCycles
		return nil
	case 0xd1: // pop de
		z.de = uint16(z.bus.Read(z.sp)) | z.de&0xff00
		z.sp++
		z.de = uint16(z.bus.Read(z.sp))<<8 | z.de&0x00ff
		z.sp++
	case 0xd2: // jp nc,nn
		if z.af&carry == 0 {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xd3: // out (n), a
		z.bus.IOWrite(z.bus.Read(z.pc+1), byte(z.af>>8))
	case 0xd5: // push de
		z.sp--
		z.bus.Write(z.sp, byte(z.de>>8))
		z.sp--
		z.bus.Write(z.sp, byte(z.de))
	case 0xda: // jp c,nn
		if z.af&carry == carry {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xdb: // in a,(n)
		z.af = uint16(z.bus.IORead(z.bus.Read(z.pc+1)))<<8 | z.af&0x00ff
	case 0xdd: // z80 only
		byte2 := z.bus.Read(z.pc + 1)
		opcodeStruct = &opcodesDD[byte2]
		switch byte2 {
		case 0x23: // inc ix
			z.ix += 1
		case 0x2b: // dec ix
			z.ix -= 1
		case 0xe1: // pop ix
			z.ix = uint16(z.bus.Read(z.sp)) | z.ix&0xff00
			z.sp++
			z.ix = uint16(z.bus.Read(z.sp))<<8 | z.ix&0x00ff
			z.sp++
		case 0xe5: // push ix
			z.sp--
			z.bus.Write(z.sp, byte(z.ix>>8))
			z.sp--
			z.bus.Write(z.sp, byte(z.ix))
		default:
			return ErrInvalidInstruction
		}
	case 0xe1: // pop hl
		z.hl = uint16(z.bus.Read(z.sp)) | z.hl&0xff00
		z.sp++
		z.hl = uint16(z.bus.Read(z.sp))<<8 | z.hl&0x00ff
		z.sp++
	case 0xe2: // jp po,nn
		if z.af&parity == 0 {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xe5: // push hl
		z.sp--
		z.bus.Write(z.sp, byte(z.hl>>8))
		z.sp--
		z.bus.Write(z.sp, byte(z.hl))
	case 0xe6:
		a := z.bus.Read(z.pc+1) & byte(z.af>>8)
		z.af = uint16(a)<<8 | z.af&0x00ff
		z.evalS(a)
		z.evalZ(a)
		z.af |= halfCarry
		// PV
		z.af &^= addsub
		z.af &^= carry
	case 0xe9: // jp (hl)
		// but we don't dereference, *sigh* zilog
		z.pc = z.hl
		z.totalCycles += opcodeStruct.noCycles
		return nil
	case 0xea: // jp pe,nn
		if z.af&parity == parity {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xeb: // ex de,hl
		t := z.hl
		z.hl = z.de
		z.de = t
	case 0xed: // z80 only
		byte2 := z.bus.Read(z.pc + 1)
		opcodeStruct = &opcodesED[byte2]
		switch byte2 {
		case 0x44: // neg
			// Condition Bits Affected
			// S is set if result is negative; otherwise, it is reset.
			// Z is set if result is 0; otherwise, it is reset.
			// H is set if borrow from bit 4; otherwise, it is reset.
			// P/V is set if Accumulator was 80h before operation; otherwise, it is reset.
			// N is set.
			// C is set if Accumulator was not 00h before operation; otherwise, it is reset.
			oldA := byte(z.af & 0xff00 >> 8)
			newA := 0 - oldA
			z.af = uint16(newA) << 8
			z.evalS(newA)
			z.evalZ(newA)
			// XXX figure out how to handle the H flag
			if oldA == 0x80 {
				z.af |= parity
			} else {
				z.af &^= parity
			}
			z.af |= addsub
			if oldA != 0x00 {
				z.af |= carry
			} else {
				z.af &^= carry
			}
		default:
			return ErrInvalidInstruction
		}
	case 0xf1: // pop af
		z.af = uint16(z.bus.Read(z.sp)) | z.af&0xff00
		z.sp++
		z.af = uint16(z.bus.Read(z.sp))<<8 | z.af&0x00ff
		z.sp++
	case 0xf2: // jp p,nn
		if z.af&sign == 0 {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xf5: // push af
		z.sp--
		z.bus.Write(z.sp, byte(z.af>>8))
		z.sp--
		z.bus.Write(z.sp, byte(z.af))
	case 0xfa: // jp m,nn
		if z.af&sign == sign {
			z.pc = uint16(z.bus.Read(z.pc+1)) |
				uint16(z.bus.Read(z.pc+2))<<8
			z.totalCycles += opcodeStruct.noCycles
			return nil
		}
	case 0xfd: // z80 only
		byte2 := z.bus.Read(z.pc + 1)
		opcodeStruct = &opcodesFD[byte2]
		switch byte2 {
		case 0x23: // inc iy
			z.iy += 1
		case 0x2b: // dec iy
			z.iy -= 1
		case 0xe1: // pop iy
			z.iy = uint16(z.bus.Read(z.sp)) | z.iy&0xff00
			z.sp++
			z.iy = uint16(z.bus.Read(z.sp))<<8 | z.iy&0x00ff
			z.sp++
		case 0xe5: // push iy
			z.sp--
			z.bus.Write(z.sp, byte(z.iy>>8))
			z.sp--
			z.bus.Write(z.sp, byte(z.iy))
		default:
			return ErrInvalidInstruction
		}
	default:
		//fmt.Printf("opcode %x\n", opcode)
		//return ErrInvalidInstruction
		// XXX make this a generic ErrInvalidInstruction
		return fmt.Errorf("invalid instruction: 0x%02x @ 0x%04x", opc, z.pc)
	}

	pi(opcodeStruct)

	return nil
}

// Disassemble disassembles the instruction at the provided address and also
// returns the number of bytes consumed.
func (z *z80) Disassemble(address uint16) (string, int) {
	opc, dst, src, noBytes := z.DisassembleComponents(address)

	if dst != "" && src != "" {
		src = "," + src
	}
	dst += src

	s := fmt.Sprintf("%-6v%-4v", opc, dst)

	return s, noBytes
}

// DisassembleComponents disassmbles the instruction at the provided address
// and returns all compnonts of the instruction (opcode, destination, source).
func (z *z80) DisassembleComponents(address uint16) (opc string, dst string, src string, noBytes int) {
	o := &opcodes[z.bus.Read(address)]
	if o.multiByte {
		switch z.bus.Read(address) {
		case 0xdd:
			o = &opcodesDD[z.bus.Read(address+1)]
		case 0xed:
			o = &opcodesED[z.bus.Read(address+1)]
		case 0xfd:
			o = &opcodesFD[z.bus.Read(address+1)]
		}
	}
	switch o.dst {
	case condition:
		dst = o.dstR[z.mode]
	case displacement:
		dst = fmt.Sprintf("$%04x", address+2+
			uint16(int8(z.bus.Read(address+1))))
	case registerIndirect:
		if z.mode == Mode8080 {
			dst = fmt.Sprintf("%v", o.dstR[z.mode])
		} else {
			dst = fmt.Sprintf("(%v)", o.dstR[z.mode])
		}
	case immediate:
		dst = fmt.Sprintf("$%02x", z.bus.Read(address+1))
	case immediateExtended:
		dst = fmt.Sprintf("$%04x", uint16(z.bus.Read(address+1))|
			uint16(z.bus.Read(address+2))<<8)
	case register:
		dst = o.dstR[z.mode]
	case indirect:
		dst = fmt.Sprintf("($%02x)", z.bus.Read(address+1))
	}

	switch o.src {
	case displacement:
		src = fmt.Sprintf("$%04x", address+2+
			uint16(int8(z.bus.Read(address+1))))
	case registerIndirect:
		if z.mode == Mode8080 {
			src = fmt.Sprintf("%v", o.srcR[z.mode])
		} else {
			src = fmt.Sprintf("(%v)", o.srcR[z.mode])
		}
	case extended:
		src = fmt.Sprintf("($%04x)", uint16(z.bus.Read(address+1))|
			uint16(z.bus.Read(address+2))<<8)
	case immediate:
		src = fmt.Sprintf("$%02x", z.bus.Read(address+1))
	case immediateExtended:
		src = fmt.Sprintf("$%04x", uint16(z.bus.Read(address+1))|
			uint16(z.bus.Read(address+2))<<8)
	case register:
		src = o.srcR[z.mode]
	case indirect:
		src = fmt.Sprintf("($%02x)", z.bus.Read(address+1))
	}

	noBytes = int(o.noBytes)
	if len(o.mnemonic) == 0 {
		opc = "INVALID"
		noBytes = 1
	} else {
		opc = o.mnemonic[z.mode]
	}

	return
}

func (z *z80) Trace() ([]string, []string, error) {
	trace := make([]string, 0, 1024)
	registers := make([]string, 0, 1024)

	for {
		s, _ := z.Disassemble(z.pc)
		trace = append(trace, fmt.Sprintf("%04x: %v", z.pc, s))
		//fmt.Printf("%04x: %v\n", z.pc, s)
		err := z.Step()
		registers = append(registers, z.DumpRegisters())
		//fmt.Printf("\t%v\n", z.DumpRegisters())
		if err != nil {
			return trace, registers, err
		}
	}
	return trace, registers, nil
}

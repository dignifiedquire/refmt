package json

import (
	"fmt"
	"io"
	"strconv"

	. "github.com/polydawn/refmt/tok"
)

func NewEncoder(wr io.Writer) *Encoder {
	return &Encoder{
		wr:    wr,
		stack: make([]phase, 0, 10),
	}
}

func (d *Encoder) Reset() {
	d.stack = d.stack[0:0]
	d.current = phase_anyExpectValue
	d.some = false
}

/*
	A json.Encoder is a TokenSink implementation that emits json bytes.
*/
type Encoder struct {
	wr io.Writer

	// Stack, tracking how many array and map opens are outstanding.
	// (Values are only 'phase_mapExpectKeyOrEnd' and 'phase_arrExpectValueOrEnd'.)
	stack   []phase
	current phase // shortcut to value at end of stack
	some    bool  // set to true after first value in any context; use to append commas.

	// Spare memory, for use in operations on leaf nodes (e.g. temp space for an int serialization).
	scratch [64]byte
}

type phase int

const (
	phase_anyExpectValue phase = iota
	phase_mapExpectKeyOrEnd
	phase_mapExpectValue
	phase_arrExpectValueOrEnd
)

func (d *Encoder) Step(tok *Token) (done bool, err error) {
	switch d.current {
	case phase_anyExpectValue:
		switch tok.Type {
		case TMapOpen:
			d.pushPhase(phase_mapExpectKeyOrEnd)
			d.wr.Write(wordMapOpen)
			return false, nil
		case TArrOpen:
			d.pushPhase(phase_arrExpectValueOrEnd)
			d.wr.Write(wordArrOpen)
			return false, nil
		case TMapClose:
			return true, fmt.Errorf("unexpected mapClose; expected start of value")
		case TArrClose:
			return true, fmt.Errorf("unexpected arrClose; expected start of value")
		default:
			// It's a value; handle it.
			d.flushValue(tok)
			return true, nil
		}
	case phase_mapExpectKeyOrEnd:
		switch tok.Type {
		case TMapOpen:
			return true, fmt.Errorf("unexpected mapOpen; expected start of key or end of map")
		case TArrOpen:
			return true, fmt.Errorf("unexpected arrOpen; expected start of key or end of map")
		case TMapClose:
			d.wr.Write(wordMapClose)
			return d.popPhase()
		case TArrClose:
			return true, fmt.Errorf("unexpected arrClose; expected start of key or end of map")
		default:
			// It's a key.  It'd better be a string.
			switch tok.Type {
			case TString:
				d.entrySep()
				d.emitString(tok.Str)
				d.wr.Write(wordColon)
				d.current = phase_mapExpectValue
				return false, nil
			default:
				return true, fmt.Errorf("unexpected token of type %T; expected map key", *tok)
			}
		}
	case phase_mapExpectValue:
		switch tok.Type {
		case TMapOpen:
			d.pushPhase(phase_mapExpectKeyOrEnd)
			d.wr.Write(wordMapOpen)
			return false, nil
		case TArrOpen:
			d.pushPhase(phase_arrExpectValueOrEnd)
			d.wr.Write(wordArrOpen)
			return false, nil
		case TMapClose:
			return true, fmt.Errorf("unexpected mapClose; expected start of value")
		case TArrClose:
			return true, fmt.Errorf("unexpected arrClose; expected start of value")
		default:
			// It's a value; handle it.
			d.flushValue(tok)
			d.current = phase_mapExpectKeyOrEnd
			return false, nil
		}
	case phase_arrExpectValueOrEnd:
		switch tok.Type {
		case TMapOpen:
			d.entrySep()
			d.pushPhase(phase_mapExpectKeyOrEnd)
			d.wr.Write(wordMapOpen)
			return false, nil
		case TArrOpen:
			d.entrySep()
			d.pushPhase(phase_arrExpectValueOrEnd)
			d.wr.Write(wordArrOpen)
			return false, nil
		case TMapClose:
			return true, fmt.Errorf("unexpected mapClose; expected start of value or end of array")
		case TArrClose:
			d.wr.Write(wordArrClose)
			return d.popPhase()
		default:
			// It's a value; handle it.
			d.entrySep()
			d.flushValue(tok)
			return false, nil
		}
	default:
		panic("Unreachable")
	}
}

func (d *Encoder) pushPhase(p phase) {
	d.current = p
	d.stack = append(d.stack, d.current)
	d.some = false
}

// Pop a phase from the stack; return 'true' if stack now empty.
func (d *Encoder) popPhase() (bool, error) {
	n := len(d.stack) - 1
	if n == 0 {
		return true, nil
	}
	if n < 0 { // the state machines are supposed to have already errored better
		panic("jsonEncoder stack overpopped")
	}
	d.current = d.stack[n-1]
	d.stack = d.stack[0:n]
	d.some = true
	return false, nil
}

// Emit an entry separater (comma), unless we're at the start of an object.
// Mark that we *do* have some content, regardless, so next time will need a sep.
func (d *Encoder) entrySep() {
	if d.some {
		d.wr.Write(wordComma)
	}
	d.some = true
}

func (d *Encoder) flushValue(tok *Token) {
	switch tok.Type {
	case TString:
		d.emitString(tok.Str)
	case TBool:
		switch tok.Bool {
		case true:
			d.wr.Write(wordTrue)
		case false:
			d.wr.Write(wordFalse)
		}
	case TInt:
		b := strconv.AppendInt(d.scratch[:0], tok.Int, 10)
		d.wr.Write(b)
	case TNull:
		d.wr.Write(wordNull)
	default:
		panic(fmt.Errorf("TODO finish more jsonEncoder primitives support: unhandled token %s", tok))
	}
}

func (d *Encoder) writeByte(b byte) {
	d.scratch[0] = b
	d.wr.Write(d.scratch[0:1])
}

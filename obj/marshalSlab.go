package obj

import (
	"fmt"
	"reflect"

	"github.com/polydawn/refmt/obj/atlas"
	. "github.com/polydawn/refmt/tok"
)

/*
	A lovely mechanism to stash marshalMachine objects pre-allocated and avoid mallocs.
	Works together with the Atlas: the Atlas says what kind of machinery is needed;
	the marshalSlab "allocates" it and returns it upon your request.
*/
type marshalSlab struct {
	atlas atlas.Atlas
	rows  []marshalSlabRow
}

type marshalSlabRow struct {
	ptrDerefDelegateMarshalMachine
	marshalMachinePrimitive
	marshalMachineWildcard
	marshalMachineMapWildcard
	marshalMachineSliceWildcard
	marshalMachineStructAtlas
	marshalMachineTransform

	errThunkMarshalMachine
}

// A thunk value that can be used to trigger `isNil` paths.
// (Substituting an 'invalid' kind reflect.Value with this is an easy way
// to emit a null without needing any additional special cases or error handling.)
var nil_rv reflect.Value = reflect.Zero(reflect.PtrTo(reflect.TypeOf(0)))

/*
	Return a reference to a machine from the slab.
	*You must release() when done.*

	Errors -- including "no info in Atlas for this type" -- are expressed by
	returning a machine that is a constantly-erroring thunk.
*/
func (slab *marshalSlab) requisitionMachine(rt reflect.Type) MarshalMachine {
	// Acquire a row.
	off := len(slab.rows)
	slab.grow()
	row := &slab.rows[off]

	// Indirect pointers as necessary.
	//  Keep count of how many times we do this; we'll use this again at the end.
	peelCount := 0
	for rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
		peelCount++
	}

	// Figure out what machinery to use at heart.
	mach := _yieldMarshalMachinePtr(row, slab.atlas, rt)
	// If nil answer, we had no match: yield an error thunk.
	if mach == nil {
		mach := &row.errThunkMarshalMachine
		mach.err = fmt.Errorf("no machine found")
		return mach
	}

	// If no indirection steps, return;
	//  otherwise wrap it in the ptrDeref machine and return that.
	if peelCount == 0 {
		return mach
	}
	row.ptrDerefDelegateMarshalMachine.MarshalMachine = mach
	row.ptrDerefDelegateMarshalMachine.peelCount = peelCount
	row.ptrDerefDelegateMarshalMachine.isNil = false
	return &row.ptrDerefDelegateMarshalMachine
}

func _yieldMarshalMachinePtr(row *marshalSlabRow, atl atlas.Atlas, rt reflect.Type) MarshalMachine {
	rtid := reflect.ValueOf(rt).Pointer()

	// Check primitives first; cheapest (and unoverridable).
	switch rtid {
	case rtid_bool,
		rtid_string,
		rtid_int, rtid_int8, rtid_int16, rtid_int32, rtid_int64,
		rtid_uint, rtid_uint8, rtid_uint16, rtid_uint32, rtid_uint64, rtid_uintptr,
		rtid_float32, rtid_float64,
		rtid_bytes:
		row.marshalMachinePrimitive.kind = rt.Kind()
		return &row.marshalMachinePrimitive
	}

	// Consult atlas second.
	if entry, ok := atl.Get(rtid); ok {
		// Switch across which of the union of configurations is applicable.
		switch {
		case entry.MarshalTransformFunc != nil:
			// Return a machine that calls the func(s), then later a real machine.
			// The entry.MarshalTransformTargetType is used to do a recursive lookup.
			// We can't just call the func here because we're still working off typeinfo
			// and don't have a real value to transform until later.
			row.marshalMachineTransform.trFunc = entry.MarshalTransformFunc
			// Pick delegate without growing stack.  (This currently means recursive transform won't fly.)
			row.marshalMachineTransform.delegate = _yieldMarshalMachinePtr(row, atl, entry.MarshalTransformTargetType)
			return &row.marshalMachineTransform
		case entry.StructMap != nil:
			row.marshalMachineStructAtlas.cfg = entry.StructMap
			return &row.marshalMachineStructAtlas
		default:
			panic("invalid atlas entry")
		}
	}

	// If no specific behavior found, use default behavior based on kind.
	switch rt.Kind() {
	case reflect.Bool,
		reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		row.marshalMachinePrimitive.kind = rt.Kind()
		return &row.marshalMachinePrimitive
	case reflect.Slice:
		// un-typedef'd byte slices were handled already, but a typedef'd one still gets gets treated like a special kind:
		if rt.Elem().Kind() == reflect.Uint8 {
			row.marshalMachinePrimitive.kind = rt.Kind()
			return &row.marshalMachinePrimitive
		}
		return &row.marshalMachineSliceWildcard
	case reflect.Array:
		return &row.marshalMachineSliceWildcard.marshalMachineArrayWildcard
	case reflect.Map:
		return &row.marshalMachineMapWildcard
	case reflect.Struct:
		panic("todo") // this is where we auto-detect, if you've enabled that; otherwise, raise error for missing configuration / unexpected type.
	case reflect.Interface:
		return &row.marshalMachineWildcard
	case reflect.Func:
		panic(fmt.Errorf("functions cannot be marshalled!"))
	case reflect.Ptr:
		panic(fmt.Errorf("unreachable: ptrs must already be resolved"))
	default:
		panic(fmt.Errorf("excursion %s", rt.Kind()))
	}
}

func (s *marshalSlab) grow() {
	s.rows = append(s.rows, marshalSlabRow{})
}

func (s *marshalSlab) release() {
	s.rows = s.rows[0 : len(s.rows)-1]
}

type errThunkMarshalMachine struct {
	err error
}

func (m *errThunkMarshalMachine) Reset(_ *marshalSlab, _ reflect.Value, _ reflect.Type) error {
	return m.err
}
func (m *errThunkMarshalMachine) Step(d *Marshaller, s *marshalSlab, tok *Token) (done bool, err error) {
	return true, m.err
}

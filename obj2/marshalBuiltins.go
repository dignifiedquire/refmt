package obj

import (
	"reflect"

	. "github.com/polydawn/go-xlate/tok"
)

type ptrDerefDelegateMarshalMachine struct {
	MarshalMachine
	peelCount int

	isNil bool
}

func (mach *ptrDerefDelegateMarshalMachine) Reset(slab *marshalSlab, rv reflect.Value, rt reflect.Type) error {
	mach.isNil = false
	for i := 0; i < mach.peelCount; i++ {
		rv = rv.Elem()
		if rv.IsNil() {
			mach.isNil = true
			return nil
		}
	}
	return mach.MarshalMachine.Reset(slab, rv, rt) // REVIEW: this rt should be peeled by here.  do we... ignore the arg and cache it at mach conf time?
}
func (mach *ptrDerefDelegateMarshalMachine) Step(driver *MarshalDriver, slab *marshalSlab, tok *Token) (done bool, err error) {
	if mach.isNil {
		tok.Type = TNull
		return true, nil
	}
	return mach.MarshalMachine.Step(driver, slab, tok)
}

type marshalMachinePrimitive struct {
	kind reflect.Kind

	rv reflect.Value
}

func (mach *marshalMachinePrimitive) Reset(_ *marshalSlab, rv reflect.Value, _ reflect.Type) error {
	mach.rv = rv
	return nil
}
func (mach *marshalMachinePrimitive) Step(_ *MarshalDriver, _ *marshalSlab, tok *Token) (done bool, err error) {
	switch mach.kind {
	case reflect.Bool:
		tok.Type = TBool
		tok.Bool = mach.rv.Bool()
		return true, nil
	case reflect.String:
		tok.Type = TString
		tok.Str = mach.rv.String()
		return true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		tok.Type = TInt
		tok.Int = mach.rv.Int()
		return true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		tok.Type = TUint
		tok.Uint = mach.rv.Uint()
		return true, nil
	case reflect.Float32, reflect.Float64:
		tok.Type = TFloat64
		tok.Float64 = mach.rv.Float()
		return true, nil
	case reflect.Slice: // implicitly bytes; no other slices are "primitve"
		tok.Type = TBytes
		tok.Bytes = mach.rv.Bytes()
		return true, nil
	default:
		panic("unhandled")
	}
}

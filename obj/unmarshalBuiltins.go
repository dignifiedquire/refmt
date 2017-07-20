package obj

import (
	"fmt"
	"reflect"

	. "github.com/polydawn/refmt/tok"
)

type ptrDerefDelegateUnmarshalMachine struct {
	UnmarshalMachine     // a delegate machine, already set for us by the slab
	peelCount        int // how deep the pointers go, already discoverd by the slab

	ptr_rv    reflect.Value // the top ptr, which we will use if setting to nil, or if we have to recursively make ptrs for `**X` types.
	firstStep bool
}

func (mach *ptrDerefDelegateUnmarshalMachine) Reset(slab *unmarshalSlab, rv reflect.Value, _ reflect.Type) error {
	mach.ptr_rv = rv
	mach.firstStep = true
	// we defer reseting the delegate machine until later, in case we get a nil, which can save a lot of time.
	return nil
}
func (mach *ptrDerefDelegateUnmarshalMachine) Step(driver *Unmarshaller, slab *unmarshalSlab, tok *Token) (done bool, err error) {
	// If nil: easy road.  Nil the ptr.
	if tok.Type == TNull {
		mach.ptr_rv.Set(reflect.Zero(mach.ptr_rv.Type()))
		return true, nil
	}
	// If real value: on the first step we have to do initializations.
	if mach.firstStep {
		mach.firstStep = false
		// Walk the pointers: if some already exist, we accept them unmodified;
		//  if any are nil, make a new one, and recursively.
		rv := mach.ptr_rv
		for i := 0; i < mach.peelCount; i++ {
			if rv.IsNil() {
				rv.Set(reflect.New(rv.Type().Elem()))
				rv = rv.Elem()
			} else {
				rv = rv.Elem()
			}
		}
		if err := mach.UnmarshalMachine.Reset(slab, rv, rv.Type()); err != nil {
			return true, err
		}
	}
	// The remainder of the time: it's just delegation.
	return mach.UnmarshalMachine.Step(driver, slab, tok)
}

type unmarshalMachinePrimitive struct {
	kind reflect.Kind

	rv reflect.Value
}

func (mach *unmarshalMachinePrimitive) Reset(_ *unmarshalSlab, rv reflect.Value, _ reflect.Type) error {
	mach.rv = rv
	return nil
}
func (mach *unmarshalMachinePrimitive) Step(_ *Unmarshaller, _ *unmarshalSlab, tok *Token) (done bool, err error) {
	switch mach.kind {
	case reflect.Bool:
		switch tok.Type {
		case TBool:
			mach.rv.SetBool(tok.Bool)
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.String:
		switch tok.Type {
		case TString:
			mach.rv.SetString(tok.Str)
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch tok.Type {
		case TInt:
			mach.rv.SetInt(tok.Int)
			return true, nil
		case TUint:
			mach.rv.SetInt(int64(tok.Uint)) // todo: overflow check
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		switch tok.Type {
		case TInt:
			if tok.Int >= 0 {
				mach.rv.SetUint(uint64(tok.Int))
				return true, nil
			}
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		case TUint:
			mach.rv.SetUint(tok.Uint)
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.Float32, reflect.Float64:
		switch tok.Type {
		case TFloat64:
			mach.rv.SetFloat(tok.Float64)
			return true, nil
		case TInt:
			mach.rv.SetFloat(float64(tok.Int))
			return true, nil
		case TUint:
			mach.rv.SetFloat(float64(tok.Uint))
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.Slice: // implicitly bytes; no other slices are "primitve"
		switch tok.Type {
		case TBytes:
			mach.rv.SetBytes(tok.Bytes)
			return true, nil
		default:
			return true, ErrUnmarshalTypeCantFit{*tok, mach.rv}
		}
	case reflect.Interface:
		switch tok.Type {
		case TString:
			mach.rv.Set(reflect.ValueOf(tok.Str))
		case TBytes:
			mach.rv.Set(reflect.ValueOf(tok.Bytes))
		case TBool:
			mach.rv.Set(reflect.ValueOf(tok.Bool))
		case TInt:
			mach.rv.Set(reflect.ValueOf(tok.Int))
		case TUint:
			mach.rv.Set(reflect.ValueOf(tok.Uint))
		case TFloat64:
			mach.rv.Set(reflect.ValueOf(tok.Float64))
		case TNull:
			mach.rv.Set(reflect.ValueOf(nil))
		default: // any of the other token types should not have been routed here to begin with.
			panic(fmt.Errorf("unhandled: %v", mach.kind))
		}
		return true, nil
	default:
		panic(fmt.Errorf("unhandled: %v", mach.kind))
	}
}

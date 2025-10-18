package common

import (
	"fmt"
	"reflect"
)

// SetDefaultStr return inval if it is not "", otherwise return defval
func SetDefaultStr(inval, defval string) string {
	if inval == "" {
		return defval
	}
	return inval
}

// SetDefaultGeneric return inval if it is not nil, otherwise return defVal
func SetDefaultGeneric[T any](inval *T, defVal T) *T {
	if inval != nil {
		return inval
	}
	r := new(T)
	*r = defVal
	return r
}

// FillNilPointers copies pointer fields from src into dst when dst's pointer fields are nil.
// dst must be a pointer to a struct; src must be a struct of the same concrete type.
//
// Notes:
// - Only exported (settable) fields are touched.
// - If src field is a pointer and non-nil, the pointer value is copied (both dst and src will point to the same underlying value).
// - If src field is a non-pointer value assignable to the pointer element type, a new pointer is allocated and its value copied.
func FillNilPointers(dst any, src any) error {

	srcVal := reflect.ValueOf(src)
	if !srcVal.IsValid() {
		return fmt.Errorf("src value is not valid")
	}
	if srcVal.Kind() == reflect.Interface {
		srcVal = srcVal.Elem()
	}
	dstVal := reflect.ValueOf(dst)
	if !dstVal.IsValid() {
		return fmt.Errorf("dst value is not valid")
	}
	if dstVal.Kind() == reflect.Interface {
		dstVal = dstVal.Elem()
	}
	if dstVal.Kind() != reflect.Ptr || dstVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("dst must be pointer to struct")
	}

	if srcVal.Kind() == reflect.Pointer {
		srcVal = srcVal.Elem()
	}
	if srcVal.Kind() != reflect.Struct {
		return fmt.Errorf("src must be a struct, but got a %v, %v", srcVal.Type().String(), srcVal.Kind())
	}
	if srcVal.Type() != dstVal.Elem().Type() {
		return fmt.Errorf("src type %s doesn't match dst type %s", srcVal.Type(), dstVal.Elem().Type())
	}

	fillNilPointersValue(dstVal.Elem(), srcVal)
	return nil
}

// recursive helper: dstStruct and srcStruct are reflect.Values of kind Struct (addressable for dst).
func fillNilPointersValue(dstStruct, srcStruct reflect.Value) {
	for i := 0; i < dstStruct.NumField(); i++ {
		dstField := dstStruct.Field(i)
		srcField := srcStruct.Field(i)

		// skip unexported / unsettable fields
		if !dstField.CanSet() {
			continue
		}

		switch dstField.Kind() {
		case reflect.Ptr:
			// Only fill when dst pointer is nil and src has a non-zero value
			if dstField.IsNil() && srcField.IsValid() && !srcField.IsZero() {
				// If src is a pointer and assignable to dst type, set directly
				if srcField.Kind() == reflect.Ptr && srcField.Type().AssignableTo(dstField.Type()) {
					if !srcField.IsNil() {
						dstField.Set(srcField)
					}
				} else {
					// src is not a pointer: if its type is assignable to pointer elem, allocate and copy
					elemType := dstField.Type().Elem()
					if srcField.Type().AssignableTo(elemType) {
						newPtr := reflect.New(elemType)
						newPtr.Elem().Set(srcField)
						dstField.Set(newPtr)
					}
				}
			}
		case reflect.Struct:
			// Recurse into nested struct
			fillNilPointersValue(dstField, srcField)
		// other kinds ignored
		default:
			// do nothing
		}
		// For other possible pointer-like container types (slices/maps/interfaces) we intentionally do nothing
	}
}

// DO NOT Use this function for API defaulting, because it will create new pointer for all nil fields,
// which is against of purpose of nil pointer field (meaning it has no user input)
// NewStructPointerFields create a new value for all pointer top level fields of s
// s must be a pointer to struct
func NewStructPointerFields(s any) error {
	val := reflect.ValueOf(s)
	if val.Kind() != reflect.Pointer {
		return fmt.Errorf("not a pointer")
	}
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("not a pointer to struct")
	}
	for i := 0; i < val.NumField(); i++ {
		if val.Field(i).Kind() == reflect.Pointer {
			newval := reflect.New(val.Field(i).Type().Elem())
			val.Field(i).Set(newval)
		}
	}
	return nil
}

// AssignPointerVal create a new pointer for inp, and assign val to it
func AssignPointerVal[T any](inp **T, val T) {
	*inp = new(T)
	**inp = val

}

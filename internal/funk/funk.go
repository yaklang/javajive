// Package funk is a minimal, self-contained reflection-based reimplementation of
// the small subset of the original go-funk API used by the ported Java tooling.
//
// It preserves the original reflection-based signatures (returning interface{})
// so callers compile unchanged after only their import path is rewritten. It has
// no third-party dependencies.
package funk

import (
	"fmt"
	"reflect"
	"strings"
)

// Filter iterates over a slice/array, returning a new slice (of the same element
// type) of all elements for which predicate returns true. predicate must be a
// func(T) bool where T is the element type.
func Filter(arr interface{}, predicate interface{}) interface{} {
	arrValue := reflect.ValueOf(arr)
	if k := arrValue.Kind(); k != reflect.Slice && k != reflect.Array {
		panic("funk.Filter: first parameter must be a slice or array")
	}
	funcValue := reflect.ValueOf(predicate)
	if funcValue.Kind() != reflect.Func {
		panic("funk.Filter: second argument must be a function")
	}

	resultSlice := reflect.MakeSlice(reflect.SliceOf(arrValue.Type().Elem()), 0, 0)
	for i := 0; i < arrValue.Len(); i++ {
		elem := arrValue.Index(i)
		if funcValue.Call([]reflect.Value{elem})[0].Bool() {
			resultSlice = reflect.Append(resultSlice, elem)
		}
	}
	return resultSlice.Interface()
}

// Map applies mapFunc to every element of arr and returns a new slice of the
// result element type. mapFunc must be a func(T) U.
func Map(arr interface{}, mapFunc interface{}) interface{} {
	arrValue := reflect.ValueOf(arr)
	if k := arrValue.Kind(); k != reflect.Slice && k != reflect.Array {
		panic("funk.Map: first parameter must be a slice or array")
	}
	funcValue := reflect.ValueOf(mapFunc)
	funcType := funcValue.Type()
	if funcValue.Kind() != reflect.Func || funcType.NumOut() != 1 {
		panic("funk.Map: second argument must be a function returning a single value")
	}

	resultSlice := reflect.MakeSlice(reflect.SliceOf(funcType.Out(0)), 0, arrValue.Len())
	for i := 0; i < arrValue.Len(); i++ {
		out := funcValue.Call([]reflect.Value{arrValue.Index(i)})[0]
		resultSlice = reflect.Append(resultSlice, out)
	}
	return resultSlice.Interface()
}

// ForEach iterates over a slice/array (or map), calling predicate for each
// element. predicate must be a func(T) for slices/arrays, or func(V) / func(K, V)
// for maps.
func ForEach(arr interface{}, predicate interface{}) {
	arrValue := reflect.ValueOf(arr)
	funcValue := reflect.ValueOf(predicate)
	if funcValue.Kind() != reflect.Func {
		panic("funk.ForEach: second argument must be a function")
	}
	funcType := funcValue.Type()

	switch arrValue.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < arrValue.Len(); i++ {
			funcValue.Call([]reflect.Value{arrValue.Index(i)})
		}
	case reflect.Map:
		for _, key := range arrValue.MapKeys() {
			if funcType.NumIn() == 2 {
				funcValue.Call([]reflect.Value{key, arrValue.MapIndex(key)})
			} else {
				funcValue.Call([]reflect.Value{arrValue.MapIndex(key)})
			}
		}
	default:
		panic("funk.ForEach: first parameter must be a slice, array or map")
	}
}

// Reverse returns a reversed copy of a slice, or a reversed string.
func Reverse(in interface{}) interface{} {
	value := reflect.ValueOf(in)
	if value.Kind() == reflect.String {
		runes := []rune(value.String())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	}
	if k := value.Kind(); k != reflect.Slice && k != reflect.Array {
		panic("funk.Reverse: parameter must be a slice, array or string")
	}
	length := value.Len()
	resultSlice := reflect.MakeSlice(reflect.SliceOf(value.Type().Elem()), length, length)
	for i := 0; i < length; i++ {
		resultSlice.Index(length - 1 - i).Set(value.Index(i))
	}
	return resultSlice.Interface()
}

// Contains reports whether elem exists in the collection in (slice/array/map/string).
func Contains(in interface{}, elem interface{}) bool {
	inValue := reflect.ValueOf(in)
	switch inValue.Kind() {
	case reflect.String:
		return strings.Contains(inValue.String(), fmt.Sprint(elem))
	case reflect.Map:
		for _, key := range inValue.MapKeys() {
			if reflect.DeepEqual(key.Interface(), elem) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < inValue.Len(); i++ {
			if reflect.DeepEqual(inValue.Index(i).Interface(), elem) {
				return true
			}
		}
	default:
		panic(fmt.Sprintf("funk.Contains: type %s is not supported", inValue.Type().String()))
	}
	return false
}

// Copyright (c) 2013 Kelsey Hightower. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package envconfig

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	strcase "github.com/stoewer/go-strcase"
)

// ErrInvalidSpecification indicates that a specification is of the wrong type.
var ErrInvalidSpecification = errors.New("specification must be a struct pointer")

// A ParseError occurs when an environment variable cannot be converted to
// the type required by a struct field during assignment.
type ParseError struct {
	KeyName   string
	KeyNames  []string
	FieldName string
	TypeName  string
	Value     string
	Err       error
}

// Decoder has the same semantics as Setter, but takes higher precedence.
// It is provided for historical compatibility.
type Decoder interface {
	Decode(value string) error
}

// Setter is implemented by types can self-deserialize values.
// Any type that implements flag.Value also implements Setter.
type Setter interface {
	Set(value string) error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("envconfig.Process: assigning %[1]s to %[2]s: converting '%[3]s' to type %[4]s. details: %[5]s", e.KeyName, e.FieldName, e.Value, e.TypeName, e.Err)
}

// varInfo maintains information about the configuration variable
type varInfo struct {
	Name  string
	Keys  []string
	Field reflect.Value
	Tags  reflect.StructTag
}

// GatherInfo gathers information about the specified struct
func gatherInfo(prefixes []string, spec interface{}, delim string) ([]varInfo, error) {
	s := reflect.ValueOf(spec)

	if s.Kind() != reflect.Ptr {
		return nil, ErrInvalidSpecification
	}
	s = s.Elem()
	if s.Kind() != reflect.Struct {
		return nil, ErrInvalidSpecification
	}
	typeOfSpec := s.Type()

	// over allocate an info array, we will extend if needed later
	infos := make([]varInfo, 0, s.NumField())
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		ftype := typeOfSpec.Field(i)
		if !f.CanSet() || isTrue(ftype.Tag.Get("envconfig_ignored")) {
			continue
		}

		for f.Kind() == reflect.Ptr {
			if f.IsNil() {
				if f.Type().Elem().Kind() != reflect.Struct {
					// nil pointer to a non-struct: leave it alone
					break
				}
				// nil pointer to struct: create a zero instance
				f.Set(reflect.New(f.Type().Elem()))
			}
			f = f.Elem()
		}

		var alt []string
		if altStr, ok := ftype.Tag.Lookup("envconfig"); ok {
			alt = strings.Split(altStr, "=")
		}

		// Capture information about the config variable
		info := varInfo{
			Name:  ftype.Name,
			Field: f,
			Tags:  ftype.Tag,
		}

		var key string
		// Best effort to un-pick camel casing as separate words
		if isTrue(ftype.Tag.Get("envconfig_snake")) {
			key = strcase.SnakeCase(info.Name)
		} else {
			// Default to the field name as the env var name (will be upcased)
			key = info.Name
		}

		var keys []string
		if len(alt) > 0 {
			if prefixed, ok := ftype.Tag.Lookup("envconfig_alt_prefixed"); !ok || isTrue(prefixed) {
				for _, prefix := range prefixes {
					for _, key := range alt {
						keys = append(keys, prefix+key)
					}
				}
			}
			keys = append(keys, alt...)
		} else {
			if len(prefixes) == 0 {
				keys = []string{key}
			} else {
				for _, prefix := range prefixes {
					keys = append(keys, prefix+key)
				}
			}
		}
		if doUpcase, ok := ftype.Tag.Lookup("envconfig_upcase"); !ok || isTrue(doUpcase) {
			for i, _ := range keys {
				keys[i] = strings.ToUpper(keys[i])
			}
		}
		info.Keys = keys
		infos = append(infos, info)

		if f.Kind() == reflect.Struct {
			// honor Decode if present
			if decoderFrom(f) == nil && setterFrom(f) == nil && textUnmarshaler(f) == nil && binaryUnmarshaler(f) == nil {
				innerPrefixes := prefixes
				if !ftype.Anonymous {
					innerPrefixes = []string{}
					for _, key := range keys {
						innerPrefixes = append(innerPrefixes, composePrefix(key, delim))
					}
				}

				embeddedPtr := f.Addr().Interface()
				embeddedInfos, err := gatherInfo(innerPrefixes, embeddedPtr, delim)
				if err != nil {
					return nil, err
				}
				infos = append(infos[:len(infos)-1], embeddedInfos...)

				continue
			}
		}
	}
	return infos, nil
}

// composePrefix gets prefix and delim concatenated and returns the result
// if prefix is not empty
func composePrefix(prefix, delim string) string {
	if prefix != "" {
		return prefix + delim
	} else {
		return ""
	}
}

// stringizeKeys builds a string that represents an array of keys
func stringizeKeys(keys []string) string {
	if len(keys) == 0 {
		return "???"
	} else if len(keys) == 1 {
		return keys[0]
	} else if len(keys) == 2 {
		return fmt.Sprintf("%s (%s)", keys[0], keys[1])
	} else {
		return fmt.Sprintf("%s (%s or %s)", keys[0], strings.Join(keys[1:len(keys)-1], ", "), keys[len(keys)-1])
	}
}

// Process populates the specified struct based on environment variables
func Process(prefixes []string, spec interface{}, delim string) error {
	infos, err := gatherInfo(prefixes, spec, delim)

	for _, info := range infos {

		// `os.Getenv` cannot differentiate between an explicitly set empty value
		// and an unset value. `os.LookupEnv` is preferred to `syscall.Getenv`,
		// but it is only available in go1.5 or newer. We're using Go build tags
		// here to use os.LookupEnv for >=go1.5
		var value string
		var ok bool
		for _, key := range info.Keys {
			value, ok = lookupEnv(key)
			if ok {
				break
			}
		}

		def := info.Tags.Get("envconfig_default")
		if def != "" && !ok {
			value = def
		}

		req := info.Tags.Get("envconfig_required")
		if !ok && def == "" {
			if isTrue(req) {
				return fmt.Errorf("required key %s missing value", stringizeKeys(info.Keys))
			}
			continue
		}

		err = processField(value, info.Field)
		if err != nil {
			return &ParseError{
				KeyName:   info.Keys[0],
				KeyNames:  info.Keys,
				FieldName: info.Name,
				TypeName:  info.Field.Type().String(),
				Value:     value,
				Err:       err,
			}
		}
	}

	return err
}

// MustProcess is the same as Process but panics if an error occurs
func MustProcess(prefixes []string, spec interface{}, delim string) {
	if err := Process(prefixes, spec, delim); err != nil {
		panic(err)
	}
}

func processField(value string, field reflect.Value) error {
	typ := field.Type()

	decoder := decoderFrom(field)
	if decoder != nil {
		return decoder.Decode(value)
	}
	// look for Set method if Decode not defined
	setter := setterFrom(field)
	if setter != nil {
		return setter.Set(value)
	}

	if t := textUnmarshaler(field); t != nil {
		return t.UnmarshalText([]byte(value))
	}

	if b := binaryUnmarshaler(field); b != nil {
		return b.UnmarshalBinary([]byte(value))
	}

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		if field.IsNil() {
			field.Set(reflect.New(typ))
		}
		field = field.Elem()
	}

	switch typ.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var (
			val int64
			err error
		)
		if field.Kind() == reflect.Int64 && typ.PkgPath() == "time" && typ.Name() == "Duration" {
			var d time.Duration
			d, err = time.ParseDuration(value)
			val = int64(d)
		} else {
			val, err = strconv.ParseInt(value, 0, typ.Bits())
		}
		if err != nil {
			return err
		}

		field.SetInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val, err := strconv.ParseUint(value, 0, typ.Bits())
		if err != nil {
			return err
		}
		field.SetUint(val)
	case reflect.Bool:
		val, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(val)
	case reflect.Float32, reflect.Float64:
		val, err := strconv.ParseFloat(value, typ.Bits())
		if err != nil {
			return err
		}
		field.SetFloat(val)
	case reflect.Slice:
		vals := strings.Split(value, ",")
		sl := reflect.MakeSlice(typ, len(vals), len(vals))
		for i, val := range vals {
			err := processField(val, sl.Index(i))
			if err != nil {
				return err
			}
		}
		field.Set(sl)
	case reflect.Map:
		mp := reflect.MakeMap(typ)
		if len(strings.TrimSpace(value)) != 0 {
			pairs := strings.Split(value, ",")
			for _, pair := range pairs {
				kvpair := strings.Split(pair, ":")
				if len(kvpair) != 2 {
					return fmt.Errorf("invalid map item: %q", pair)
				}
				k := reflect.New(typ.Key()).Elem()
				err := processField(kvpair[0], k)
				if err != nil {
					return err
				}
				v := reflect.New(typ.Elem()).Elem()
				err = processField(kvpair[1], v)
				if err != nil {
					return err
				}
				mp.SetMapIndex(k, v)
			}
		}
		field.Set(mp)
	}

	return nil
}

func interfaceFrom(field reflect.Value, fn func(interface{}, *bool)) {
	// it may be impossible for a struct field to fail this check
	if !field.CanInterface() {
		return
	}
	var ok bool
	fn(field.Interface(), &ok)
	if !ok && field.CanAddr() {
		fn(field.Addr().Interface(), &ok)
	}
}

func decoderFrom(field reflect.Value) (d Decoder) {
	interfaceFrom(field, func(v interface{}, ok *bool) { d, *ok = v.(Decoder) })
	return d
}

func setterFrom(field reflect.Value) (s Setter) {
	interfaceFrom(field, func(v interface{}, ok *bool) { s, *ok = v.(Setter) })
	return s
}

func textUnmarshaler(field reflect.Value) (t encoding.TextUnmarshaler) {
	interfaceFrom(field, func(v interface{}, ok *bool) { t, *ok = v.(encoding.TextUnmarshaler) })
	return t
}

func binaryUnmarshaler(field reflect.Value) (b encoding.BinaryUnmarshaler) {
	interfaceFrom(field, func(v interface{}, ok *bool) { b, *ok = v.(encoding.BinaryUnmarshaler) })
	return b
}

func isTrue(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}

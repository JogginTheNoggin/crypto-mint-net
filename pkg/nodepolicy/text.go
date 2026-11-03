package nodepolicy

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// checkText refuses a control character, a bidirectional override or bytes
// that are not UTF-8 in any text of a node policy: a registry, a peer, a
// template or job name, a target. None is valid in one, and a file is
// somebody else's: text in it reaches the terminal in a preview and in the
// node's log, where such a character could erase or fake a line (RED-053,
// D156). The error quotes the field and the value with Go escapes.
func checkText(p Policy) error {
	return walkText(reflect.ValueOf(p), "")
}

func walkText(v reflect.Value, path string) error {
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); !plainText(s) {
			return fmt.Errorf("%w: %s %q holds a control character or bytes that are not UTF-8", ErrInvalid, fieldName(path), s)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return walkText(v.Elem(), path)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag := f.Tag.Get("json"); tag == "-" {
				continue
			} else if tag != "" {
				if n, _, _ := strings.Cut(tag, ","); n != "" {
					name = n
				}
			}
			if err := walkText(v.Field(i), join(path, name)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil // bytes, not text
		}
		for i := 0; i < v.Len(); i++ {
			if err := walkText(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			k := it.Key()
			if k.Kind() == reflect.String && !plainText(k.String()) {
				return fmt.Errorf("%w: %s has a key %q with a control character or bytes that are not UTF-8", ErrInvalid, fieldName(path), k.String())
			}
			if err := walkText(it.Value(), fmt.Sprintf("%s[%v]", path, k)); err != nil {
				return err
			}
		}
	}
	return nil
}

// plainText reports whether s is UTF-8 without a control character, a
// bidirectional formatting character or a line or paragraph separator.
func plainText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func fieldName(path string) string {
	if path == "" {
		return "a value"
	}
	return path
}

package macos

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParsePlist decodes an XML property list into map[string]any, []any, string, int64, float64,
// bool, []byte and time.Time values.
func ParsePlist(data []byte) (any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "plist" {
			value, ok, err := next(decoder)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, errors.New("plist: empty")
			}
			return value, nil
		}
	}
}

// next decodes the following value; ok is false when the enclosing element ends instead.
func next(decoder *xml.Decoder) (value any, ok bool, err error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, false, fmt.Errorf("plist: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			value, err := element(decoder, t)
			return value, err == nil, err
		case xml.EndElement:
			return nil, false, nil
		}
	}
}

func element(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	text := func() (string, error) {
		var s string
		err := decoder.DecodeElement(&s, &start)
		return s, err
	}
	switch start.Name.Local {
	case "dict":
		dict := map[string]any{}
		for {
			token, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("plist: %w", err)
			}
			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local != "key" {
					return nil, fmt.Errorf("plist: <%s> where a key belongs", t.Name.Local)
				}
				var key string
				if err := decoder.DecodeElement(&key, &t); err != nil {
					return nil, err
				}
				value, ok, err := next(decoder)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("plist: key %q has no value", key)
				}
				dict[key] = value
			case xml.EndElement:
				return dict, nil
			}
		}
	case "array":
		array := []any{}
		for {
			value, ok, err := next(decoder)
			if err != nil {
				return nil, err
			}
			if !ok {
				return array, nil
			}
			array = append(array, value)
		}
	case "string":
		return text()
	case "integer":
		s, err := text()
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, nil
		}
		// IOKit stores some signed values (e.g. BatteryPower) as unsigned 64-bit, so a negative
		// reading arrives as a huge unsigned number whose bit pattern is the signed one.
		u, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("plist: integer %q", s)
		}
		return int64(u), nil
	case "real":
		s, err := text()
		if err != nil {
			return nil, err
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "true", "false":
		if err := decoder.Skip(); err != nil {
			return nil, err
		}
		return start.Name.Local == "true", nil
	case "data":
		s, err := text()
		if err != nil {
			return nil, err
		}
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	case "date":
		s, err := text()
		if err != nil {
			return nil, err
		}
		return time.Parse(time.RFC3339, strings.TrimSpace(s))
	}
	return nil, fmt.Errorf("plist: unknown element <%s>", start.Name.Local)
}

// Int reads an integer field; plist reals are truncated.
func Int(dict map[string]any, key string) (int64, bool) {
	switch v := dict[key].(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	}
	return 0, false
}

func Bool(dict map[string]any, key string) (bool, bool) {
	switch v := dict[key].(type) {
	case bool:
		return v, true
	case int64:
		return v != 0, true
	}
	return false, false
}

func String(dict map[string]any, key string) (string, bool) {
	v, ok := dict[key].(string)
	return v, ok
}

func Dict(dict map[string]any, key string) (map[string]any, bool) {
	v, ok := dict[key].(map[string]any)
	return v, ok
}

package macos

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
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

// EncodePlist writes Go values as an XML property list: maps with string keys, slices, strings,
// integers, floats, bools, []byte and time.Time.
func EncodePlist(value any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<plist version="1.0">`)
	if err := encode(&b, value); err != nil {
		return nil, err
	}
	b.WriteString("</plist>")
	return b.Bytes(), nil
}

func encode(b *bytes.Buffer, value any) error {
	escape := func(s string) {
		_ = xml.EscapeText(b, []byte(s))
	}
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("<dict>")
		for _, k := range keys {
			b.WriteString("<key>")
			escape(k)
			b.WriteString("</key>")
			if err := encode(b, v[k]); err != nil {
				return err
			}
		}
		b.WriteString("</dict>")
	case []any:
		b.WriteString("<array>")
		for _, item := range v {
			if err := encode(b, item); err != nil {
				return err
			}
		}
		b.WriteString("</array>")
	case []string:
		items := make([]any, len(v))
		for i, s := range v {
			items[i] = s
		}
		return encode(b, items)
	case string:
		b.WriteString("<string>")
		escape(v)
		b.WriteString("</string>")
	case int:
		fmt.Fprintf(b, "<integer>%d</integer>", v)
	case int64:
		fmt.Fprintf(b, "<integer>%d</integer>", v)
	case float64:
		fmt.Fprintf(b, "<real>%s</real>", strconv.FormatFloat(v, 'g', -1, 64))
	case bool:
		if v {
			b.WriteString("<true/>")
		} else {
			b.WriteString("<false/>")
		}
	case []byte:
		fmt.Fprintf(b, "<data>%s</data>", base64.StdEncoding.EncodeToString(v))
	case time.Time:
		fmt.Fprintf(b, "<date>%s</date>", v.UTC().Format(time.RFC3339))
	default:
		return fmt.Errorf("plist: can't encode %T", value)
	}
	return nil
}

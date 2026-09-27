package node

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf16"
)

type writeEncodingError struct {
	encoding string
	detail   string
}

func (e *writeEncodingError) Error() string {
	if e.detail != "" {
		return fmt.Sprintf("Invalid %s input: %s", e.encoding, e.detail)
	}
	return fmt.Sprintf("Unknown encoding: %s", e.encoding)
}

func encodeWriteString(value, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "utf8", "utf-8":
		return []byte(value), nil
	case "ascii":
		return lowByteString(value), nil
	case "latin1", "binary":
		return lowByteString(value), nil
	case "utf16le", "utf-16le", "ucs2", "ucs-2":
		runes := utf16.Encode([]rune(value))
		data := make([]byte, len(runes)*2)
		for i, r := range runes {
			data[i*2] = byte(r)
			data[i*2+1] = byte(r >> 8)
		}
		return data, nil
	case "base64", "base64url":
		data, err := decodeWriteBase64(value, strings.EqualFold(encoding, "base64url"))
		if err != nil {
			return nil, &writeEncodingError{encoding: encoding, detail: err.Error()}
		}
		return data, nil
	case "hex":
		data, _ := hex.DecodeString(value)
		return data, nil
	default:
		return nil, &writeEncodingError{encoding: encoding}
	}
}

func lowByteString(value string) []byte {
	units := utf16.Encode([]rune(value))
	data := make([]byte, len(units))
	for i, unit := range units {
		data[i] = byte(unit)
	}
	return data
}

func decodeWriteBase64(value string, urlSafe bool) ([]byte, error) {
	value = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		default:
			return r
		}
	}, value)

	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	if urlSafe {
		encodings = []*base64.Encoding{
			base64.RawURLEncoding,
			base64.URLEncoding,
			base64.RawStdEncoding,
			base64.StdEncoding,
		}
	}
	for _, encoding := range encodings {
		if data, err := encoding.DecodeString(value); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("invalid base64 data")
}

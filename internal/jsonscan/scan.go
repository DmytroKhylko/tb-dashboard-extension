// Package jsonscan locates values inside a JSON document by byte offset, so
// callers can edit a document without reordering keys or reformatting it.
package jsonscan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Span is a half-open byte range [Start, End) within a document.
type Span struct {
	Start int
	End   int
}

// Member is a key/value pair of a JSON object.
type Member struct {
	Key   string
	Value Span
}

var ErrInvalid = errors.New("invalid JSON")

// StringValue is a string literal used as a value (not an object key).
type StringValue struct {
	Span  Span
	Value string
}

// StringValues returns every string value in doc, in document order.
func StringValues(doc []byte) ([]StringValue, error) {
	if !json.Valid(doc) {
		return nil, ErrInvalid
	}
	var values []StringValue
	for i := 0; i < len(doc); i++ {
		if doc[i] != '"' {
			continue
		}
		end, err := scanString(doc, i)
		if err != nil {
			return nil, err
		}
		if next := skipSpace(doc, end); next >= len(doc) || doc[next] != ':' {
			var s string
			if err := json.Unmarshal(doc[i:end], &s); err != nil {
				return nil, fmt.Errorf("decode string at %d: %w", i, err)
			}
			values = append(values, StringValue{Span: Span{Start: i, End: end}, Value: s})
		}
		i = end - 1
	}
	return values, nil
}

// ObjectMembers returns the members of the object that starts at offset start.
func ObjectMembers(doc []byte, start int) ([]Member, error) {
	i := skipSpace(doc, start)
	if i >= len(doc) || doc[i] != '{' {
		return nil, fmt.Errorf("%w: expected object at %d", ErrInvalid, start)
	}
	var members []Member
	i = skipSpace(doc, i+1)
	if i < len(doc) && doc[i] == '}' {
		return members, nil
	}
	for {
		keyEnd, err := scanString(doc, i)
		if err != nil {
			return nil, err
		}
		var key string
		if err := json.Unmarshal(doc[i:keyEnd], &key); err != nil {
			return nil, fmt.Errorf("decode key at %d: %w", i, err)
		}
		i = skipSpace(doc, keyEnd)
		if i >= len(doc) || doc[i] != ':' {
			return nil, fmt.Errorf("%w: expected ':' at %d", ErrInvalid, i)
		}
		valueStart := skipSpace(doc, i+1)
		valueEnd, err := SkipValue(doc, valueStart)
		if err != nil {
			return nil, err
		}
		members = append(members, Member{Key: key, Value: Span{Start: valueStart, End: valueEnd}})

		i = skipSpace(doc, valueEnd)
		if i >= len(doc) {
			return nil, fmt.Errorf("%w: unterminated object", ErrInvalid)
		}
		switch doc[i] {
		case ',':
			i = skipSpace(doc, i+1)
		case '}':
			return members, nil
		default:
			return nil, fmt.Errorf("%w: unexpected %q at %d", ErrInvalid, doc[i], i)
		}
	}
}

// ArrayElements returns the spans of the elements of the array that starts at offset start.
func ArrayElements(doc []byte, start int) ([]Span, error) {
	i := skipSpace(doc, start)
	if i >= len(doc) || doc[i] != '[' {
		return nil, fmt.Errorf("%w: expected array at %d", ErrInvalid, start)
	}
	var elements []Span
	i = skipSpace(doc, i+1)
	if i < len(doc) && doc[i] == ']' {
		return elements, nil
	}
	for {
		end, err := SkipValue(doc, i)
		if err != nil {
			return nil, err
		}
		elements = append(elements, Span{Start: i, End: end})

		i = skipSpace(doc, end)
		if i >= len(doc) {
			return nil, fmt.Errorf("%w: unterminated array", ErrInvalid)
		}
		switch doc[i] {
		case ',':
			i = skipSpace(doc, i+1)
		case ']':
			return elements, nil
		default:
			return nil, fmt.Errorf("%w: unexpected %q at %d", ErrInvalid, doc[i], i)
		}
	}
}

// SkipValue returns the offset just past the JSON value that starts at offset start.
func SkipValue(doc []byte, start int) (int, error) {
	i := skipSpace(doc, start)
	if i >= len(doc) {
		return 0, fmt.Errorf("%w: expected value at %d", ErrInvalid, start)
	}
	switch doc[i] {
	case '"':
		return scanString(doc, i)
	case '{', '[':
		depth := 0
		for ; i < len(doc); i++ {
			switch doc[i] {
			case '"':
				end, err := scanString(doc, i)
				if err != nil {
					return 0, err
				}
				i = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
		}
		return 0, fmt.Errorf("%w: unterminated container at %d", ErrInvalid, start)
	default:
		end := i
		for end < len(doc) && !bytes.ContainsRune([]byte(",}] \t\r\n"), rune(doc[end])) {
			end++
		}
		return end, nil
	}
}

// LineIndent returns the leading whitespace of the line that contains offset pos.
func LineIndent(doc []byte, pos int) []byte {
	lineStart := bytes.LastIndexByte(doc[:pos], '\n') + 1
	end := lineStart
	for end < pos && (doc[end] == ' ' || doc[end] == '\t') {
		end++
	}
	return doc[lineStart:end]
}

func scanString(doc []byte, start int) (int, error) {
	if start >= len(doc) || doc[start] != '"' {
		return 0, fmt.Errorf("%w: expected string at %d", ErrInvalid, start)
	}
	for i := start + 1; i < len(doc); i++ {
		switch doc[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("%w: unterminated string at %d", ErrInvalid, start)
}

func skipSpace(doc []byte, i int) int {
	for i < len(doc) {
		switch doc[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

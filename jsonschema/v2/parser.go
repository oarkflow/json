package v2

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"unsafe"
)

type JSONParser struct {
	data []byte
	pos  int
}

func (p *JSONParser) skipWhitespace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\n', '\t', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *JSONParser) parseValue() (any, error) {
	p.skipWhitespace()
	if p.pos >= len(p.data) {
		return nil, errors.New("unexpected end of input")
	}
	ch := p.data[p.pos]
	switch ch {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseString()
	case 't':
		return p.parseLiteral("true", true)
	case 'f':
		return p.parseLiteral("false", false)
	case 'n':
		return p.parseLiteral("null", nil)
	default:
		if ch == '-' || (ch >= '0' && ch <= '9') {
			return p.parseNumber()
		}
	}
	return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, p.pos)
}

func (p *JSONParser) parseLiteral(lit string, value any) (any, error) {
	end := p.pos + len(lit)
	if end > len(p.data) || string(p.data[p.pos:end]) != lit {
		return nil, fmt.Errorf("invalid literal at position %d", p.pos)
	}
	p.pos += len(lit)
	return value, nil
}

func (p *JSONParser) parseObject() (any, error) {
	obj := make(map[string]any)
	p.pos++
	p.skipWhitespace()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return obj, nil
	}
	for {
		p.skipWhitespace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, errors.New("expected string key in object")
		}
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		p.skipWhitespace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, errors.New("expected ':' after key in object")
		}
		p.pos++
		p.skipWhitespace()
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj[key] = value
		p.skipWhitespace()
		if p.pos < len(p.data) && p.data[p.pos] == '}' {
			p.pos++
			break
		}
		if p.pos < len(p.data) && p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		return nil, errors.New("expected ',' or '}' in object")
	}
	return obj, nil
}

func (p *JSONParser) parseArray() (any, error) {
	arr := []any{}
	p.pos++
	p.skipWhitespace()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return arr, nil
	}
	for {
		p.skipWhitespace()
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, value)
		p.skipWhitespace()
		if p.pos < len(p.data) && p.data[p.pos] == ']' {
			p.pos++
			break
		}
		if p.pos < len(p.data) && p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		return nil, errors.New("expected ',' or ']' in array")
	}
	return arr, nil
}

func (p *JSONParser) parseString() (string, error) {
	if p.data[p.pos] != '"' {
		return "", errors.New("expected '\"' at beginning of string")
	}
	p.pos++
	var result []rune
	for p.pos < len(p.data) {
		ch := p.data[p.pos]
		if ch == '"' {
			p.pos++
			return string(result), nil
		}
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.data) {
				return "", errors.New("unexpected end of input in string escape")
			}
			esc := p.data[p.pos]
			if esc == 'u' {
				if p.pos+4 >= len(p.data) {
					return "", errors.New("incomplete unicode escape")
				}
				hexStr := string(p.data[p.pos+1 : p.pos+5])
				code, err := strconv.ParseInt(hexStr, 16, 32)
				if err != nil {
					return "", fmt.Errorf("invalid unicode escape: %v", err)
				}
				result = append(result, rune(code))
				p.pos += 5
				continue
			}
			switch esc {
			case '"', '\\', '/':
				result = append(result, rune(esc))
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			default:
				return "", fmt.Errorf("invalid escape character '%c'", esc)
			}
			p.pos++
		} else {
			result = append(result, rune(ch))
			p.pos++
		}
	}
	return "", errors.New("unexpected end of string")
}

// NEW: Optimize parseNumber to reduce allocations.
func (p *JSONParser) parseNumber() (any, error) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
		p.pos++
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		for p.pos < len(p.data) && (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') {
			p.pos++
		}
	}
	numBytes := p.data[start:p.pos]
	// Use unsafe conversion to convert []byte to string without allocation.
	numStr := *(*string)(unsafe.Pointer(&numBytes))
	if i, err := strconv.ParseInt(numStr, 10, 64); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var jsonParserPool = sync.Pool{
	New: func() interface{} {
		return &JSONParser{}
	},
}

func ParseJSON(data []byte) (any, error) {
	parser := jsonParserPool.Get().(*JSONParser)
	parser.data = data
	parser.pos = 0
	ret, err := parser.parseValue()

	parser.data = nil
	jsonParserPool.Put(parser)
	return ret, err
}

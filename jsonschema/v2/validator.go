package v2

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/oarkflow/date"
)

var formatValidators = map[string]func(string) error{
	"email": func(value string) error {
		_, err := mail.ParseAddress(value)
		if err != nil {
			return fmt.Errorf("invalid email: %v", err)
		}
		return nil
	},
	"uri": func(value string) error {
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" {
			return fmt.Errorf("invalid URI")
		}
		return nil
	},
	"uri-reference": func(value string) error {
		_, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid URI reference: %v", err)
		}
		return nil
	},
	"date": func(value string) error {
		_, err := date.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid date: %v", err)
		}
		return nil
	},
	"date-time": func(value string) error {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("invalid date-time: %v", err)
		}
		return nil
	},
	"ipv4": func(value string) error {
		if net.ParseIP(value) == nil || strings.Contains(value, ":") {
			return fmt.Errorf("invalid IPv4 address")
		}
		return nil
	},
	"ipv6": func(value string) error {
		if net.ParseIP(value) == nil || !strings.Contains(value, ":") {
			return fmt.Errorf("invalid IPv6 address")
		}
		return nil
	},
	"hostname": func(value string) error {
		if len(value) == 0 || len(value) > 253 {
			return fmt.Errorf("invalid hostname length")
		}
		matched, err := regexp.MatchString(`^(?:(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}|localhost)$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid hostname")
		}
		return nil
	},
	"uuid": func(value string) error {
		matched, err := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid UUID")
		}
		return nil
	},
	"json-pointer": func(value string) error {
		if value != "" && !strings.HasPrefix(value, "/") {
			return fmt.Errorf("invalid JSON pointer")
		}
		return nil
	},
	"relative-json-pointer": func(value string) error {
		matched, err := regexp.MatchString(`^\d+(?:#(?:\/.*)?)?$`, value)
		if err != nil || !matched {
			return fmt.Errorf("invalid relative JSON pointer")
		}
		return nil
	},
}

func RegisterFormatValidator(name string, validator func(string) error) {
	formatValidators[name] = validator
}

func validateFormat(format, value string) error {
	if fn, ok := formatValidators[format]; ok {
		return fn(value)
	}
	return nil
}

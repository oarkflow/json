package jsonschema

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	hostname       string = `^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
	unescapedTilda        = `\~[^01]`
	endingTilda           = `\~$`
	schemePrefix          = `^[^\:]+\:`
	uriTemplate           = `\{[^\{\}\\]*\}`
)

var (
	hostnamePattern        = regexp.MustCompile(hostname)
	unescaptedTildaPattern = regexp.MustCompile(unescapedTilda)
	endingTildaPattern     = regexp.MustCompile(endingTilda)
	schemePrefixPattern    = regexp.MustCompile(schemePrefix)
	uriTemplatePattern     = regexp.MustCompile(uriTemplate)
	disallowedIdnChars     = map[string]bool{"\u0020": true, "\u002D": true, "\u00A2": true, "\u00A3": true, "\u00A4": true, "\u00A5": true, "\u034F": true, "\u0640": true, "\u07FA": true, "\u180B": true, "\u180C": true, "\u180D": true, "\u200B": true, "\u2060": true, "\u2104": true, "\u2108": true, "\u2114": true, "\u2117": true, "\u2118": true, "\u211E": true, "\u211F": true, "\u2123": true, "\u2125": true, "\u2282": true, "\u2283": true, "\u2284": true, "\u2285": true, "\u2286": true, "\u2287": true, "\u2288": true, "\u2616": true, "\u2617": true, "\u2619": true, "\u262F": true, "\u2638": true, "\u266C": true, "\u266D": true, "\u266F": true, "\u2752": true, "\u2756": true, "\u2758": true, "\u275E": true, "\u2761": true, "\u2775": true, "\u2794": true, "\u2798": true, "\u27AF": true, "\u27B1": true, "\u27BE": true, "\u3004": true, "\u3012": true, "\u3013": true, "\u3020": true, "\u302E": true, "\u302F": true, "\u3031": true, "\u3032": true, "\u3035": true, "\u303B": true, "\u3164": true, "\uFFA0": true}
)

func isValidDateTime(dateTime string) error {
	if _, err := time.Parse(time.RFC3339, dateTime); err != nil {
		return fmt.Errorf("date-time incorrectly Formatted: %s", err.Error())
	}
	return nil
}

func isValidDate(date string) error {
	arbitraryTime := "T08:30:06.283185Z"
	dateTime := fmt.Sprintf("%s%s", date, arbitraryTime)
	return isValidDateTime(dateTime)
}

func isValidEmail(email string) error {
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("email address incorrectly Formatted: %s", err.Error())
	}
	return nil
}

func isValidHostname(hostname string) error {
	if !hostnamePattern.MatchString(hostname) || len(hostname) > 255 {
		return fmt.Errorf("invalid hostname string")
	}
	return nil
}

func isValidIDNEmail(idnEmail string) error {
	if _, err := mail.ParseAddress(idnEmail); err != nil {
		return fmt.Errorf("email address incorrectly Formatted: %s", err.Error())
	}
	return nil
}

func isValidIDNHostname(idnHostname string) error {
	if len(idnHostname) > 255 {
		return fmt.Errorf("invalid idn hostname string")
	}
	for _, r := range idnHostname {
		s := string(r)
		if disallowedIdnChars[s] {
			return fmt.Errorf("invalid hostname: contains illegal character %#U", r)
		}
	}
	return nil
}

func isValidIPv4(ipv4 string) error {
	parsedIP := net.ParseIP(ipv4)
	hasDots := strings.Contains(ipv4, ".")
	if !hasDots || parsedIP == nil {
		return fmt.Errorf("invalid IPv4 address")
	}
	return nil
}

func isValidIPv6(ipv6 string) error {
	parsedIP := net.ParseIP(ipv6)
	hasColons := strings.Contains(ipv6, ":")
	if !hasColons || parsedIP == nil {
		return fmt.Errorf("invalid IPv4 address")
	}
	return nil
}

func isValidIriRef(iriRef string) error {
	return isValidURIRef(iriRef)
}

func isValidIri(iri string) error {
	return isValidURI(iri)
}

func isValidJSONPointer(jsonPointer string) error {
	if len(jsonPointer) == 0 {
		return nil
	}
	if jsonPointer[0] != '/' {
		return fmt.Errorf("non-empty references must begin with a '/' character")
	}
	str := jsonPointer[1:]
	if unescaptedTildaPattern.MatchString(str) {
		return fmt.Errorf("unescaped tilda error")
	}
	if endingTildaPattern.MatchString(str) {
		return fmt.Errorf("unescaped tilda error")
	}
	return nil
}

func isValidRegex(regex string) error {
	if _, err := regexp.Compile(regex); err != nil {
		return fmt.Errorf("invalid regex expression")
	}
	return nil
}

func isValidRelJSONPointer(relJSONPointer string) error {
	parts := strings.Split(relJSONPointer, "/")
	if len(parts) == 1 {
		parts = strings.Split(relJSONPointer, "#")
	}
	if i, err := strconv.Atoi(parts[0]); err != nil || i < 0 {
		return fmt.Errorf("RJP must begin with positive integer")
	}
	str := relJSONPointer[len(parts[0]):]
	if len(str) > 0 && str[0] == '#' {
		return nil
	}
	return isValidJSONPointer(str)
}

func isValidTime(time string) error {
	arbitraryDate := "1963-06-19"
	dateTime := fmt.Sprintf("%sT%s", arbitraryDate, time)
	return isValidDateTime(dateTime)
	return nil
}

func isValidURIRef(uriRef string) error {
	if _, err := url.Parse(uriRef); err != nil {
		return fmt.Errorf("uri incorrectly Formatted: %s", err.Error())
	}
	if strings.Contains(uriRef, "\\") {
		return fmt.Errorf("invalid uri")
	}
	return nil
}

func isValidURITemplate(uriTemplate string) error {
	arbitraryValue := "aaa"
	uriRef := uriTemplatePattern.ReplaceAllString(uriTemplate, arbitraryValue)
	if strings.Contains(uriRef, "{") || strings.Contains(uriRef, "}") {
		return fmt.Errorf("invalid uri template")
	}
	return isValidURIRef(uriRef)
}

func isValidURI(uri string) error {
	if _, err := url.Parse(uri); err != nil {
		return fmt.Errorf("uri incorrectly Formatted: %s", err.Error())
	}
	if !schemePrefixPattern.MatchString(uri) {
		return fmt.Errorf("uri missing scheme prefix")
	}
	return nil
}

func isValidPhone(phone string) error {
	if len(phone) != 11 || phone[0] != '1' {
		return fmt.Errorf("value bust be valid phone:%s", phone)
	}

	return nil
}

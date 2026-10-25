// Package privacy finds data that must not appear in Lucarne's public
// fixtures and examples: credentials, private hostnames and addresses, and
// URLs or e-mail addresses outside the reserved example domains.
package privacy

import (
	"fmt"
	"regexp"
	"strings"
)

// Finding is one forbidden match.
type Finding struct {
	Rule  string
	Match string
}

func (f Finding) String() string { return fmt.Sprintf("%s: %q", f.Rule, f.Match) }

// forbiddenWords are names of private systems, matched case-insensitively.
// Hostnames are not listed here, which would publish them: every URL and
// e-mail address outside the example domains is a finding instead.
var forbiddenWords = []string{"vikunja"}

// credentialPatterns match token formats of the services agents talk to.
var credentialPatterns = map[string]*regexp.Regexp{
	"github token":  regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`),
	"api key":       regexp.MustCompile(`\bsk-(ant-)?[A-Za-z0-9_-]{20,}`),
	"slack token":   regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`),
	"aws key":       regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	"private key":   regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	"jwt":           regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`),
	"bearer header": regexp.MustCompile(`(?i)\bbearer [A-Za-z0-9._~+/-]{20,}`),
}

var (
	urlPattern   = regexp.MustCompile(`https?://([^/\s"'\\)<>]+)(/[^\s"'\\)<>]*)?`)
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+)`)
)

// exampleDomains are the domains RFC 2606 reserves for examples.
var exampleDomains = []string{"example.com", "example.org", "example.net"}

// publicExampleURLPrefixes are public URLs fixtures may use, under an
// "example" owner that is fabricated.
var publicExampleURLPrefixes = []string{"github.com/example/"}

// Scan returns every forbidden match in data.
func Scan(data string) []Finding {
	var findings []Finding
	lower := strings.ToLower(data)
	for _, word := range forbiddenWords {
		if strings.Contains(lower, word) {
			findings = append(findings, Finding{Rule: "private name", Match: word})
		}
	}
	for rule, pattern := range credentialPatterns {
		for _, match := range pattern.FindAllString(data, -1) {
			findings = append(findings, Finding{Rule: rule, Match: match})
		}
	}
	findings = append(findings, scanURLs(data)...)
	return append(findings, scanEmails(data)...)
}

func scanURLs(data string) []Finding {
	var findings []Finding
	for _, m := range urlPattern.FindAllStringSubmatch(data, -1) {
		host := strings.ToLower(m[1])
		if isExampleHost(host) || hasPublicExamplePrefix(host+m[2]) {
			continue
		}
		findings = append(findings, Finding{Rule: "url outside example domains", Match: m[0]})
	}
	return findings
}

func scanEmails(data string) []Finding {
	var findings []Finding
	for _, m := range emailPattern.FindAllStringSubmatch(data, -1) {
		if !isExampleHost(strings.ToLower(m[1])) {
			findings = append(findings, Finding{Rule: "e-mail outside example domains", Match: m[0]})
		}
	}
	return findings
}

func isExampleHost(host string) bool {
	host, _, _ = strings.Cut(host, ":")
	if host == "localhost" {
		return true
	}
	for _, domain := range exampleDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func hasPublicExamplePrefix(hostAndPath string) bool {
	for _, prefix := range publicExampleURLPrefixes {
		if strings.HasPrefix(hostAndPath, prefix) {
			return true
		}
	}
	return false
}

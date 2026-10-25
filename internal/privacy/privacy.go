// Package privacy finds data that must not appear in Lucarne's public
// fixtures and examples: credentials, names of private systems, IP
// addresses outside the loopback and documentation ranges, hostnames under
// private-use TLDs, and URLs or e-mail addresses outside the reserved
// example domains. Hostnames under public TLDs are found only inside a URL
// or an e-mail address, which includes a scp-style remote with a user
// (git@host:repo).
package privacy

import (
	"fmt"
	"net/netip"
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
	// ipv4Pattern and ipv6Pattern find candidates; netip.ParseAddr decides.
	ipv4Pattern = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	ipv6Pattern = regexp.MustCompile(`(?i)([0-9a-f]{0,4}:){2,7}[0-9a-f]{0,4}`)
	// dottedNamePattern finds whole dotted names, so a file name such as
	// settings.local.json is judged by its last label, not by ".local".
	dottedNamePattern = regexp.MustCompile(`(?i)\b[a-z0-9-]+(\.[a-z0-9-]+)+\b`)
)

// allowedAddressRanges are the ranges that identify no real host: the
// documentation ranges of RFC 5737 and RFC 3849. Loopback and unspecified
// addresses are allowed too.
var allowedAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// privateTLDs are the top-level names used for private networks
// (RFC 6762, RFC 8375, ICANN's .internal, and common conventions).
var privateTLDs = []string{"lan", "internal", "local", "localdomain", "corp", "home", "intranet", "private"}

// privateSuffixes are private-use names of more than one label.
var privateSuffixes = []string{".home.arpa"}

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
	findings = append(findings, scanEmails(data)...)
	findings = append(findings, scanAddresses(data)...)
	return append(findings, scanPrivateHostnames(data)...)
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

func scanAddresses(data string) []Finding {
	var findings []Finding
	for _, pattern := range []*regexp.Regexp{ipv4Pattern, ipv6Pattern} {
		for _, match := range pattern.FindAllString(data, -1) {
			addr, err := netip.ParseAddr(match)
			if err == nil && !isAllowedAddress(addr) {
				findings = append(findings, Finding{Rule: "ip address outside documentation ranges", Match: match})
			}
		}
	}
	return findings
}

func isAllowedAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsUnspecified() {
		return true
	}
	for _, prefix := range allowedAddressRanges {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func scanPrivateHostnames(data string) []Finding {
	var findings []Finding
	for _, name := range dottedNamePattern.FindAllString(data, -1) {
		if isPrivateHostname(strings.ToLower(name)) {
			findings = append(findings, Finding{Rule: "private hostname", Match: name})
		}
	}
	return findings
}

func isPrivateHostname(name string) bool {
	for _, suffix := range privateSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	tld := name[strings.LastIndexByte(name, '.')+1:]
	for _, private := range privateTLDs {
		if tld == private {
			return true
		}
	}
	return false
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

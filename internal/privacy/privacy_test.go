package privacy

import "testing"

func TestScanFlagsPrivateData(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"private name":                   "see Vikunja task 12",
		"github token":                   "token ghp_abcdefghijklmnopqrstuvwxyz0123",
		"api key":                        "key sk-ant-abcdefghijklmnopqrstuvwxyz",
		"private key":                    "-----BEGIN OPENSSH PRIVATE KEY-----",
		"jwt":                            "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.sig",
		"bearer header":                  "Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
		"url outside example domains":    "clone https://git.internal.lan/repo.git",
		"e-mail outside example domains": "author dev@corp.io",
	}
	for rule, input := range cases {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			if !hasRule(Scan(input), rule) {
				t.Errorf("Scan(%q) = %v, want a %q finding", input, Scan(input), rule)
			}
		})
	}
}

func TestScanAllowsExampleData(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"pr: https://github.com/example/widget/pull/7",
		"see https://docs.example.com/page and http://localhost:8080/",
		"author dev@example.org",
		"ok  \texample.com/widget\t0.004s",
		`{"model":"claude-opus-5-5","session_id":"0b1c2d3e-4f50-4617-8293-a4b5c6d7e8f9"}`,
	}
	for _, input := range inputs {
		if findings := Scan(input); len(findings) != 0 {
			t.Errorf("Scan(%q) = %v, want none", input, findings)
		}
	}
}

func TestScanRejectsOtherGitHubOwners(t *testing.T) {
	t.Parallel()
	if !hasRule(Scan("https://github.com/someone/private-repo"), "url outside example domains") {
		t.Error("a GitHub URL outside the example owner is not flagged")
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

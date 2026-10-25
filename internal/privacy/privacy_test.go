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
		"ip address outside documentation ranges": "ssh: connect to host 10.12.0.4 port 22",
		"private hostname":                        "fatal: unable to access git.corp.lan:repo",
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
		"connect to 192.0.2.10, 198.51.100.7, 203.0.113.99 or 2001:db8::5",
		"listening on 127.0.0.1:8080, [::1]:8080 and 0.0.0.0:8080",
		"at 12:34:56, version 2.1.296, read .claude/settings.local.json",
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

func TestScanFlagsAddresses(t *testing.T) {
	t.Parallel()
	const rule = "ip address outside documentation ranges"
	inputs := []string{
		"192.168.1.20", "172.16.5.4", "169.254.10.1", "100.64.3.2", "8.8.8.8",
		"fd12:3456::1", "fe80::1", "2606:4700::1111", "::ffff:10.0.0.1",
	}
	for _, input := range inputs {
		if !hasRule(Scan(input), rule) {
			t.Errorf("Scan(%q) = %v, want a %q finding", input, Scan(input), rule)
		}
	}
}

func TestScanFlagsPrivateHostnames(t *testing.T) {
	t.Parallel()
	const rule = "private hostname"
	inputs := []string{
		"nas.lan", "db.internal", "printer.local", "build.localdomain", "git.corp",
		"router.home", "wiki.intranet", "box.private", "nas.home.arpa",
		"lucarne.default.svc.cluster.local", "ssh GIT.CORP.LAN",
	}
	for _, input := range inputs {
		if !hasRule(Scan(input), rule) {
			t.Errorf("Scan(%q) = %v, want a %q finding", input, Scan(input), rule)
		}
	}
}

func TestScanFlagsScpRemoteWithUser(t *testing.T) {
	t.Parallel()
	if !hasRule(Scan("git@git.someone.io:team/repo.git"), "e-mail outside example domains") {
		t.Error("a scp-style remote outside the example domains is not flagged")
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

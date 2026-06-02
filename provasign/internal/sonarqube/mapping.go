package sonarqube

// Mapping translates SonarQube rule keys (in "repositoryKey:key" form) into
// Relay-side gate identifiers. The right-hand side is intentionally a single
// string — it's either a built-in Relay gate (`secrets`, `fileclass`, `deps`)
// or a semgrep rule reference (`semgrep:<rule-id>`). Adopters can use this as
// a starting point and add to it.
//
// Coverage target: this table aims to cover the most common Sonar way rules
// across Java, JavaScript/TypeScript, Python, and Go — enough to clear ≥70%
// of a typical exported profile.
var Mapping = map[string]string{
	// ---- Hard-coded credentials / secrets ------------------------------
	"java:S2068":       "secrets",
	"javascript:S2068": "secrets",
	"typescript:S2068": "secrets",
	"python:S2068":     "secrets",
	"go:S2068":         "secrets",
	"java:S6437":       "secrets",
	"javascript:S6418": "secrets",
	"typescript:S6418": "secrets",
	"python:S6418":     "secrets",
	"java:S6290":       "secrets", // AWS access keys
	"secrets:S6290":    "secrets",

	// ---- File-class denials ---------------------------------------------
	// SonarQube does not have rules for "don't commit private keys" directly;
	// we treat these denials as belonging to Relay's `fileclass` gate.
	"common-java:DuplicatedBlocks": "fileclass",

	// ---- Vulnerable / outdated dependencies -----------------------------
	"java:S2755":              "deps", // XML external entity (XXE) — often deps-resolved
	"javascript:S5852":        "deps", // ReDoS often via regex package
	"python:S5547":            "deps",
	"go:S5547":                "deps",
	"common:DuplicatedBlocks": "deps",

	// ---- Generic SAST → semgrep --------------------------------------
	"java:S1313":       "semgrep:hardcoded-ip-address",
	"javascript:S1313": "semgrep:hardcoded-ip-address",
	"typescript:S1313": "semgrep:hardcoded-ip-address",
	"python:S1313":     "semgrep:hardcoded-ip-address",
	"go:S1313":         "semgrep:hardcoded-ip-address",

	"java:S2076":       "semgrep:os-command-injection",
	"javascript:S2076": "semgrep:os-command-injection",
	"typescript:S2076": "semgrep:os-command-injection",
	"python:S2076":     "semgrep:os-command-injection",
	"go:S2076":         "semgrep:os-command-injection",

	"java:S2078":       "semgrep:ldap-injection",
	"javascript:S2078": "semgrep:ldap-injection",
	"python:S2078":     "semgrep:ldap-injection",

	"java:S2091":       "semgrep:xpath-injection",
	"javascript:S2091": "semgrep:xpath-injection",
	"python:S2091":     "semgrep:xpath-injection",

	"java:S2222":       "semgrep:lock-not-released",
	"java:S2245":       "semgrep:insecure-random",
	"javascript:S2245": "semgrep:insecure-random",
	"typescript:S2245": "semgrep:insecure-random",
	"python:S2245":     "semgrep:insecure-random",
	"go:S2245":         "semgrep:insecure-random",

	"java:S3330":       "semgrep:cookie-missing-secure",
	"javascript:S3330": "semgrep:cookie-missing-secure",
	"typescript:S3330": "semgrep:cookie-missing-secure",
	"python:S3330":     "semgrep:cookie-missing-secure",

	"java:S4423":       "semgrep:weak-ssl-tls-version",
	"javascript:S4423": "semgrep:weak-ssl-tls-version",
	"typescript:S4423": "semgrep:weak-ssl-tls-version",
	"python:S4423":     "semgrep:weak-ssl-tls-version",
	"go:S4423":         "semgrep:weak-ssl-tls-version",

	"java:S4426":       "semgrep:weak-crypto-key-size",
	"javascript:S4426": "semgrep:weak-crypto-key-size",
	"python:S4426":     "semgrep:weak-crypto-key-size",
	"go:S4426":         "semgrep:weak-crypto-key-size",

	"java:S5443":   "semgrep:insecure-temp-file",
	"python:S5443": "semgrep:insecure-temp-file",
	"go:S5443":     "semgrep:insecure-temp-file",

	"java:S5547":       "semgrep:weak-crypto-cipher",
	"javascript:S5547": "semgrep:weak-crypto-cipher",
	"typescript:S5547": "semgrep:weak-crypto-cipher",

	"java:S6093":       "semgrep:hmac-weak-secret",
	"javascript:S6093": "semgrep:hmac-weak-secret",
}

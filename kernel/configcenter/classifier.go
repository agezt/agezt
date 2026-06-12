// SPDX-License-Identifier: MIT

package configcenter

import (
	"regexp"
	"strings"
	"sync"
)

// SecretClassifier automatically detects the sensitivity rating of config values.
type SecretClassifier struct {
	mu              sync.RWMutex
	overrides       map[string]Rating
	keyPatterns     []*CompiledPattern
	valuePatterns   []*CompiledPattern
	contextualRules []*ContextualRule
}

// CompiledPattern is a regex pattern with associated rating and confidence.
type CompiledPattern struct {
	Regex   *regexp.Regexp
	Rating  Rating
	Weight  int
	Example string
}

// ContextualRule evaluates both key and value together.
type ContextualRule struct {
	Name       string
	KeyPattern *regexp.Regexp
	ValueTest  func(key, value string) bool
	Rating     Rating
	Reason     string
}

// NewSecretClassifier creates a classifier with default patterns.
func NewSecretClassifier() *SecretClassifier {
	sc := &SecretClassifier{
		overrides:       make(map[string]Rating),
		keyPatterns:     compileKeyPatterns(),
		valuePatterns:   compileValuePatterns(),
		contextualRules: compileContextualRules(),
	}
	return sc
}

func compileKeyPatterns() []*CompiledPattern {
	return []*CompiledPattern{
		// Definite secrets — key explicitly mentions secret
		{regexp.MustCompile(`(?i)(api[_\-]?key|apikey|api[_\-]?secret|secret[_\-]?key|private[_\-]?key)`), RatingSecret, 10, "api_key, apiKey, secret_key, private_key"},
		{regexp.MustCompile(`(?i)(password|passwd|pwd|credential[s]?)`), RatingSecret, 10, "password, passwd, credentials"},
		{regexp.MustCompile(`(?i)(token|jwt|bearer|auth[_\-]?token|access[_\-]?token|refresh[_\-]?token)`), RatingSecret, 9, "token, jwt_token, bearer_token"},
		{regexp.MustCompile(`(?i)(cert|certificate|key\.pem|key\.jks|key\.p12|keystore|truststore)`), RatingSecret, 9, "certificate, cert.pem, keystore.jks"},
		{regexp.MustCompile(`(?i)(aws[_\-]?access|aws[_\-]?secret|aws[_\-]?key)`), RatingSecret, 9, "aws_access_key, aws_secret_key"},
		{regexp.MustCompile(`(?i)(github[_\-]?token|github[_\-]?pat|ghp[_\-]?|ghs[_\-]?|gho[_\-]?)`), RatingSecret, 9, "github_token, ghp_xxx"},
		{regexp.MustCompile(`(?i)(slack[_\-]?token|slack[_\-]?webhook|xox[baprs])`), RatingSecret, 9, "slack_token, xoxb-xxx"},
		{regexp.MustCompile(`(?i)(stripe[_\-]?key|stripe[_\-]?secret|sk[_\-]?(live|test))`), RatingSecret, 9, "stripe_key, sk_live_xxx"},
		{regexp.MustCompile(`(?i)(sendgrid[_\-]?key|twilio[_\-]?key|mailgun[_\-]?key)`), RatingSecret, 9, "sendgrid_key, twilio_key"},
		{regexp.MustCompile(`(?i)(datadog[_\-]?key|newrelic[_\-]?key|sentry[_\-]?dsn)`), RatingSecret, 8, "datadog_key, newrelic_key"},
		{regexp.MustCompile(`(?i)(mysql[_\-]?password|postgres[_\-]?password|db[_\-]?password|mongodb[_\-]?password)`), RatingSecret, 9, "mysql_password, postgres_password"},
		{regexp.MustCompile(`(?i)(secret[_\-]?string|secret[_\-]?value|secret[_\-]?data)`), RatingSecret, 9, "secret_string, secret_value"},
		{regexp.MustCompile(`(?i)(encryption[_\-]?key|encrypt[_\-]?key|cipher[_\-]?key)`), RatingSecret, 9, "encryption_key, encrypt_key"},

		// Semi-sensitive
		{regexp.MustCompile(`(?i)(admin|root|privileged|sudo)`), RatingRestricted, 5, "admin_user, root_access"},
		{regexp.MustCompile(`(?i)(database[_\-]?url|db[_\-]?connection|connection[_\-]?string|postgres[_\-]?url|mysql[_\-]?url)`), RatingRestricted, 6, "database_url, db_connection"},
		{regexp.MustCompile(`(?i)(private[_\-]?endpoint|private[_\-]?url|internal[_\-]?api)`), RatingRestricted, 4, "private_endpoint, internal_api"},
		{regexp.MustCompile(`(?i)(ssh[_\-]?key|ssh[_\-]?password|ftp[_\-]?password)`), RatingRestricted, 7, "ssh_key, ftp_password"},

		// Non-sensitive
		{regexp.MustCompile(`(?i)(endpoint|url|host|port|address|server|base[_\-]?url|api[_\-]?url)`), RatingPublic, 1, "endpoint, api_url, service_address"},
		{regexp.MustCompile(`(?i)(version|env|environment|stage|debug|log[_\-]?level)`), RatingInternal, 2, "version, env, stage"},
		{regexp.MustCompile(`(?i)(name|title|description|display[_\-]?name)`), RatingPublic, 1, "name, title, description"},
		{regexp.MustCompile(`(?i)(timeout|retry|limit|max[_\-]?count|page[_\-]?size)`), RatingPublic, 1, "timeout, retry_count"},
	}
}

func compileValuePatterns() []*CompiledPattern {
	return []*CompiledPattern{
		// Definite secrets — value matches specific secret patterns
		{regexp.MustCompile(`^eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+$`), RatingSecret, 10, "eyJhbGciOiJIUzI1NiJ9..."},
		{regexp.MustCompile(`^AKIA[0-9A-Z]{16}$`), RatingSecret, 10, "AKIAIOSFODNN7EXAMPLE"},
		{regexp.MustCompile(`^AKIA[0-9A-Z]{16}$`), RatingSecret, 10, "AKIAIOSFODNN7EXAMPLE"},
		{regexp.MustCompile(`^ghp_[a-zA-Z0-9]{36}$`), RatingSecret, 10, "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		{regexp.MustCompile(`^ghs_[a-zA-Z0-9]{36}$`), RatingSecret, 10, "ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		{regexp.MustCompile(`^gho_[a-zA-Z0-9]{36}$`), RatingSecret, 10, "gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		{regexp.MustCompile(`^xox[baprs]-[0-9a-zA-Z-]{10,}$`), RatingSecret, 9, "xoxb-xxxx-xxxx-xxxx"},
		{regexp.MustCompile(`^sk_(live|test)_[a-zA-Z0-9]{24,}$`), RatingSecret, 10, "sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
		{regexp.MustCompile(`^sk-[a-zA-Z0-9]{20,}$`), RatingSecret, 9, "sk-xxxxxxxxxxxxxxxxxxxx"},
		{regexp.MustCompile(`^SG\.[a-zA-Z0-9_-]{22}\.[a-zA-Z0-9_-]{43}$`), RatingSecret, 10, "SG.xxxxxxxx.xxxxxxxx"},
		{regexp.MustCompile(`^S3Cr3t[a-zA-Z0-9]{20,}$`), RatingSecret, 8, "S3Cr3txxxxxxxxxxxxxxxxxxxx"},

		// Potential secrets — long base64-like strings
		{regexp.MustCompile(`^[A-Za-z0-9/+=]{40,}$`), RatingRestricted, 5, "40+ char base64 — potential secret"},

		// Non-secrets
		{regexp.MustCompile(`^https?://`), RatingPublic, 1, "https://api.example.com"},
		{regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-z]{2,}$`), RatingPublic, 1, "user@example.com"},
		{regexp.MustCompile(`^true|false$`), RatingPublic, 1, "true, false"},
		{regexp.MustCompile(`^\d+$`), RatingPublic, 1, "12345"},
		{regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`), RatingPublic, 1, "1.2.3, 1.2.3-beta"},
		{regexp.MustCompile(`^(us-east|us-west|eu-west|ap-south)`), RatingPublic, 1, "us-east, eu-west"},
	}
}

func compileContextualRules() []*ContextualRule {
	return []*ContextualRule{
		{
			Name:       "database_connection_with_creds",
			KeyPattern: regexp.MustCompile(`(?i)(db|database|postgres|mysql|mongo|redis|sql)[_\-]?(url|connection|connection_string|uri)`),
			ValueTest: func(key, value string) bool {
				return strings.Contains(value, "://") && (strings.Contains(value, "@") || strings.Contains(value, "password="))
			},
			Rating: RatingRestricted,
			Reason: "Database connection string with embedded credentials",
		},
		{
			Name:       "private_key_content",
			KeyPattern: regexp.MustCompile(`(?i)(private[_\-]?key|key\.pem|\.key|pkcs8)`),
			ValueTest: func(key, value string) bool {
				return strings.Contains(value, "-----BEGIN") && strings.Contains(value, "PRIVATE KEY-----")
			},
			Rating: RatingSecret,
			Reason: "Private key content detected",
		},
		{
			Name:       "api_key_disguised_as_name",
			KeyPattern: regexp.MustCompile(`(?i)(name|display[_\-]?name|title|label)`),
			ValueTest: func(key, value string) bool {
				matched, _ := regexp.MatchString(`^sk-(live|test)-`, value)
				return matched
			},
			Rating: RatingSecret,
			Reason: "API key found in name-like field",
		},
		{
			Name:       "hmac_secret",
			KeyPattern: regexp.MustCompile(`(?i)(hmac|signature|signing[_\-]?key)`),
			ValueTest: func(key, value string) bool {
				return len(value) >= 32 && len(value) <= 64
			},
			Rating: RatingSecret,
			Reason: "HMAC/signing key detected",
		},
	}
}

// Classify determines the rating for a given key-value pair.
func (sc *SecretClassifier) Classify(key, value string) Rating {
	sc.mu.RLock()
	override, hasOverride := sc.overrides[key]
	sc.mu.RUnlock()

	// 1. Manual override takes precedence
	if hasOverride {
		return override
	}

	// 2. Contextual rules (highest confidence when matched)
	for _, rule := range sc.contextualRules {
		if rule.KeyPattern.MatchString(key) && rule.ValueTest(key, value) {
			if rule.Rating == RatingSecret {
				return RatingSecret
			}
			return rule.Rating
		}
	}

	// 3. Key patterns (high confidence for explicit secret keywords)
	for _, p := range sc.keyPatterns {
		if p.Regex.MatchString(key) {
			if p.Rating == RatingSecret && p.Weight >= 8 {
				return RatingSecret
			}
			// Track the highest non-secret rating
			if p.Weight >= 6 {
				return p.Rating
			}
		}
	}

	// 4. Value patterns
	for _, p := range sc.valuePatterns {
		if p.Regex.MatchString(value) {
			if p.Rating == RatingSecret && p.Weight >= 8 {
				return RatingSecret
			}
			return p.Rating
		}
	}

	// 5. Length heuristics — very long alphanumeric strings are suspicious
	if len(value) > 50 && !strings.ContainsAny(value, " \t\n") {
		// Single word, 50+ chars — potentially a secret
		return RatingRestricted
	}

	// Default to internal (not public, not secret)
	return RatingInternal
}

// SetOverride manually assigns a rating to a key.
func (sc *SecretClassifier) SetOverride(key string, rating Rating) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.overrides[key] = rating
}

// GetOverride returns the manual override for a key (if any).
func (sc *SecretClassifier) GetOverride(key string) (Rating, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	r, ok := sc.overrides[key]
	return r, ok
}

// RemoveOverride removes a manual override.
func (sc *SecretClassifier) RemoveOverride(key string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.overrides, key)
}

// SuggestRating explains why a rating was suggested.
func (sc *SecretClassifier) SuggestRating(key, value string) (Rating, string) {
	rating := sc.Classify(key, value)

	// Find the reason
	for _, rule := range sc.contextualRules {
		if rule.KeyPattern.MatchString(key) && rule.ValueTest(key, value) {
			return rating, rule.Reason
		}
	}

	for _, p := range sc.keyPatterns {
		if p.Regex.MatchString(key) {
			return p.Rating, "key matches pattern: " + p.Example
		}
	}

	for _, p := range sc.valuePatterns {
		if p.Regex.MatchString(value) {
			return p.Rating, "value matches pattern: " + p.Example
		}
	}

	if len(value) > 50 && !strings.ContainsAny(value, " \t\n") {
		return RatingRestricted, "long alphanumeric string (potential secret)"
	}

	return rating, "default rating"
}

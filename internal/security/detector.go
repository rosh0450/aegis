package security

import (
	"regexp"
	"sort"
)

// KeyPattern defines a regex pattern for detecting a specific type of API key.
type KeyPattern struct {
	// Provider is the API provider identifier, e.g. "openai", "anthropic", "aws".
	Provider string

	// Name is a human-readable description, e.g. "OpenAI Secret Key".
	Name string

	// Pattern is the compiled regex used to match potential keys.
	Pattern *regexp.Regexp

	// MinEntropy is the minimum Shannon entropy required to consider a
	// regex match as a genuine key. This filters out low-randomness
	// false positives. A typical default is 3.5.
	MinEntropy float64
}

// DetectedKey represents a potential API key found in text.
type DetectedKey struct {
	// Provider is the API provider identifier.
	Provider string

	// Name is the human-readable pattern name that matched.
	Name string

	// Value is the full detected key string.
	Value string

	// Redacted is a truncated version of the key safe for logging.
	// Keys longer than 16 characters show first 8 + "..." + last 4.
	// Shorter keys show first 4 + "...".
	Redacted string

	// Offset is the byte offset in the original text where the key starts.
	Offset int

	// Entropy is the Shannon entropy of the detected value.
	Entropy float64
}

// Detector scans text for API key patterns using regex matching and
// Shannon entropy analysis to reduce false positives.
type Detector struct {
	patterns []KeyPattern
}

// NewDetector creates a [Detector] pre-loaded with built-in patterns for
// common API providers including OpenAI, Anthropic, Google/Gemini, AWS,
// Stripe, and generic Bearer tokens.
func NewDetector() *Detector {
	return &Detector{
		patterns: []KeyPattern{
			{
				Provider:   "openai",
				Name:       "OpenAI Secret Key",
				Pattern:    regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
				MinEntropy: 3.5,
			},
			{
				Provider:   "openai",
				Name:       "OpenAI Project Key",
				Pattern:    regexp.MustCompile(`sk-proj-[a-zA-Z0-9_\-]{20,}`),
				MinEntropy: 3.0,
			},
			{
				Provider:   "anthropic",
				Name:       "Anthropic Secret Key",
				Pattern:    regexp.MustCompile(`sk-ant-[a-zA-Z0-9_\-]{20,}`),
				MinEntropy: 3.0,
			},
			{
				Provider:   "google",
				Name:       "Google/Gemini API Key",
				Pattern:    regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
				MinEntropy: 3.5,
			},
			{
				Provider:   "aws",
				Name:       "AWS Access Key",
				Pattern:    regexp.MustCompile(`(AKIA|ASIA)[A-Z0-9]{16}`),
				MinEntropy: 3.0,
			},
			{
				Provider:   "generic",
				Name:       "Generic Bearer Token",
				Pattern:    regexp.MustCompile(`Bearer\s+[A-Za-z0-9\-._~+/]{20,}=*`),
				MinEntropy: 4.0,
			},
			{
				Provider:   "stripe",
				Name:       "Stripe API Key",
				Pattern:    regexp.MustCompile(`(sk|pk)_(test|live)_[a-zA-Z0-9]{20,}`),
				MinEntropy: 3.0,
			},
		},
	}
}

// AddPattern adds a custom key pattern to the detector.
func (d *Detector) AddPattern(p KeyPattern) {
	d.patterns = append(d.patterns, p)
}

// Scan searches text for potential API keys and returns all matches
// that exceed their pattern's minimum entropy threshold. Results are
// sorted by byte offset in the original text.
func (d *Detector) Scan(text string) []DetectedKey {
	var results []DetectedKey

	for _, kp := range d.patterns {
		matches := kp.Pattern.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			value := text[loc[0]:loc[1]]
			entropy := ShannonEntropy(value)

			if entropy < kp.MinEntropy {
				continue
			}

			results = append(results, DetectedKey{
				Provider: kp.Provider,
				Name:     kp.Name,
				Value:    value,
				Redacted: redact(value),
				Offset:   loc[0],
				Entropy:  entropy,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Offset < results[j].Offset
	})

	return results
}

// redact produces a truncated version of a key suitable for logging.
// Keys longer than 16 characters show first 8 + "..." + last 4.
// Shorter keys show first 4 + "...".
func redact(s string) string {
	if len(s) > 16 {
		return s[:8] + "..." + s[len(s)-4:]
	}
	if len(s) > 4 {
		return s[:4] + "..."
	}
	return s + "..."
}

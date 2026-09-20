package security

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// ---------- Shannon Entropy Tests ----------

func TestShannonEntropy_EmptyString(t *testing.T) {
	got := ShannonEntropy("")
	if got != 0.0 {
		t.Errorf("ShannonEntropy(\"\") = %f; want 0.0", got)
	}
}

func TestShannonEntropy_SingleCharRepeated(t *testing.T) {
	// "aaaa" has zero entropy – every byte is the same.
	got := ShannonEntropy("aaaa")
	if got != 0.0 {
		t.Errorf("ShannonEntropy(\"aaaa\") = %f; want 0.0", got)
	}
}

func TestShannonEntropy_TwoDistinctChars(t *testing.T) {
	// "ab" has 1 bit of entropy per character.
	got := ShannonEntropy("ab")
	if math.Abs(got-1.0) > 0.001 {
		t.Errorf("ShannonEntropy(\"ab\") = %f; want ~1.0", got)
	}
}

func TestShannonEntropy_HighEntropyString(t *testing.T) {
	// A string with many distinct characters should have high entropy.
	high := "aB3$xZ9!mK7@pQ2&wL5"
	got := ShannonEntropy(high)
	if got < 3.5 {
		t.Errorf("ShannonEntropy(%q) = %f; want >= 3.5", high, got)
	}
}

func TestShannonEntropy_RealisticAPIKey(t *testing.T) {
	key := "sk-Rg4kZ9mNvX2bQ7wYpL8jT3cF6hA0dU5eI1oS"
	got := ShannonEntropy(key)
	if got < 3.5 {
		t.Errorf("ShannonEntropy(realistic key) = %f; want >= 3.5", got)
	}
}

// ---------- Detector Pattern Tests ----------

func TestDetector_OpenAIKey(t *testing.T) {
	d := NewDetector()
	text := "sk-Rg4kZ9mNvX2bQ7wYpL8jT3cF6hA0dU5eI1oS"
	results := d.Scan(text)

	if len(results) == 0 {
		t.Fatal("expected at least one match for OpenAI key pattern")
	}

	found := false
	for _, r := range results {
		if r.Provider == "openai" && r.Name == "OpenAI Secret Key" {
			found = true
			if r.Value != text {
				t.Errorf("Value = %q; want %q", r.Value, text)
			}
			break
		}
	}
	if !found {
		t.Error("did not find an OpenAI Secret Key match")
	}
}

func TestDetector_OpenAIProjectKey(t *testing.T) {
	d := NewDetector()
	text := "sk-proj-aB3xZ9mK7pQ2wL5nR8-vY4jT6cF0hU1"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "openai" && r.Name == "OpenAI Project Key" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find an OpenAI Project Key match")
	}
}

func TestDetector_AnthropicKey(t *testing.T) {
	d := NewDetector()
	text := "sk-ant-aB3xZ9mK7pQ2wL5nR8_vY4jT6cF0hU1"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "anthropic" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find an Anthropic key match")
	}
}

func TestDetector_GoogleGeminiKey(t *testing.T) {
	d := NewDetector()
	// AIza + exactly 35 chars of [0-9A-Za-z\-_]
	text := "AIzaSyB3xZ9mK7pQ2wL5nR8vY4jT6cF0hU1eI3oW"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "google" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find a Google/Gemini key match")
	}
}

func TestDetector_AWSAccessKey(t *testing.T) {
	d := NewDetector()
	text := "AKIAIOSFODNN7EXAMPLE"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "aws" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find an AWS Access Key match")
	}
}

func TestDetector_StripeKey(t *testing.T) {
	d := NewDetector()
	text := "sk_test_4eC39HqLyjWDarjtT1zdp7dc"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "stripe" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find a Stripe key match")
	}
}

func TestDetector_GenericBearerToken(t *testing.T) {
	d := NewDetector()
	text := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkw"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "generic" && r.Name == "Generic Bearer Token" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find a Generic Bearer Token match")
	}
}

// ---------- Low-Entropy Filtering ----------

func TestDetector_LowEntropyFiltered(t *testing.T) {
	d := NewDetector()
	// Construct a string that matches the OpenAI regex but has very low
	// entropy (all same chars after the prefix).
	lowEntropy := "sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	results := d.Scan(lowEntropy)

	for _, r := range results {
		if r.Provider == "openai" && r.Name == "OpenAI Secret Key" {
			t.Errorf("low-entropy string should have been filtered out; got entropy=%f", r.Entropy)
		}
	}
}

// ---------- Redaction Tests ----------

func TestDetector_RedactedLongKey(t *testing.T) {
	d := NewDetector()
	text := "sk-Rg4kZ9mNvX2bQ7wYpL8jT3cF6hA0dU5eI1oS"
	results := d.Scan(text)

	if len(results) == 0 {
		t.Fatal("expected at least one match")
	}

	for _, r := range results {
		if r.Provider == "openai" && r.Name == "OpenAI Secret Key" {
			// first 8 + "..." + last 4
			wantPrefix := text[:8]
			wantSuffix := text[len(text)-4:]
			if !strings.HasPrefix(r.Redacted, wantPrefix) {
				t.Errorf("Redacted %q does not start with %q", r.Redacted, wantPrefix)
			}
			if !strings.HasSuffix(r.Redacted, wantSuffix) {
				t.Errorf("Redacted %q does not end with %q", r.Redacted, wantSuffix)
			}
			if !strings.Contains(r.Redacted, "...") {
				t.Errorf("Redacted %q does not contain \"...\"", r.Redacted)
			}
			return
		}
	}
	t.Error("did not find an OpenAI Secret Key match for redaction test")
}

func TestRedact_ShortKey(t *testing.T) {
	// A key with 16 or fewer chars should show first 4 + "..."
	short := "AKIAIOSFODNN7EXA" // exactly 16 chars
	got := redact(short)
	want := "AKIA..."
	if got != want {
		t.Errorf("redact(%q) = %q; want %q", short, got, want)
	}
}

func TestRedact_VeryShortKey(t *testing.T) {
	got := redact("abcd")
	want := "abcd..."
	if got != want {
		t.Errorf("redact(%q) = %q; want %q", "abcd", got, want)
	}
}

// ---------- Multi-Key Scan ----------

func TestDetector_MultipleKeysInText(t *testing.T) {
	d := NewDetector()

	text := `Configuration file:
  OPENAI_KEY=sk-Rg4kZ9mNvX2bQ7wYpL8jT3cF6hA0dU5eI1oS
  ANTHROPIC_KEY=sk-ant-aB3xZ9mK7pQ2wL5nR8_vY4jT6cF0hU1
  STRIPE_KEY=sk_test_4eC39HqLyjWDarjtT1zdp7dc
`

	results := d.Scan(text)

	providers := make(map[string]bool)
	for _, r := range results {
		providers[r.Provider] = true
	}

	for _, want := range []string{"openai", "anthropic", "stripe"} {
		if !providers[want] {
			t.Errorf("expected provider %q in scan results", want)
		}
	}

	// Verify results are sorted by offset.
	for i := 1; i < len(results); i++ {
		if results[i].Offset < results[i-1].Offset {
			t.Errorf("results not sorted by offset: [%d].Offset=%d < [%d].Offset=%d",
				i, results[i].Offset, i-1, results[i-1].Offset)
		}
	}
}

// ---------- Normal Text (No Keys) ----------

func TestDetector_NormalTextNoMatches(t *testing.T) {
	d := NewDetector()
	text := `Hello world! This is a normal paragraph of text.
It contains no API keys, secrets, or tokens.
Just regular English sentences with numbers like 42 and punctuation.`

	results := d.Scan(text)
	if len(results) != 0 {
		t.Errorf("expected 0 matches for normal text; got %d", len(results))
		for _, r := range results {
			t.Logf("  unexpected match: provider=%s value=%q entropy=%f", r.Provider, r.Value, r.Entropy)
		}
	}
}

// ---------- AddPattern ----------

func TestDetector_AddPattern(t *testing.T) {
	d := NewDetector()
	d.AddPattern(KeyPattern{
		Provider:   "custom",
		Name:       "Custom Token",
		Pattern:    regexp.MustCompile(`ctk_[a-zA-Z0-9]{20,}`),
		MinEntropy: 3.0,
	})

	text := "ctk_aB3xZ9mK7pQ2wL5nR8vY"
	results := d.Scan(text)

	found := false
	for _, r := range results {
		if r.Provider == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find a Custom Token match after AddPattern")
	}
}

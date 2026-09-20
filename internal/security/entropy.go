// Package security provides API key leak detection capabilities.
// It scans strings and byte slices for accidentally included API keys
// using regex pattern matching combined with Shannon entropy analysis
// to reduce false positives.
package security

import "math"

// ShannonEntropy calculates the Shannon entropy of a string.
// Higher entropy indicates more randomness; API keys typically have
// entropy >= 3.5 bits per character.
//
// The formula is: H(X) = -Σ P(xi) * log2(P(xi))
//
// Returns 0.0 for empty strings.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0.0
	}

	// Count the frequency of each byte.
	freq := make(map[byte]int)
	for i := 0; i < len(s); i++ {
		freq[s[i]]++
	}

	// Calculate Shannon entropy.
	length := float64(len(s))
	var entropy float64
	for _, count := range freq {
		p := float64(count) / length
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

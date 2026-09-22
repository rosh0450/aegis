package budget

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// TokenUsage represents token consumption extracted from an API response.
type TokenUsage struct {
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	CacheReadTokens  int    `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int    `json:"cache_write_tokens,omitempty"`
}

// apiResponse is a minimal representation of an AI API response body used to
// extract token usage. It supports both OpenAI and Anthropic response formats.
type apiResponse struct {
	Model string    `json:"model"`
	Usage *apiUsage `json:"usage"`
}

// apiUsage captures the union of OpenAI and Anthropic usage fields.
type apiUsage struct {
	// OpenAI fields
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`

	// OpenAI cache details (nested)
	PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details"`

	// Anthropic fields
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`

	// Anthropic cache fields
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// promptTokensDetails holds OpenAI's nested cache token details.
type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// ExtractTokenUsage parses token usage from an API response body. It supports
// OpenAI and Anthropic response formats. The response body is read and then
// restored so that downstream consumers can read it again.
//
// If the response does not contain a usage object (e.g. streaming chunks,
// non-chat endpoints), nil is returned with no error.
func ExtractTokenUsage(resp *http.Response) (*TokenUsage, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Restore the body so downstream consumers can read it.
	resp.Body = io.NopCloser(bytes.NewReader(body))

	var parsed apiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Not valid JSON — not an error for our purposes (could be
		// streaming, binary, etc.).
		return nil, nil //nolint:nilerr // intentional
	}

	if parsed.Usage == nil {
		return nil, nil
	}

	usage := &TokenUsage{
		Model: parsed.Model,
	}

	// Determine format: Anthropic uses input_tokens/output_tokens,
	// OpenAI uses prompt_tokens/completion_tokens.
	if parsed.Usage.InputTokens > 0 || parsed.Usage.OutputTokens > 0 {
		// Anthropic format
		usage.PromptTokens = parsed.Usage.InputTokens
		usage.CompletionTokens = parsed.Usage.OutputTokens
		usage.TotalTokens = parsed.Usage.InputTokens + parsed.Usage.OutputTokens
		usage.CacheReadTokens = parsed.Usage.CacheReadInputTokens
		usage.CacheWriteTokens = parsed.Usage.CacheCreationInputTokens
	} else {
		// OpenAI format
		usage.PromptTokens = parsed.Usage.PromptTokens
		usage.CompletionTokens = parsed.Usage.CompletionTokens
		usage.TotalTokens = parsed.Usage.TotalTokens

		if parsed.Usage.PromptTokensDetails != nil {
			usage.CacheReadTokens = parsed.Usage.PromptTokensDetails.CachedTokens
		}
	}

	return usage, nil
}

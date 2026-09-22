// Package budget provides token usage tracking, cost calculation, and spend
// enforcement for AI model API calls routed through the Aegis egress proxy.
package budget

// ModelPricing holds per-token pricing for an AI model.
// All prices are expressed in US dollars per 1 million tokens.
type ModelPricing struct {
	Model               string  `json:"model"`
	InputPerMToken      float64 `json:"input_per_m_token"`       // $ per 1M input tokens
	OutputPerMToken     float64 `json:"output_per_m_token"`      // $ per 1M output tokens
	CacheReadPerMToken  float64 `json:"cache_read_per_m_token"`  // $ per 1M cached input tokens
	CacheWritePerMToken float64 `json:"cache_write_per_m_token"` // $ per 1M cache write tokens
}

// PricingCatalog holds pricing for all known models. It provides lookup by
// model name and cost calculation given token usage.
type PricingCatalog struct {
	models map[string]ModelPricing
}

// NewPricingCatalog creates a catalog pre-populated with built-in pricing for
// common OpenAI, Anthropic, and Google models.
func NewPricingCatalog() *PricingCatalog {
	c := &PricingCatalog{
		models: make(map[string]ModelPricing),
	}

	builtIn := []ModelPricing{
		// OpenAI models
		{Model: "gpt-4o", InputPerMToken: 2.50, OutputPerMToken: 10.00},
		{Model: "gpt-4o-mini", InputPerMToken: 0.15, OutputPerMToken: 0.60},
		{Model: "gpt-4-turbo", InputPerMToken: 10.00, OutputPerMToken: 30.00},
		{Model: "gpt-3.5-turbo", InputPerMToken: 0.50, OutputPerMToken: 1.50},

		// Anthropic models
		{Model: "claude-3-5-sonnet", InputPerMToken: 3.00, OutputPerMToken: 15.00, CacheReadPerMToken: 0.30, CacheWritePerMToken: 3.75},
		{Model: "claude-3-haiku", InputPerMToken: 0.25, OutputPerMToken: 1.25, CacheReadPerMToken: 0.03, CacheWritePerMToken: 0.30},
		{Model: "claude-3-opus", InputPerMToken: 15.00, OutputPerMToken: 75.00, CacheReadPerMToken: 1.50, CacheWritePerMToken: 18.75},

		// Google models
		{Model: "gemini-1.5-pro", InputPerMToken: 1.25, OutputPerMToken: 5.00},
		{Model: "gemini-1.5-flash", InputPerMToken: 0.075, OutputPerMToken: 0.30},
	}

	for _, p := range builtIn {
		c.models[p.Model] = p
	}

	return c
}

// GetPricing returns pricing for a model. If the model is not found in the
// catalog, it returns a zero-value ModelPricing and false.
func (c *PricingCatalog) GetPricing(model string) (ModelPricing, bool) {
	p, ok := c.models[model]
	return p, ok
}

// CalculateCost computes the total cost in US dollars for the given token usage
// and model. The formula is:
//
//	cost = (prompt_tokens - cache_read_tokens) * input_rate / 1M
//	     + cache_read_tokens * cache_read_rate / 1M
//	     + completion_tokens * output_rate / 1M
//
// If the model is unknown, the cost is zero.
func (c *PricingCatalog) CalculateCost(model string, usage TokenUsage) float64 {
	p, ok := c.models[model]
	if !ok {
		return 0
	}

	const million = 1_000_000.0

	nonCachedInput := usage.PromptTokens - usage.CacheReadTokens
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}

	cost := float64(nonCachedInput) * p.InputPerMToken / million
	cost += float64(usage.CacheReadTokens) * p.CacheReadPerMToken / million
	cost += float64(usage.CompletionTokens) * p.OutputPerMToken / million

	return cost
}

# Aegis 🛡️

**Open-Source API Key & Security Governance Proxy**

Aegis is a single-binary, ultra-fast API egress proxy that sits between your application and third-party APIs. It provides zero-trust egress filtering, local semantic caching, and budget & token guardrails — all without requiring a cloud account or mandatory telemetry.

```
Your App  ──→  Aegis Proxy  ──→  OpenAI / Anthropic / Stripe / etc.
                   │
                   ├── 🔒 Strip & inject API keys
                   ├── 🚫 Block unauthorized domains
                   ├── 💰 Enforce budget limits
                   ├── ⚡ Cache repeated requests
                   └── 📊 Log & audit all traffic
```

## ✨ Features

- **Zero-Trust Egress Filtering** — Automatically strip sensitive auth headers before requests reach unauthorized domains. Allowlist/denylist domain control.
- **Credential Governance** — Applications never see real API keys. Aegis injects credentials from environment variables at proxy time.
- **Local Semantic Caching** — Cache costly API endpoints (like OpenAI embeddings) locally in memory, SQLite, or Redis to reduce redundant requests.
- **Budget & Token Guardrails** — Set hard cost ceilings per developer, endpoint, or environment to prevent accidental $10,000 cloud bills.
- **API Key Leak Detection** — Scan outbound request bodies for accidentally included API keys before they leave your network.
- **Single Binary** — Zero dependencies. Download and run. No cloud account or signup required.

## 🚀 Quick Start

### Install

```bash
# From source
go install github.com/rishavkumarj/aegis/cmd/aegis@latest

# Or build from source
git clone https://github.com/rishavkumarj/aegis.git
cd aegis
make build
```

### Run

```bash
# Generate example config
aegis config init

# Set your API keys as environment variables
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."

# Start the proxy
aegis serve
```

### Use

Point your API clients to Aegis instead of the provider directly:

```python
# Python (OpenAI SDK)
import openai

client = openai.OpenAI(
    base_url="http://localhost:8080/v1/openai",  # Point to Aegis
    api_key="unused",  # Aegis injects the real key
)

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello!"}],
)
```

```bash
# cURL
curl http://localhost:8080/v1/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello!"}]}'
```

```typescript
// TypeScript (Anthropic SDK)
import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  baseURL: "http://localhost:8080/v1/anthropic", // Point to Aegis
  apiKey: "unused", // Aegis injects the real key
});
```

## ⚙️ Configuration

Aegis uses a YAML configuration file. Generate one with:

```bash
aegis config init
```

See [`config.example.yaml`](config.example.yaml) for all options.

### Environment Variables

All config values can be overridden via environment variables with the `AEGIS_` prefix:

```bash
AEGIS_SERVER_ADDR=":9000"           # Override listen address
AEGIS_SECURITY_MODE="allowlist"     # Override security mode
AEGIS_LOGGING_LEVEL="debug"         # Override log level
```

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────┐
│                   AEGIS PROXY                        │
│                                                      │
│  Request ──→ Allowlist ──→ Credential ──→ Rate      │
│              Filter       Strip/Inject    Limiter    │
│                                              │       │
│                                              ▼       │
│                                          Cache ──→   │
│                                          Lookup      │
│                                           │    │     │
│                                      Hit  │    │Miss │
│                                           ▼    ▼     │
│                                        Return  Upstream
│                                        Cached  Transport
│                                              │       │
│                                              ▼       │
│                              Token Count ──→ Budget  │
│                              & Cost Calc    Tracker  │
│                                              │       │
│                                              ▼       │
│                                          Audit Log   │
└─────────────────────────────────────────────────────┘
```

## 📝 License

MIT License — see [LICENSE](LICENSE) for details.

## 🤝 Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

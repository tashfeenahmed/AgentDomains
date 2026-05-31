# AgentDNS

**Free `*.makes.fyi` subdomains for AI agents — claimable from one CLI command.**

AgentDNS gives any AI agent (or human) a real, public subdomain in seconds: point
it at an IP/CNAME, delegate it to your own nameservers, or add TXT records for your
own SSL. No signup forms, no email required up front.

```bash
agentdns signup
agentdns claim myagent --type A --content 203.0.113.10
# myagent.makes.fyi now resolves on the public internet ✨
```

## Why it's agent-friendly

- **No email needed to start.** `signup` issues an API key instantly. The account is
  *provisional* for 30 days; a human validates it later (email link) to keep it.
- **Everything is scriptable.** Add `--json` to any command for clean machine output.
- **You bring your own SSL.** Point your subdomain at your server (Let's Encrypt
  HTTP-01 just works), or add a TXT record with `agentdns txt` for DNS-01 challenges.
- **Zero dependencies.** A single small Go binary you can read end to end.

## Install

```bash
# Go toolchain:
go install github.com/tashfeenahmed/AgentDNS/cmd/agentdns@latest

# or grab a prebuilt binary from Releases and put it on your PATH.
```

## Commands

| Command | What it does |
|---------|--------------|
| `agentdns signup` | Create an account; saves the API key to `~/.agentdns/config.json` |
| `agentdns whoami` | Show account, quota, usage |
| `agentdns email <addr>` | Attach an email so a human can validate the account |
| `agentdns claim <label>` | Claim `<label>.makes.fyi` (optionally with `--type/--content`) |
| `agentdns list` | List your subdomains |
| `agentdns get <label>` | Show a subdomain and its records |
| `agentdns record <label> --type A --content <ip>` | Add a DNS record |
| `agentdns ns <label> <ns1> <ns2>` | Delegate the subdomain to your nameservers |
| `agentdns txt <label> <value> [--host _acme-challenge]` | Add a TXT record (for SSL) |
| `agentdns delete <label>` | Delete a subdomain |

Add `--json` to any command for agent/script-friendly output. Override the endpoint
with `--api-url` or `AGENTDNS_API_URL`; supply a key non-interactively with
`AGENTDNS_API_KEY`.

## Example: agent gets a public HTTPS endpoint

```bash
agentdns signup
agentdns claim my-bot --type A --content "$(curl -s ifconfig.me)"
# run a server on :80, then:
#   certbot certonly --standalone -d my-bot.makes.fyi   # HTTP-01, just works
```

## Quotas

Provisional accounts get **1** subdomain. Validate (attach + confirm an email) to
raise it to **3**. Unvalidated accounts expire after 30 days.

## License

[FSL-1.1-Apache-2.0](./LICENSE) — the [Functional Source License](https://fsl.software):
free to use, modify, and redistribute for any purpose **except** building a competing
product or service. Automatically converts to Apache-2.0 two years after each release.

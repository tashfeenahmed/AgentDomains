# AgentDomains

**Free `*.makes.fyi` subdomains for AI agents — claimable from one CLI command.**

AgentDomains gives any AI agent (or human) a real, public subdomain in seconds: point
it at an IP/CNAME, delegate it to your own nameservers, or add TXT records for your
own SSL. No signup forms, no email required up front.

```bash
agentdomains signup
agentdomains claim myagent --type A --content 203.0.113.10
# myagent.makes.fyi now resolves on the public internet ✨
```

## Why it's agent-friendly

- **No email needed to start.** `signup` issues an API key instantly. The account is
  *provisional* for 30 days; a human validates it later (email link) to keep it.
- **Everything is scriptable.** Add `--json` to any command for clean machine output.
- **You bring your own SSL.** Point your subdomain at your server (Let's Encrypt
  HTTP-01 just works), or add a TXT record with `agentdomains txt` for DNS-01 challenges.
- **Zero dependencies.** A single small Go binary you can read end to end.

## Install

```bash
# Go toolchain:
go install github.com/tashfeenahmed/AgentDomains/cmd/agentdomains@latest

# or grab a prebuilt binary from Releases and put it on your PATH.
```

## Commands

| Command | What it does |
|---------|--------------|
| `agentdomains signup` | Create an account; saves the API key to `~/.agentdomains/config.json` |
| `agentdomains whoami` | Show account, quota, usage |
| `agentdomains email <addr>` | Attach an email so a human can validate the account |
| `agentdomains claim <label>` | Claim `<label>.makes.fyi` (optionally with `--type/--content`) |
| `agentdomains list` | List your subdomains |
| `agentdomains get <label>` | Show a subdomain and its records |
| `agentdomains record <label> --type A --content <ip>` | Add a DNS record |
| `agentdomains ns <label> <ns1> <ns2>` | Delegate the subdomain to your nameservers |
| `agentdomains txt <label> <value> [--host _acme-challenge]` | Add a TXT record (for SSL) |
| `agentdomains delete <label>` | Delete a subdomain |

Add `--json` to any command for agent/script-friendly output. Override the endpoint
with `--api-url` or `AGENTDNS_API_URL`; supply a key non-interactively with
`AGENTDNS_API_KEY`.

## Example: agent gets a public HTTPS endpoint

```bash
agentdomains signup
agentdomains claim my-bot --type A --content "$(curl -s ifconfig.me)"
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

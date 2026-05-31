// Command agentdomains is the CLI for AgentDomains: free domains for the sites
// and APIs AI agents build. Names live under makes.fyi or agentdomains.co.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/tashfeenahmed/AgentDomains/internal/client"
	"github.com/tashfeenahmed/AgentDomains/internal/config"
)

const usage = `agentdomains — free domains for the sites your AI agents build

USAGE
  agentdomains <command> [flags]

COMMANDS
  signup                 Create an account and save the API key locally
  whoami                 Show your account, quota, usage, and available domains
  email <address>        Attach an email so a human can validate the account
  claim <label>          Claim <label>.<domain> (default domain: makes.fyi)
  list                   List your domains
  get <label>            Show one domain and its records
  record <label>         Add a DNS record to a domain
  ns <label> <ns>...     Delegate the domain to your own nameservers
  txt <label> <value>    Add a TXT record (e.g. for ACME / SSL challenges)
  delete <label>         Delete a domain and its records

GLOBAL FLAGS
  --json                 Emit raw JSON (ideal for agents/scripts)
  --api-url <url>        Override API endpoint (default: https://api.agentdomains.co)
  --domain <domain>      Which domain to act under: makes.fyi or agentdomains.co
                         (default: makes.fyi)

Run "agentdomains <command> -h" for command-specific flags.`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(0)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "-h", "--help", "help":
		fmt.Println(usage)
	case "signup":
		cmdSignup(args)
	case "whoami":
		cmdWhoami(args)
	case "email":
		cmdEmail(args)
	case "claim":
		cmdClaim(args)
	case "list":
		cmdList(args)
	case "get":
		cmdGet(args)
	case "record":
		cmdRecord(args)
	case "ns":
		cmdNS(args)
	case "txt":
		cmdTXT(args)
	case "delete":
		cmdDelete(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", cmd, usage)
		os.Exit(2)
	}
}

// ---------- shared helpers ----------

type globals struct {
	json   bool
	apiURL string
	domain string
}

// newFlagSet registers the global flags on a per-command flag set.
func newFlagSet(name string) (*flag.FlagSet, *globals) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	g := &globals{}
	fs.BoolVar(&g.json, "json", false, "emit raw JSON")
	fs.StringVar(&g.apiURL, "api-url", "", "override API endpoint")
	fs.StringVar(&g.domain, "domain", "", "domain to act under (makes.fyi or agentdomains.co)")
	return fs, g
}

// parse handles flags and positionals in ANY order. Go's stdlib flag package
// stops at the first positional, so we repeatedly Parse, peeling off one
// positional each round until only flags remain. Returns the positionals.
func parse(fs *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		_ = fs.Parse(args)
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	return pos
}

// resourcePath builds /v1/subdomains/<label>[?domain=...] for the {label}
// endpoints, scoping the lookup to a domain when one was given.
func resourcePath(label string, g *globals, suffix string) string {
	p := "/v1/subdomains/" + url.PathEscape(label) + suffix
	if g.domain != "" {
		p += "?domain=" + url.QueryEscape(g.domain)
	}
	return p
}

func mustClient(g *globals, needKey bool) (*client.Client, config.Config) {
	cfg := config.Load()
	if g.apiURL != "" {
		cfg.APIURL = g.apiURL
	}
	if needKey && cfg.APIKey == "" {
		fail("no API key found — run `agentdomains signup` first (or set AGENTDOMAINS_API_KEY)")
	}
	return client.New(cfg.APIURL, cfg.APIKey), cfg
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}

// out prints either raw JSON (when --json) or a human line via the formatter.
func out(g *globals, v map[string]any, human func(map[string]any)) {
	if g.json {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return
	}
	human(v)
}

// ---------- commands ----------

func cmdSignup(args []string) {
	fs, g := newFlagSet("signup")
	parse(fs, args)
	c, cfg := mustClient(g, false)

	var resp map[string]any
	check(c.Do("POST", "/v1/signup", nil, &resp))

	if key, ok := resp["api_key"].(string); ok {
		cfg.APIKey = key
		if id, ok := resp["account_id"].(string); ok {
			cfg.AccountID = id
		}
		check(config.Save(cfg))
	}
	out(g, resp, func(m map[string]any) {
		fmt.Println("✓ Account created. API key saved to ~/.agentdomains/config.json")
		fmt.Printf("  account: %v\n  quota:   %v domain(s)\n", m["account_id"], m["quota"])
		fmt.Println("\n  This is a PROVISIONAL account. Validate within 30 days to keep it:")
		fmt.Println("    agentdomains email you@example.com   # then click the link we send")
	})
}

func cmdWhoami(args []string) {
	fs, g := newFlagSet("whoami")
	parse(fs, args)
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("GET", "/v1/whoami", nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("account:        %v\n", m["account_id"])
		fmt.Printf("state:          %v\n", m["state"])
		fmt.Printf("email:          %v (verified: %v)\n", orDash(m["email"]), m["email_verified"])
		fmt.Printf("domains used:   %v / %v\n", m["used"], m["quota"])
		if d, ok := m["domains"].([]any); ok && len(d) > 0 {
			fmt.Printf("available:      %v\n", joinAny(d))
		}
	})
}

func cmdEmail(args []string) {
	fs, g := newFlagSet("email")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains email <address>")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("POST", "/v1/account/email", map[string]any{"email": pos[0]}, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Verification link sent to %v. A human must click it within 30 days.\n", m["sent_to"])
	})
}

func cmdClaim(args []string) {
	fs, g := newFlagSet("claim")
	typ := fs.String("type", "", "record type to create immediately (A, AAAA, CNAME, TXT)")
	content := fs.String("content", "", "record value (e.g. an IP or hostname)")
	host := fs.String("host", "", "optional sub-label (e.g. www)")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains claim <label> [--domain makes.fyi] [--type A --content 1.2.3.4]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"label": pos[0]}
	if g.domain != "" {
		body["domain"] = g.domain
	}
	if *typ != "" {
		body["type"] = *typ
		body["content"] = *content
		body["host"] = *host
	}
	var resp map[string]any
	check(c.Do("POST", "/v1/subdomains", body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Claimed %v\n", m["fqdn"])
		if rec, ok := m["record"].(map[string]any); ok && rec != nil {
			fmt.Printf("  record: %v %v -> %v\n", rec["type"], rec["name"], rec["content"])
		}
	})
}

func cmdList(args []string) {
	fs, g := newFlagSet("list")
	parse(fs, args)
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("GET", "/v1/subdomains", nil, &resp))
	out(g, resp, func(m map[string]any) {
		subs, _ := m["subdomains"].([]any)
		if len(subs) == 0 {
			fmt.Println("(no domains yet — `agentdomains claim <label>`)")
			return
		}
		for _, s := range subs {
			sd := s.(map[string]any)
			recs, _ := sd["records"].([]any)
			fmt.Printf("%-32v  %d record(s)  delegated=%v\n", sd["fqdn"], len(recs), sd["delegated"])
		}
	})
}

func cmdGet(args []string) {
	fs, g := newFlagSet("get")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains get <label> [--domain makes.fyi]")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("GET", resourcePath(pos[0], g, ""), nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("%v (delegated=%v)\n", m["fqdn"], m["delegated"])
		recs, _ := m["records"].([]any)
		for _, r := range recs {
			rec := r.(map[string]any)
			fmt.Printf("  %-6v %v -> %v\n", rec["type"], rec["name"], rec["content"])
		}
	})
}

func cmdRecord(args []string) {
	fs, g := newFlagSet("record")
	typ := fs.String("type", "A", "record type (A, AAAA, CNAME, TXT)")
	content := fs.String("content", "", "record value")
	host := fs.String("host", "", "optional sub-label")
	pos := parse(fs, args)
	if len(pos) < 1 || *content == "" {
		fail("usage: agentdomains record <label> --type A --content 1.2.3.4 [--host www]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"type": *typ, "content": *content, "host": *host}
	var resp map[string]any
	check(c.Do("POST", resourcePath(pos[0], g, "/records"), body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ %v %v -> %v\n", m["type"], m["name"], m["content"])
	})
}

func cmdNS(args []string) {
	fs, g := newFlagSet("ns")
	pos := parse(fs, args)
	if len(pos) < 3 {
		fail("usage: agentdomains ns <label> <ns1> <ns2> [ns3...]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"nameservers": pos[1:]}
	var resp map[string]any
	check(c.Do("PUT", resourcePath(pos[0], g, "/ns"), body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ %v delegated to your nameservers\n", m["fqdn"])
	})
}

func cmdTXT(args []string) {
	fs, g := newFlagSet("txt")
	host := fs.String("host", "", "optional sub-label (e.g. _acme-challenge)")
	pos := parse(fs, args)
	if len(pos) < 2 {
		fail("usage: agentdomains txt <label> <value> [--host _acme-challenge]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"type": "TXT", "content": pos[1], "host": *host}
	var resp map[string]any
	check(c.Do("POST", resourcePath(pos[0], g, "/records"), body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ TXT %v -> %v\n", m["name"], m["content"])
	})
}

func cmdDelete(args []string) {
	fs, g := newFlagSet("delete")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains delete <label> [--domain makes.fyi]")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("DELETE", resourcePath(pos[0], g, ""), nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Deleted %v\n", m["deleted"])
	})
}

func joinAny(items []any) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%v", it))
	}
	return strings.Join(parts, ", ")
}

func orDash(v any) any {
	if v == nil {
		return "—"
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return "—"
	}
	return v
}

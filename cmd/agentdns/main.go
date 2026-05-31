// Command agentdns is the CLI for AgentDNS — free *.makes.fyi subdomains for AI agents.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tashfeenahmed/AgentDNS/internal/client"
	"github.com/tashfeenahmed/AgentDNS/internal/config"
)

const usage = `agentdns — free subdomains under makes.fyi, built for AI agents

USAGE
  agentdns <command> [flags]

COMMANDS
  signup                 Create an account and save the API key locally
  whoami                 Show your account, quota, and usage
  email <address>        Attach an email so a human can validate the account
  claim <label>          Claim <label>.makes.fyi (optionally with a record)
  list                   List your subdomains
  get <label>            Show one subdomain and its records
  record <label>         Add a DNS record to a subdomain
  ns <label> <ns>...     Delegate the subdomain to your own nameservers
  txt <label> <value>    Add a TXT record (e.g. for ACME / SSL challenges)
  delete <label>         Delete a subdomain and its records

GLOBAL FLAGS
  --json                 Emit raw JSON (ideal for agents/scripts)
  --api-url <url>        Override API endpoint (default: https://api.makes.fyi)

Run "agentdns <command> -h" for command-specific flags.`

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
}

// newFlagSet registers the global flags on a per-command flag set.
func newFlagSet(name string) (*flag.FlagSet, *globals) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	g := &globals{}
	fs.BoolVar(&g.json, "json", false, "emit raw JSON")
	fs.StringVar(&g.apiURL, "api-url", "", "override API endpoint")
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

func mustClient(g *globals, needKey bool) (*client.Client, config.Config) {
	cfg := config.Load()
	if g.apiURL != "" {
		cfg.APIURL = g.apiURL
	}
	if needKey && cfg.APIKey == "" {
		fail("no API key found — run `agentdns signup` first (or set AGENTDNS_API_KEY)")
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
		fmt.Println("✓ Account created. API key saved to ~/.agentdns/config.json")
		fmt.Printf("  account: %v\n  quota:   %v subdomain(s)\n", m["account_id"], m["quota"])
		fmt.Println("\n  This is a PROVISIONAL account. Validate within 30 days to keep it:")
		fmt.Println("    agentdns email you@example.com   # then click the link we send")
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
		fmt.Printf("subdomains:     %v / %v used\n", m["used"], m["quota"])
	})
}

func cmdEmail(args []string) {
	fs, g := newFlagSet("email")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdns email <address>")
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
		fail("usage: agentdns claim <label> [--type A --content 1.2.3.4]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"label": pos[0]}
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
			fmt.Println("(no subdomains yet — `agentdns claim <label>`)")
			return
		}
		for _, s := range subs {
			sd := s.(map[string]any)
			recs, _ := sd["records"].([]any)
			fmt.Printf("%-28v  %d record(s)  delegated=%v\n", sd["fqdn"], len(recs), sd["delegated"])
		}
	})
}

func cmdGet(args []string) {
	fs, g := newFlagSet("get")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdns get <label>")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("GET", "/v1/subdomains/"+pos[0], nil, &resp))
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
		fail("usage: agentdns record <label> --type A --content 1.2.3.4 [--host www]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"type": *typ, "content": *content, "host": *host}
	var resp map[string]any
	check(c.Do("POST", "/v1/subdomains/"+pos[0]+"/records", body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ %v %v -> %v\n", m["type"], m["name"], m["content"])
	})
}

func cmdNS(args []string) {
	fs, g := newFlagSet("ns")
	pos := parse(fs, args)
	if len(pos) < 3 {
		fail("usage: agentdns ns <label> <ns1> <ns2> [ns3...]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"nameservers": pos[1:]}
	var resp map[string]any
	check(c.Do("PUT", "/v1/subdomains/"+pos[0]+"/ns", body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ %v delegated to your nameservers\n", m["fqdn"])
	})
}

func cmdTXT(args []string) {
	fs, g := newFlagSet("txt")
	host := fs.String("host", "", "optional sub-label (e.g. _acme-challenge)")
	pos := parse(fs, args)
	if len(pos) < 2 {
		fail("usage: agentdns txt <label> <value> [--host _acme-challenge]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"type": "TXT", "content": pos[1], "host": *host}
	var resp map[string]any
	check(c.Do("POST", "/v1/subdomains/"+pos[0]+"/records", body, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ TXT %v -> %v\n", m["name"], m["content"])
	})
}

func cmdDelete(args []string) {
	fs, g := newFlagSet("delete")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdns delete <label>")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("DELETE", "/v1/subdomains/"+pos[0], nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Deleted %v\n", m["deleted"])
	})
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

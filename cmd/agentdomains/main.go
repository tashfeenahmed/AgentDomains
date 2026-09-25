// Command agentdomains is the CLI for AgentDomains: free domains for the sites
// and APIs AI agents build. Names live under makes.fyi or agentdomains.co.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime/debug"
	"strings"

	"github.com/tashfeenahmed/AgentDomains/internal/client"
	"github.com/tashfeenahmed/AgentDomains/internal/config"
)

// version is stamped at build time with -ldflags "-X main.version=v0.1.2",
// which is how the released archives know their own tag.
var version = ""

// cliVersion is what `agentdomains version` prints. `go install <module>@latest`
// applies no ldflags, so a binary installed that way used to call itself "dev"
// while being a perfectly good release; Go records the module version it was
// built from, and reading it back is more truthful than the fallback.
func cliVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

const usage = `agentdomains — free domains for the sites your AI agents build

USAGE
  agentdomains <command> [flags]

COMMANDS
  signup                 Create an account and save the API key locally
  whoami                 Show your account, quota, usage, and available domains
  email <address>        Attach an email so a human can validate the account
  recover-key <email>    Ask for an API-key reset link by email (no key needed)
  claim <label>          Register <label>.<domain> (needs --email the first time;
                         confirm within 30 days or it's deleted)
  list                   List your domains
  get <label>            Show one domain and its records
  record <label>         Add a DNS record to a domain
  unrecord <label> <id>  Remove one DNS record, keeping the name
  forward <label> <url>  Forward <label>.<domain> to a URL (claims it if needed)
  unforward <label>      Remove the forward, keeping the name
  proxy <label> <host>   Serve a backend at <label>.<domain> over HTTPS — our cert,
                         your origin, no setup on the origin (claims it if needed)
  unproxy <label>        Tear the reverse proxy down, keeping the name
  ns <label> <ns>...     Delegate the domain to your own nameservers
  txt <label> <value>    Add a TXT record (e.g. for ACME / SSL challenges)
  delete <label>         Delete a domain and its records
  account delete         Close your account (--force also deletes names it holds)
  version                Print the CLI version

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
	case "version", "-v", "--version":
		fmt.Printf("agentdomains %s\n", cliVersion())
	case "signup":
		cmdSignup(args)
	case "whoami":
		cmdWhoami(args)
	case "email":
		cmdEmail(args)
	case "recover-key":
		cmdRecoverKey(args)
	case "claim":
		cmdClaim(args)
	case "list":
		cmdList(args)
	case "get":
		cmdGet(args)
	case "record":
		cmdRecord(args)
	case "unrecord":
		cmdUnrecord(args)
	case "forward":
		cmdForward(args)
	case "unforward":
		cmdUnforward(args)
	case "proxy":
		cmdProxy(args)
	case "unproxy":
		cmdUnproxy(args)
	case "ns":
		cmdNS(args)
	case "txt":
		cmdTXT(args)
	case "delete":
		cmdDelete(args)
	case "account":
		cmdAccount(args)
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
	// cliVersion also names the client in the User-Agent, so `agentdomains
	// version` and what the API sees can never disagree.
	return client.New(cfg.APIURL, cfg.APIKey, cliVersion()), cfg
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		failAPI(err)
	}
}

// failAPI ends the program on an API error, adding the one thing the message
// itself can't say: whether coming back later would help. The API marks an
// upstream failure retry:true when it is an outage, and retry:false when it is
// a misconfiguration on our side that no amount of waiting fixes.
func failAPI(err error) {
	failAPIWith(err, "")
}

// failAPIWith is failAPI for commands that look a name up: notFound is the
// hint to attach when the API answers 404 for the name itself (see
// nameNotFoundHint). Commands that don't look up a name pass "".
func failAPIWith(err error, notFound string) {
	var api *client.APIError
	if errors.As(err, &api) {
		if hint := apiHint(api, notFound); hint != "" {
			fail(api.Message + "\n  " + hint)
		}
	}
	fail(err.Error())
}

// checkName is check for the commands that address a name by label (get,
// delete): a 404 there means the name wasn't found, so it gets the hint.
func checkName(err error, g *globals) {
	if err != nil {
		failAPIWith(err, nameNotFoundHint(g.domain))
	}
}

// apiHint turns the statuses an agent hits most into the next thing to do.
// A bare "request failed (401)" sends an agent retrying the same call forever;
// the hint names the recovery instead. notFound is attached to a 404 only when
// the caller looked up a name; a 404 from a record/forward/proxy lookup or an
// unknown endpoint says nothing about the name, so it gets no hint. Keep this
// pure — main_test.go pins it.
func apiHint(api *client.APIError, notFound string) string {
	if retry, present := api.Flag("retry"); present {
		if retry {
			return "(temporary — worth retrying in a moment)"
		}
		return "(not retryable — this one is on our side; retrying will not help)"
	}
	switch api.Status {
	case 401:
		// Not "run signup": a new account doesn't own the names the old key held.
		return "the API key was rejected — check AGENTDOMAINS_API_KEY (or ~/.agentdomains/config.json).\n" +
			"  Lost the key? agentdomains recover-key <verified email> sends a reset link.\n" +
			"  Signing up again would not own your names."
	case 404:
		if notFound != "" && !strings.HasPrefix(api.Message, "no such endpoint") {
			return notFound
		}
	case 429:
		if secs, ok := api.Body["retry_after"].(float64); ok && secs > 0 {
			return fmt.Sprintf("rate limited — retry in about %d second(s)", int(secs))
		}
		return "rate limited — wait a minute before retrying"
	}
	return ""
}

// nameNotFoundHint is the 404 hint for a name lookup. Without --domain the
// server already searched every domain, so there is nowhere else to point;
// with --domain, the name may live under the other one.
func nameNotFoundHint(domain string) string {
	h := "`agentdomains list` shows the names you hold"
	if other := otherDomain(domain); other != "" {
		h += fmt.Sprintf(" — or it may be under --domain %s", other)
	}
	return h
}

func otherDomain(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "makes.fyi":
		return "agentdomains.co"
	case "agentdomains.co":
		return "makes.fyi"
	}
	return ""
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
		q := quotaText(m["quota"])
		if q != "unlimited" {
			q += " domain(s)"
		}
		fmt.Printf("  account: %v\n  quota:   %s\n", m["account_id"], q)
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
		fmt.Printf("domains used:   %v / %s\n", m["used"], quotaOf(m))
		if cap, ok := m["max_subdomains"].(float64); ok && cap > 0 {
			fmt.Printf("per-account cap: %d name(s) at once\n", int(cap))
		}
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

// cmdRecoverKey asks the API for a key-reset link for the account whose
// verified email is <email>. It deliberately needs NO API key — that is the
// whole point: this is the command you run after you have lost the key. The
// email carries a link; opening and confirming it (a human click) rotates the
// key, so this command can only ever start the process, never finish it.
func cmdRecoverKey(args []string) {
	fs, g := newFlagSet("recover-key")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains recover-key <email>\n  the verified email on the account; no API key is needed")
	}
	_, cfg := mustClient(g, false)
	// Send no key, even if one is configured: the endpoint takes none, and the
	// configured key is typically the stale one that got us here.
	c := client.New(cfg.APIURL, "", cliVersion())
	var resp map[string]any
	check(c.Do("POST", "/v1/account/key/recover", map[string]any{"email": pos[0]}, &resp))
	out(g, resp, func(m map[string]any) { printRecoverKey(os.Stdout, m) })
}

// printRecoverKey renders the recover-key answer. The server's note already
// says to open the link and confirm, so it is shown as-is rather than repeated;
// the fallback only covers a server that sends no note.
func printRecoverKey(w io.Writer, m map[string]any) {
	note, _ := m["note"].(string)
	if note == "" {
		note = "If a verified account uses that email, a one-time reset link is on its way. " +
			"Open it and confirm to get a new API key (a human click is required)."
	}
	fmt.Fprintf(w, "✓ %s\n", note)
	fmt.Fprintln(w, "  Then point the CLI at the new key: export AGENTDOMAINS_API_KEY=<new key>")
	fmt.Fprintln(w, "  and verify with: agentdomains whoami")
}

func cmdClaim(args []string) {
	fs, g := newFlagSet("claim")
	email := fs.String("email", "", "your email — required the first time you register a name; we send a confirmation link")
	typ := fs.String("type", "", "record type to create immediately (A, AAAA, CNAME, TXT)")
	content := fs.String("content", "", "record value (e.g. an IP or hostname)")
	host := fs.String("host", "", "optional sub-label (e.g. www)")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains claim <label> --email you@example.com [--domain makes.fyi] [--type A --content 1.2.3.4]")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"label": pos[0]}
	if g.domain != "" {
		body["domain"] = g.domain
	}
	if *email != "" {
		body["email"] = *email
	}
	if *typ != "" {
		body["type"] = *typ
		body["content"] = *content
		body["host"] = *host
	}
	var resp map[string]any
	if err := c.Do("POST", "/v1/subdomains", body, &resp); err != nil {
		// Re-claiming a name you already hold is a 409 like any other, but it is
		// the one 409 that means "carry on": the name is yours. Saying so and
		// exiting 0 makes claim safe to run twice, which is how an agent that
		// lost its place actually behaves.
		var api *client.APIError
		if errors.As(err, &api) {
			if owned, _ := api.Flag("owned"); owned {
				if g.json {
					b, _ := json.MarshalIndent(api.Body, "", "  ")
					fmt.Println(string(b))
					return
				}
				fmt.Printf("✓ You already own %v — nothing to do.\n", api.Body["fqdn"])
				return
			}
		}
		failAPI(err)
	}
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Registered %v\n", m["fqdn"])
		if rec, ok := m["record"].(map[string]any); ok && rec != nil {
			fmt.Printf("  record: %v %v -> %v  (id %v)\n", rec["type"], rec["name"], rec["content"], rec["id"])
		}
		if note, ok := m["note"].(string); ok && note != "" {
			fmt.Printf("  ✉ %s\n", note)
		}
		// Printed on the one command that proves the thing works: a name is
		// registered and nobody has been asked for a card. out() has already
		// returned by the --json path, so machine output stays clean.
		fmt.Println("  Free, no card. How it compares: https://agentdomains.co/compare")
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
			if f, ok := sd["forward"].(map[string]any); ok && f != nil {
				fmt.Printf("%-32v  forward -> %v\n", sd["fqdn"], f["target"])
				continue
			}
			if p, ok := sd["proxy"].(map[string]any); ok && p != nil {
				fmt.Printf("%-32v  proxy -> %v\n", sd["fqdn"], p["origin"])
				continue
			}
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
	checkName(c.Do("GET", resourcePath(pos[0], g, ""), nil, &resp), g)
	out(g, resp, func(m map[string]any) {
		fmt.Printf("%v (delegated=%v)\n", m["fqdn"], m["delegated"])
		if f, ok := m["forward"].(map[string]any); ok && f != nil {
			code := 302
			if c, ok := f["code"].(float64); ok {
				code = int(c)
			}
			fmt.Printf("  FWD    %v -> %v (%d)\n", m["fqdn"], f["target"], code)
		}
		if p, ok := m["proxy"].(map[string]any); ok && p != nil {
			fmt.Printf("  PROXY  %v -> %v\n", m["fqdn"], p["origin"])
		}
		recs, _ := m["records"].([]any)
		for _, r := range recs {
			rec := r.(map[string]any)
			// The id is here because `unrecord` needs it, and this is the only
			// place a caller can read it off.
			fmt.Printf("  %-6v %v -> %v  (id %v)\n", rec["type"], rec["name"], rec["content"], rec["id"])
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
		fmt.Printf("✓ %v %v -> %v  (id %v)\n", m["type"], m["name"], m["content"], m["id"])
		fmt.Printf("  remove it with: agentdomains unrecord %s %v\n", pos[0], m["id"])
	})
}

// cmdUnrecord removes a single record and keeps the name. Before the API had
// this, undoing one record meant deleting the whole domain and claiming it back
// — and hoping nobody took it in between.
func cmdUnrecord(args []string) {
	fs, g := newFlagSet("unrecord")
	pos := parse(fs, args)
	if len(pos) < 2 {
		fail("usage: agentdomains unrecord <label> <record-id> [--domain makes.fyi]\n" +
			"  record ids are shown by `agentdomains get <label>`")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("DELETE", resourcePath(pos[0], g, "/records/"+url.PathEscape(pos[1])), nil, &resp))
	out(g, resp, func(m map[string]any) {
		rec, _ := m["deleted"].(map[string]any)
		fmt.Printf("✓ Removed %v %v -> %v from %v\n", rec["type"], rec["name"], rec["content"], m["fqdn"])
	})
}

func cmdForward(args []string) {
	fs, g := newFlagSet("forward")
	email := fs.String("email", "", "your email — required if this also claims a new name on an account with no email yet")
	permanent := fs.Bool("permanent", false, "use a 301 permanent redirect (default: 302 temporary)")
	temporary := fs.Bool("temporary", false, "use a 302 temporary redirect (the default)")
	noPreservePath := fs.Bool("no-preserve-path", false, "always land on the target root, ignoring the request path/query")
	cloak := fs.Bool("cloak", false, "keep your domain in the address bar and load the target in a frame (discouraged)")
	pos := parse(fs, args)
	if len(pos) < 2 {
		fail("usage: agentdomains forward <label> <url> [--email you@example.com] [--permanent] [--no-preserve-path] [--cloak]")
	}
	if *permanent && *temporary {
		fail("--permanent and --temporary are mutually exclusive")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{
		"target":        pos[1],
		"permanent":     *permanent,
		"preserve_path": !*noPreservePath,
		"cloak":         *cloak,
	}
	if g.domain != "" {
		body["domain"] = g.domain
	}
	if *email != "" {
		body["email"] = *email
	}
	var resp map[string]any
	check(c.Do("PUT", resourcePath(pos[0], g, "/forward"), body, &resp))
	out(g, resp, func(m map[string]any) {
		f, _ := m["forward"].(map[string]any)
		kind := "→ (302 temporary"
		if code, ok := f["code"].(float64); ok && int(code) == 301 {
			kind = "→ (301 permanent"
		}
		if pp, ok := f["preserve_path"].(bool); ok && pp {
			kind += ", path preserved"
		}
		if cl, ok := f["cloak"].(bool); ok && cl {
			kind += ", cloaked"
		}
		kind += ")"
		fmt.Printf("✓ %v %s %v\n", m["fqdn"], kind, f["target"])
		printReplaced(m, "forward")
		fmt.Println("  DNS is live within seconds; HTTPS may take a minute on first use.")
		if note, ok := m["note"].(string); ok && note != "" {
			fmt.Printf("  ✉ %s\n", note)
		}
	})
}

func cmdUnforward(args []string) {
	fs, g := newFlagSet("unforward")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains unforward <label> [--domain makes.fyi]")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("DELETE", resourcePath(pos[0], g, "/forward"), nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Removed forward on %v\n", m["fqdn"])
	})
}

func cmdProxy(args []string) {
	fs, g := newFlagSet("proxy")
	email := fs.String("email", "", "your email — required if this also claims a new name on an account with no email yet")
	pos := parse(fs, args)
	if len(pos) < 2 {
		fail("usage: agentdomains proxy <label> <origin-host> [--email you@example.com] [--domain makes.fyi]\n" +
			"  e.g. agentdomains proxy shop myapp.fly.dev")
	}
	c, _ := mustClient(g, true)
	body := map[string]any{"origin": pos[1]}
	if g.domain != "" {
		body["domain"] = g.domain
	}
	if *email != "" {
		body["email"] = *email
	}
	var resp map[string]any
	check(c.Do("PUT", resourcePath(pos[0], g, "/proxy"), body, &resp))
	out(g, resp, func(m map[string]any) {
		p, _ := m["proxy"].(map[string]any)
		fmt.Printf("✓ %v serves %v (reverse proxy, HTTPS at our edge)\n", m["fqdn"], p["origin"])
		printReplaced(m, "proxy")
		fmt.Println("  DNS is live within seconds; HTTPS may take a minute on first use.")
		fmt.Println("  Note: apps that hardcode their own hostname (e.g. OAuth logins) may")
		fmt.Println("  need that hostname added on their side for every flow to work.")
		if note, ok := m["note"].(string); ok && note != "" {
			fmt.Printf("  ✉ %s\n", note)
		}
	})
}

func cmdUnproxy(args []string) {
	fs, g := newFlagSet("unproxy")
	pos := parse(fs, args)
	if len(pos) < 1 {
		fail("usage: agentdomains unproxy <label> [--domain makes.fyi]")
	}
	c, _ := mustClient(g, true)
	var resp map[string]any
	check(c.Do("DELETE", resourcePath(pos[0], g, "/proxy"), nil, &resp))
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Removed proxy on %v\n", m["fqdn"])
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
	checkName(c.Do("DELETE", resourcePath(pos[0], g, ""), nil, &resp), g)
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Deleted %v\n", m["deleted"])
	})
}

// cmdAccount handles the one command that ends everything: `account delete`.
// It is two words on purpose — closing an account is not something to fire off
// by mistyping a one-word verb.
func cmdAccount(args []string) {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fail("usage: agentdomains account delete [--force]")
	}
	switch args[0] {
	case "delete", "close":
		cmdAccountDelete(args[1:])
	default:
		fail(fmt.Sprintf("unknown account command %q (only `account delete` exists)", args[0]))
	}
}

// cmdAccountDelete closes the account and invalidates its API key. The server
// refuses while names are still held unless --force is passed, which deletes
// them too — the refusal exists so an agent tidying up cannot silently take a
// live hostname down with it.
func cmdAccountDelete(args []string) {
	fs, g := newFlagSet("account delete")
	force := fs.Bool("force", false, "also delete every name the account still holds (they stop resolving immediately)")
	parse(fs, args)
	c, cfg := mustClient(g, true)

	path := "/v1/account"
	if *force {
		path += "?force=true"
	}
	var resp map[string]any
	if err := c.Do("DELETE", path, nil, &resp); err != nil {
		var api *client.APIError
		if errors.As(err, &api) && api.Status == 409 {
			// The names are listed in the body; repeating the instruction here
			// saves the caller a second guess about the flag's name.
			fail(api.Message + "\n  i.e. agentdomains account delete --force")
		}
		failAPI(err)
	}
	// The key is dead now; leaving it in the config file only produces 401s.
	cfg.APIKey, cfg.AccountID = "", ""
	_ = config.Save(cfg)
	out(g, resp, func(m map[string]any) {
		fmt.Printf("✓ Account %v deleted (%v name(s) removed). The saved API key was cleared.\n",
			m["account_id"], m["subdomains_deleted"])
	})
}

// printReplaced reports the address records a forward or proxy displaced. The
// call deletes any A/AAAA/CNAME sitting at the label itself — that is how it
// takes over the hostname — and a caller who is not told loses records without
// noticing.
func printReplaced(m map[string]any, mode string) {
	replaced, _ := m["replaced_records"].([]any)
	if len(replaced) == 0 {
		return
	}
	fmt.Printf("  %d record(s) replaced by the %s:\n", len(replaced), mode)
	for _, r := range replaced {
		rec, ok := r.(map[string]any)
		if !ok {
			continue
		}
		fmt.Printf("    %v %v -> %v\n", rec["type"], rec["name"], rec["content"])
	}
}

// quotaOf renders the quota line from a whoami body. The server omits `quota`
// entirely when quotas are off (reporting "quota":0 read as "you may hold zero
// names"), so its absence — or an unlimited flag — is the unlimited case.
func quotaOf(m map[string]any) string {
	if unlimited, ok := m["unlimited"].(bool); ok && unlimited {
		return "unlimited"
	}
	if _, ok := m["quota"]; !ok {
		return "unlimited"
	}
	return quotaText(m["quota"])
}

// quotaText renders a quota value: "unlimited" when it's 0 or less (quotas
// disabled), otherwise the number. JSON numbers arrive as float64.
func quotaText(v any) string {
	if f, ok := v.(float64); ok {
		if f <= 0 {
			return "unlimited"
		}
		return fmt.Sprintf("%d", int(f))
	}
	return fmt.Sprintf("%v", v)
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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// cmdUpgrade asks the API for a Stripe Checkout link for AgentDomains Pro and
// prints it. When a human is at the terminal it also opens the browser; an
// agent (no TTY, or --json) just gets the link to hand to its human, since
// paying is something only a person can do. An account that is already Pro
// gets its billing-portal link instead, so running this twice can never start
// a second subscription.
func cmdUpgrade(args []string) {
	fs, g := newFlagSet("upgrade")
	yearly := fs.Bool("yearly", false, "pay $48/year instead of $5/month")
	noOpen := fs.Bool("no-open", false, "print the link without opening a browser")
	parse(fs, args)
	c, _ := mustClient(g, true)

	interval := "month"
	if *yearly {
		interval = "year"
	}
	var resp map[string]any
	check(c.Do("POST", "/v1/billing/checkout", map[string]any{"interval": interval}, &resp))
	out(g, resp, func(m map[string]any) {
		link, _ := m["url"].(string)
		if m["kind"] == "portal" {
			fmt.Println("✓ This account is already on Pro. Manage it here:")
		} else {
			price := "$5/month"
			if m["interval"] == "year" {
				price = "$48/year"
			}
			fmt.Printf("AgentDomains Pro (%s): 100 names, never released for being unreachable, priority support.\n", price)
			fmt.Println("Open this link to pay (a human has to do this part):")
		}
		fmt.Printf("\n  %s\n\n", link)
		if m["kind"] != "portal" {
			fmt.Println("The account switches to Pro as soon as payment clears; `agentdomains whoami` shows the plan.")
		}
		if !*noOpen && isTerminal() && link != "" {
			_ = openBrowser(link)
		}
	})
}

// cmdBilling prints (and on a terminal opens) the Stripe Customer Portal for
// an account that has subscribed: card, invoices, monthly/yearly, cancel.
func cmdBilling(args []string) {
	fs, g := newFlagSet("billing")
	noOpen := fs.Bool("no-open", false, "print the link without opening a browser")
	parse(fs, args)
	c, _ := mustClient(g, true)

	var resp map[string]any
	check(c.Do("POST", "/v1/billing/portal", nil, &resp))
	out(g, resp, func(m map[string]any) {
		link, _ := m["url"].(string)
		fmt.Printf("Manage your AgentDomains subscription:\n\n  %s\n\n", link)
		fmt.Println("Cancelling keeps Pro until the end of the period you paid for. Your names are never deleted by a downgrade.")
		if !*noOpen && isTerminal() && link != "" {
			_ = openBrowser(link)
		}
	})
}

// isTerminal reports whether stdout is an interactive terminal, which is the
// signal that a person (not an agent) ran the command.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// openBrowser opens a URL with the platform's default handler. Failure is
// harmless: the link has already been printed.
func openBrowser(link string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", link)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		cmd = exec.Command("xdg-open", link)
	}
	return cmd.Start()
}

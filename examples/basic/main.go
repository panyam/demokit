// Basic example showcasing demokit with both plain and TUI rendering modes.
//
// Run with default (plain) renderer:
//
//	go run ./examples/basic/
//
// Run with TUI renderer (styled boxes + smooth scroll):
//
//	go run ./examples/basic/ --tui
//
// Run plain with smooth scroll:
//
//	go run ./examples/basic/ --smooth
//
// Run non-interactively (no pauses):
//
//	go run ./examples/basic/ --non-interactive
//	go run ./examples/basic/ --tui --non-interactive
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/tui"
)

func main() {
	demo := demokit.New("Token Exchange Flow").
		Description("How a client obtains and uses an access token").
		Dir("basic").
		Actors(
			demokit.Actor("Client", "Client App"),
			demokit.Actor("AS", "Auth Server"),
			demokit.Actor("API", "Resource API"),
		)

	demo.Section("Overview",
		"This example walks through a simplified OAuth-style token exchange.",
		"",
		"The client registers, obtains a token, then calls a protected API.",
	)

	demo.Step("Register the client").
		Arrow("Client", "AS", "POST /register").
		DashedArrow("AS", "Client", "{client_id, client_secret}").
		Ref(demokit.Ref{
			Name: "RFC 6749 §2",
			URL:  "https://www.rfc-editor.org/rfc/rfc6749#section-2",
		}).
		Note("The auth server issues credentials that the client will use to authenticate.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("Registered client: app-demo-001")
			fmt.Println("  client_id:     app-demo-001")
			fmt.Println("  client_secret: ********")
			return
		})

	demo.Step("Request an access token").
		Arrow("Client", "AS", "POST /token (client_credentials)").
		DashedArrow("AS", "Client", "{access_token, expires_in}").
		Ref(demokit.Ref{
			Name: "RFC 6749 §4.4",
			URL:  "https://www.rfc-editor.org/rfc/rfc6749#section-4.4",
		}).
		Note("Using the client_credentials grant, the client exchanges its credentials for a bearer token.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("Token response:")
			fmt.Println("  access_token: eyJhbGci...truncated")
			fmt.Println("  token_type:   Bearer")
			fmt.Println("  expires_in:   3600")
			return
		})

	demo.Step("Call a protected API").
		Arrow("Client", "API", "GET /users/me (Bearer token)").
		DashedArrow("API", "AS", "Validate token").
		DashedArrow("AS", "API", "Token valid").
		DashedArrow("API", "Client", "{user profile}").
		Note("The API validates the token with the auth server before returning data.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("API response (200 OK):")
			fmt.Println("  {")
			fmt.Println(`    "id": "user-42",`)
			fmt.Println(`    "name": "Alice",`)
			fmt.Println(`    "email": "alice@example.com"`)
			fmt.Println("  }")
			return
		})

	demo.Step("Refresh with expired token").
		Arrow("Client", "API", "GET /users/me (expired token)").
		DashedArrow("API", "Client", "401 Unauthorized").
		Note("Demonstrates error handling when a token has expired.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("API response (401 Unauthorized):")
			fmt.Println(`  {"error": "token_expired"}`)
			return demokit.Errf("token expired, need to refresh")
		})

	demo.Step("Retry with backoff").
		Arrow("Client", "AS", "POST /token (refresh)").
		DashedArrow("AS", "Client", "429 Too Many Requests").
		Note("Demonstrates warning when rate-limited.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("Rate limited — will retry in 2s")
			return demokit.Warn("rate limited, backing off")
		})

	demo.Step("Token refreshed (cached)").
		Arrow("Client", "AS", "POST /token (refresh)").
		DashedArrow("AS", "Client", "{new_access_token}").
		Note("Demonstrates info result for cache hits.").
		Run(func() (result *demokit.StepResult) {
			fmt.Println("New token issued")
			return demokit.Info("served from token cache")
		})

	demo.Section("What happened",
		"1. The client registered and received credentials.",
		"2. It exchanged those credentials for a short-lived access token.",
		"3. It used that token to call a protected API endpoint.",
		"4. The token expired — the API returned 401 (shown as Error).",
		"5. Refresh was rate-limited (shown as Warning).",
		"6. Token was served from cache (shown as Info).",
		"",
		"In production, tokens expire and must be refreshed — but that's a story for another demo.",
	)

	// Parse display flags.
	useTUI := false
	smooth := false
	for _, arg := range os.Args[1:] {
		switch strings.TrimSpace(arg) {
		case "--tui":
			useTUI = true
		case "--smooth":
			smooth = true
		}
	}

	if useTUI {
		demo.WithRenderer(tui.New())
	} else if smooth {
		demo.WithRenderer(&demokit.PlainRenderer{Delay: 18 * time.Millisecond})
	}

	demo.Execute()
}

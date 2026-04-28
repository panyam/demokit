// Basic example showcasing demokit with both plain and TUI rendering modes.
//
// Run with default (plain) renderer:
//
//	go run ./examples/basic/
//
// Run with TUI renderer:
//
//	go run ./examples/basic/ --tui
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
		Run(func() {
			fmt.Println("Registered client: app-demo-001")
			fmt.Println("  client_id:     app-demo-001")
			fmt.Println("  client_secret: ********")
		})

	demo.Step("Request an access token").
		Arrow("Client", "AS", "POST /token (client_credentials)").
		DashedArrow("AS", "Client", "{access_token, expires_in}").
		Ref(demokit.Ref{
			Name: "RFC 6749 §4.4",
			URL:  "https://www.rfc-editor.org/rfc/rfc6749#section-4.4",
		}).
		Note("Using the client_credentials grant, the client exchanges its credentials for a bearer token.").
		Run(func() {
			fmt.Println("Token response:")
			fmt.Println("  access_token: eyJhbGci...truncated")
			fmt.Println("  token_type:   Bearer")
			fmt.Println("  expires_in:   3600")
		})

	demo.Step("Call a protected API").
		Arrow("Client", "API", "GET /users/me (Bearer token)").
		DashedArrow("API", "AS", "Validate token").
		DashedArrow("AS", "API", "Token valid").
		DashedArrow("API", "Client", "{user profile}").
		Note("The API validates the token with the auth server before returning data.").
		Run(func() {
			fmt.Println("API response (200 OK):")
			fmt.Println("  {")
			fmt.Println(`    "id": "user-42",`)
			fmt.Println(`    "name": "Alice",`)
			fmt.Println(`    "email": "alice@example.com"`)
			fmt.Println("  }")
		})

	demo.Section("What happened",
		"1. The client registered and received credentials.",
		"2. It exchanged those credentials for a short-lived access token.",
		"3. It used that token to call a protected API endpoint.",
		"",
		"In production, tokens expire and must be refreshed — but that's a story for another demo.",
	)

	// Use TUI renderer if --tui flag is passed.
	for _, arg := range os.Args[1:] {
		if strings.TrimSpace(arg) == "--tui" {
			demo.WithRenderer(tui.New())
			break
		}
	}

	demo.Execute()
}

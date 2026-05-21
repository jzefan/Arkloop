// oauth-seed registers (or rotates the secret of) an OAuth/OIDC client.
//
// Usage:
//
//	# First time: print a freshly generated client_secret to stdout, save it!
//	oauth-seed -dsn "$ARKLOOP_DATABASE_URL" \
//	           -client-id exam-web \
//	           -name "Exam Backend" \
//	           -redirect "https://exam.example.com/api/auth/oidc/callback" \
//	           -scopes "openid,profile,email,offline_access,exam:read,exam:write,exam:admin"
//
//	# Rotate the secret (re-print, old secret invalidated)
//	oauth-seed -dsn "$ARKLOOP_DATABASE_URL" -client-id exam-web -rotate
//
// We deliberately do not provide a delete subcommand here; soft-delete via
// the (yet-to-be-built) admin UI is the supported path.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"arkloop/services/api/internal/data"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	var (
		dsn         = flag.String("dsn", os.Getenv("ARKLOOP_DATABASE_URL"), "Postgres DSN")
		clientID    = flag.String("client-id", "", "client_id (e.g. exam-web)")
		name        = flag.String("name", "", "display name shown on the consent page")
		redirectCSV = flag.String("redirect", "", "comma-separated list of redirect_uris (exact match)")
		scopesCSV   = flag.String("scopes", "openid,profile,email,offline_access", "comma-separated allowed scopes")
		clientType  = flag.String("client-type", "confidential", "confidential | public")
		requirePKCE = flag.Bool("require-pkce", true, "require PKCE S256")
		rotate      = flag.Bool("rotate", false, "rotate the client_secret of an existing client (preserves other fields)")
		idempotent  = flag.Bool("idempotent", false, "skip insertion (exit 0) when a non-deleted row with this client_id already exists; useful for re-running from deploy scripts")
	)
	flag.Parse()

	if *dsn == "" {
		log.Fatal("dsn required (-dsn or ARKLOOP_DATABASE_URL)")
	}
	if *clientID == "" {
		log.Fatal("-client-id required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, data.NormalizePostgresDSN(*dsn))
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	repo, err := data.NewOAuthClientRepository(pool)
	if err != nil {
		log.Fatalf("repo: %v", err)
	}

	secret, err := generateSecret()
	if err != nil {
		log.Fatalf("generate secret: %v", err)
	}
	secretHash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash secret: %v", err)
	}

	if *rotate {
		// Soft-delete the old row, then re-insert with the same fields and
		// a fresh secret. The two-row approach keeps history for audit.
		existing, err := repo.GetByClientID(ctx, *clientID)
		if err != nil || existing == nil {
			log.Fatalf("client %q not found (cannot rotate)", *clientID)
		}
		if err := repo.SoftDelete(ctx, existing.ID); err != nil {
			log.Fatalf("soft delete: %v", err)
		}
		if _, err := repo.Create(ctx, existing.ClientID, string(secretHash), existing.ClientType,
			existing.Name, existing.RedirectURIs, existing.AllowedScopes, existing.RequirePKCE); err != nil {
			log.Fatalf("re-insert: %v", err)
		}
		printSecret(*clientID, secret, true)
		return
	}

	if *name == "" || *redirectCSV == "" {
		log.Fatal("-name and -redirect are required for new clients")
	}

	// Idempotent mode: bail out cleanly when the client already exists. This
	// matters for re-runs from the deploy script — we want a green exit and
	// no spurious "client_id already exists" error in the log.
	if *idempotent {
		existing, err := repo.GetByClientID(ctx, *clientID)
		if err != nil {
			log.Fatalf("idempotent precheck: %v", err)
		}
		if existing != nil {
			fmt.Printf("OAuth client %q already exists; skipped (idempotent).\n", *clientID)
			return
		}
	}

	redirects := splitCSV(*redirectCSV)
	scopes := splitCSV(*scopesCSV)
	if _, err := repo.Create(ctx, *clientID, string(secretHash), *clientType, *name, redirects, scopes, *requirePKCE); err != nil {
		log.Fatalf("insert: %v", err)
	}
	printSecret(*clientID, secret, false)
}

func generateSecret() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func printSecret(clientID, secret string, rotated bool) {
	action := "created"
	if rotated {
		action = "rotated"
	}
	fmt.Printf("\n────────────────────────────────────────────────────────────\n")
	fmt.Printf("  OAuth client %s: %s\n", action, clientID)
	fmt.Printf("  client_secret (save this — it will not be shown again):\n\n")
	fmt.Printf("      %s\n\n", secret)
	fmt.Printf("  Configure your OAuth client with:\n")
	fmt.Printf("      client_id=%s\n", clientID)
	fmt.Printf("      client_secret=%s\n", secret)
	fmt.Printf("────────────────────────────────────────────────────────────\n")
}

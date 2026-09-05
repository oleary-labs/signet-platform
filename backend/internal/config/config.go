package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, loaded from the environment.
type Config struct {
	Port        string
	Env         string
	CORSOrigins []string

	DatabaseURL string

	// Signing secret for platform session tokens. Rotating it logs everyone out,
	// which is the intended blast radius for a compromised key.
	SessionSecret string
	// Session lifetime in seconds.
	SessionTTL int
	// Cookie domain; empty means host-only, which is right for local dev.
	CookieDomain string
	// Send the session cookie only over TLS. Defaults to on outside development.
	CookieSecure bool

	PublicWebURL string

	// ── Chain / protocol wiring ────────────────────────────────────────────
	ChainID        int64
	RPCURL         string
	FactoryAddress string
	// SignetAccountFactory, which the console uses to derive the
	// counterfactual address of a developer's smart account.
	AccountFactoryAddress string
	EntryPointAddress     string
	BundlerURL            string
	BootstrapGroup        string
	BootstrapNodes        []string
	// SIWE domain checked against the ERC-4361 message. Empty disables the
	// SIWE login route rather than accepting any domain.
	SIWEDomain string

	// ERC-7677 paymaster that sponsors group creation. O'Leary Labs runs it,
	// so a developer's first group costs them nothing to deploy.
	PaymasterURL     string
	PaymasterAddress string
	// Whether to sponsor group creation at all. On by default: the first group
	// is the step where an empty wallet is most likely to stop someone.
	SponsorGroupCreation bool

	// Key the platform signs node auth-key certificates with. Its public half
	// must be registered on the platform's signing group with addAuthKey, or
	// the nodes reject every certificate it issues.
	//
	// This is what lets a developer who signed in with a wallet hold a Signet
	// key: the platform verifies their SIWE signature, then vouches for them to
	// its own group. Whoever holds this key can mint a session for any identity
	// in that group, so treat it as the hot key it is.
	PlatformAuthKey string
	// Bundler API key. The bundler accepts UserOperations only from callers
	// presenting it, so sponsorship cannot be drained by anyone who can reach
	// the endpoint.
	BundlerAPIKey string

	// ── Storage ────────────────────────────────────────────────────────────
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3PublicURL      string
	S3ForcePathStyle bool

	// When false (the default), uploads land on the local filesystem under
	// LocalUploadDir and are served by this server, so development needs no S3.
	InProduction   bool
	LocalUploadDir string

	// Shared secret for the metering ingest endpoint (X-Ingest-Key). Empty
	// disables ingest entirely rather than leaving it open.
	IngestKey string

	// Emails granted platform-staff rights on boot and at login. Staff curate
	// the node-operator marketplace; they are not org admins by default.
	StaffEmails []string

	// Node health probing interval in seconds; 0 disables the prober.
	HealthProbeSecs int
	// Chain sync interval in seconds; 0 disables the chain indexer.
	ChainSyncSecs int
}

// Load reads .env (if present) and the process environment.
func Load() *Config {
	_ = godotenv.Load()

	env := get("ENV", "development")
	inProd := get("IN_PRODUCTION", "false") == "true"

	c := &Config{
		Port:        get("PORT", "8080"),
		Env:         env,
		CORSOrigins: splitList(get("CORS_ORIGINS", "http://localhost:3000")),

		DatabaseURL: must("DATABASE_URL"),

		SessionSecret: must("SESSION_SECRET"),
		SessionTTL:    getInt("SESSION_TTL_SECONDS", 60*60*12),
		CookieDomain:  os.Getenv("COOKIE_DOMAIN"),
		CookieSecure:  get("COOKIE_SECURE", boolStr(inProd)) == "true",

		PublicWebURL: get("PUBLIC_WEB_URL", "http://localhost:3000"),

		ChainID:               int64(getInt("CHAIN_ID", 31337)),
		RPCURL:                get("RPC_URL", "http://127.0.0.1:8545"),
		FactoryAddress:        os.Getenv("FACTORY_ADDRESS"),
		AccountFactoryAddress: os.Getenv("ACCOUNT_FACTORY_ADDRESS"),
		EntryPointAddress:     os.Getenv("ENTRYPOINT_ADDRESS"),
		BundlerURL:            get("BUNDLER_URL", "http://127.0.0.1:4337"),
		BootstrapGroup:        os.Getenv("BOOTSTRAP_GROUP"),
		BootstrapNodes:        splitList(os.Getenv("BOOTSTRAP_NODES")),
		SIWEDomain:            os.Getenv("SIWE_DOMAIN"),

		PaymasterURL:         os.Getenv("PAYMASTER_URL"),
		PaymasterAddress:     os.Getenv("PAYMASTER_ADDRESS"),
		SponsorGroupCreation: get("SPONSOR_GROUP_CREATION", "true") == "true",
		PlatformAuthKey:      os.Getenv("PLATFORM_AUTH_KEY"),
		BundlerAPIKey:        os.Getenv("BUNDLER_API_KEY"),

		S3Endpoint:       os.Getenv("S3_ENDPOINT"),
		S3Region:         get("S3_REGION", "us-east-1"),
		S3Bucket:         get("S3_BUCKET", "signet-platform-assets"),
		S3AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:      os.Getenv("S3_SECRET_KEY"),
		S3PublicURL:      os.Getenv("S3_PUBLIC_URL"),
		S3ForcePathStyle: get("S3_FORCE_PATH_STYLE", "true") == "true",

		InProduction:   inProd,
		LocalUploadDir: get("LOCAL_UPLOAD_DIR", "uploads"),

		IngestKey: os.Getenv("INGEST_KEY"),

		StaffEmails: lowerList(splitList(os.Getenv("STAFF_EMAILS"))),

		HealthProbeSecs: getInt("HEALTH_PROBE_SECONDS", 120),
		ChainSyncSecs:   getInt("CHAIN_SYNC_SECONDS", 60),
	}
	return c
}

// IsDev reports whether the server is running in a development environment,
// where a few conveniences (local uploads, permissive cookies) are enabled.
func (c *Config) IsDev() bool { return !c.InProduction }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("config: %s=%q is not an integer, using %d", k, v, def)
		return def
	}
	return n
}

func must(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("config: required env var %s is not set", k)
	}
	return v
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func lowerList(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

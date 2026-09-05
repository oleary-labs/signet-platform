// Command seed populates a development database with a node-operator
// directory and a demo organization, so the console has something to render
// before a devnet is wired up.
//
// It is idempotent: every write is an upsert keyed on a stable identifier, so
// running it twice changes nothing.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/config"
	"github.com/oleary-labs/signet-platform/backend/internal/db"
	"github.com/oleary-labs/signet-platform/backend/internal/store"
)

// devOperators is a plausible founding operator set for local development.
// The addresses are deterministic placeholders, not real registrations — they
// exist so the marketplace, selection wizard, and group screens have data to
// exercise before a devnet is running.
var devOperators = []store.UpsertNodeOperatorInput{
	{
		Address:      "0x1000000000000000000000000000000000000001",
		Name:         "Signet Foundation",
		Description:  "The bootstrap operator run by O'Leary Labs. Serves the console's own signing group and is the reference deployment for the node software.",
		WebsiteURL:   "https://olearylabs.com",
		APIURL:       "http://127.0.0.1:8080",
		Region:       "us-east",
		Jurisdiction: "United States",
		Category:     "signet",
	},
	{
		Address:      "0x1000000000000000000000000000000000000002",
		Name:         "Meridian Infrastructure",
		Description:  "Independent infrastructure operator running validators and threshold signers across three continents. SOC 2 Type II.",
		WebsiteURL:   "https://example.com/meridian",
		APIURL:       "http://127.0.0.1:8081",
		Region:       "eu-west",
		Jurisdiction: "Ireland",
		Category:     "infrastructure",
	},
	{
		Address:      "0x1000000000000000000000000000000000000003",
		Name:         "Kestrel Custody",
		Description:  "Regulated custodian offering threshold signing as a distinct failure domain from its custody business.",
		WebsiteURL:   "https://example.com/kestrel",
		APIURL:       "http://127.0.0.1:8082",
		Region:       "us-west",
		Jurisdiction: "United States",
		Category:     "custodian",
	},
	{
		Address:      "0x1000000000000000000000000000000000000004",
		Name:         "Hokusai Systems",
		Description:  "APAC operator focused on low-latency signing for consumer applications.",
		WebsiteURL:   "https://example.com/hokusai",
		APIURL:       "http://127.0.0.1:8083",
		Region:       "ap-northeast",
		Jurisdiction: "Japan",
		Category:     "infrastructure",
	},
	{
		Address:      "0x1000000000000000000000000000000000000005",
		Name:         "Ardent Security",
		Description:  "Enterprise operator for regulated fintech, offering contractual SLAs and a named security contact.",
		WebsiteURL:   "https://example.com/ardent",
		APIURL:       "http://127.0.0.1:8084",
		Region:       "eu-central",
		Jurisdiction: "Germany",
		Category:     "enterprise",
	},
	{
		Address:      "0x1000000000000000000000000000000000000006",
		Name:         "Pale Blue Dot",
		Description:  "Community-run operator staffed by contributors to the protocol. Independent of any commercial entity.",
		WebsiteURL:   "https://example.com/pbd",
		APIURL:       "http://127.0.0.1:8085",
		Region:       "us-central",
		Jurisdiction: "United States",
		Category:     "independent",
	},
}

func main() {
	demoSubject := flag.String("subject", "", "platform subject to make the owner of a demo organization (e.g. eth:0x…)")
	nodeAddrs := flag.String("nodes", "", "comma-separated node addresses to give the demo operator identities to, instead of placeholders")
	nodeAPIs := flag.String("node-apis", "", "comma-separated API URLs, positionally matched to -nodes")
	groupAddr := flag.String("group", "", "signing group to attach to the demo app (dev fixture — skips the manager check the API enforces)")
	appEnv := flag.String("environment", "development", "environment for the demo app")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	s := store.New(pool)

	// When boot.sh has deployed contracts it passes the addresses that actually
	// registered on-chain. Giving those the demo identities means the
	// marketplace shows the operators of the local group under real names,
	// rather than six placeholders next to three "Node 0x3c44…" rows nobody
	// can tell apart.
	operators := devOperators
	if *nodeAddrs != "" {
		addrs := splitList(*nodeAddrs)
		apis := splitList(*nodeAPIs)
		operators = make([]store.UpsertNodeOperatorInput, 0, len(addrs))
		for i, addr := range addrs {
			op := devOperators[i%len(devOperators)]
			op.Address = addr
			if i < len(apis) {
				op.APIURL = apis[i]
			}
			operators = append(operators, op)
		}
	}

	verified := true
	for _, op := range operators {
		op.Verified = &verified
		if _, err := s.UpsertNodeOperator(ctx, op); err != nil {
			log.Fatalf("seed operator %s: %v", op.Name, err)
		}
		// Placeholder operators get one synthetic sample so the marketplace
		// renders its liveness state instead of "never probed" on a fresh
		// database. Real nodes get nothing: the health prober is about to
		// measure them, and a fabricated 90ms would be a lie that the console
		// presents as a measurement.
		if *nodeAddrs == "" {
			if err := s.RecordHealthSample(ctx, op.Address, true, 90, 5, ""); err != nil {
				log.Fatalf("seed health for %s: %v", op.Name, err)
			}
		}
		fmt.Printf("operator: %s (%s)\n", op.Name, op.Address)
	}

	if *demoSubject == "" {
		fmt.Println("\nDone. Pass -subject to also create a demo organization and app,")
		fmt.Println("for example: go run ./cmd/seed -subject eth:0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266")
		return
	}

	user, err := s.UpsertUser(ctx, *demoSubject, "siwe", "", "", false)
	if err != nil {
		log.Fatalf("seed user: %v", err)
	}
	org, err := s.CreateOrg(ctx, user.ID, "Demo Labs", "")
	if err != nil {
		log.Fatalf("seed org: %v", err)
	}
	app, err := s.CreateApp(ctx, store.CreateAppInput{
		OrgID:       org.ID,
		CreatedBy:   user.ID,
		Name:        "Demo Wallet",
		Description: "A sample app wired to the local devnet.",
		Environment: *appEnv,
		ChainID:     cfg.ChainID,
	})
	if err != nil {
		log.Fatalf("seed app: %v", err)
	}

	// A week of plausible metering, so the analytics screen has a shape to
	// render instead of an empty chart.
	events := []store.UsageEvent{}
	for day := 6; day >= 0; day-- {
		at := time.Now().AddDate(0, 0, -day)
		for i := 0; i < 12+day*3; i++ {
			subject := hex.EncodeToString([]byte(fmt.Sprintf("demo-user-%02d", i%9)))
			events = append(events, store.UsageEvent{
				AppID:       app.ID,
				SubjectHash: subject,
				Kind:        []string{"auth", "keygen", "sign", "sign", "sign"}[i%5],
				Curve:       "frost_secp256k1",
				LatencyMS:   80 + (i*7)%320,
				OK:          i%23 != 0,
				OccurredAt:  at,
			})
		}
	}
	if err := s.RecordUsage(ctx, events); err != nil {
		log.Fatalf("seed usage: %v", err)
	}

	// Attaching here goes straight through the store, so it bypasses the
	// on-chain manager check POST /group/attach enforces. That is acceptable
	// for a development fixture and would not be for anything else — the API
	// path is the one that matters, and it is unchanged.
	if *groupAddr != "" {
		if _, err := s.AttachGroup(ctx, app.ID, *groupAddr, "", 0, 0); err != nil {
			log.Printf("attach group: %v (the app is still usable; link it from the console)", err)
		} else {
			fmt.Printf("group attached: %s\n", *groupAddr)
		}
	}

	fmt.Printf("\norganization: %s (%s)\napp: %s (%s)\nusage events: %d\n",
		org.Name, org.ID, app.Name, app.ID, len(events))
}

// splitList parses a comma-separated flag, dropping blanks so a trailing comma
// does not produce an empty entry.
func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

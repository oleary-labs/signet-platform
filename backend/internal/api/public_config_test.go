package api

import (
	"strings"
	"testing"

	"github.com/oleary-labs/signet-platform/backend/internal/config"
)

// /v1/config is unauthenticated and the console fetches it on every page load,
// signed in or not — so everything in it reaches anyone who loads the site.
// RPC_URL is the server's own endpoint and is expected to carry a provider key;
// publishing it handed that key to every visitor. The browser gets
// PUBLIC_RPC_URL or nothing.
func TestPublicConfigDoesNotLeakTheServerRPC(t *testing.T) {
	const serverRPC = "https://eth-mainnet.example.com/v2/SECRET-KEY"

	t.Run("no public endpoint configured", func(t *testing.T) {
		s := &Server{cfg: &config.Config{RPCURL: serverRPC}}
		got := s.networkConfig()
		if got.RPCURL != "" {
			t.Errorf("published %q; want empty when PUBLIC_RPC_URL is unset", got.RPCURL)
		}
		if strings.Contains(got.RPCURL, "SECRET-KEY") {
			t.Error("the server's own RPC credential reached the public config")
		}
	})

	t.Run("public endpoint configured", func(t *testing.T) {
		s := &Server{cfg: &config.Config{
			RPCURL:       serverRPC,
			PublicRPCURL: "https://ethereum-rpc.publicnode.com",
		}}
		got := s.networkConfig()
		if got.RPCURL != "https://ethereum-rpc.publicnode.com" {
			t.Errorf("published %q; want the public endpoint", got.RPCURL)
		}
	})

	// get() treats an empty environment variable as unset, so a localhost
	// default could not be cleared in production — every visitor was told the
	// bundler lived on their own machine.
	t.Run("bundler url is not defaulted to localhost", func(t *testing.T) {
		s := &Server{cfg: &config.Config{}}
		if got := s.networkConfig().BundlerURL; got != "" {
			t.Errorf("published bundler_url %q; want empty when unset", got)
		}
	})
}

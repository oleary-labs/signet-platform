#!/usr/bin/env bash
#
# Boots the whole Signet platform stack for local development:
#   - infra:     Postgres (Docker, or a local server if Docker is not running)
#   - chain:     anvil on :8545
#   - contracts: SignetFactory + SignetGroup beacon, three nodes registered,
#                a 2-of-3 bootstrap group created
#   - nodes:     three signetd instances on :8080-:8082 (+ kms-tss unless --no-kms)
#   - bundler:   signet-min-bundler on :4337, when the repo is present
#   - backend:   platform API on :8088   - web: console on :3000
#
# The chain, contracts, and nodes come from ../signet-protocol via its own
# devnet/start.sh — this script layers the platform on top rather than
# reimplementing a devnet that would drift from the protocol's.
#
# Usage:
#   ./boot.sh                # everything: devnet + nodes + platform
#   ./boot.sh --no-kms       # nodes use in-process Go TSS (no Rust toolchain)
#   ./boot.sh --auth         # seed Google as a trusted issuer on the group
#   ./boot.sh --no-nodes     # anvil + contracts only, no signetd (fast)
#   ./boot.sh --no-chain     # no chain at all — platform against Postgres only
#   ./boot.sh --no-bundler   # skip the ERC-4337 bundler
#   ./boot.sh --no-web       # skip the Next.js console
#   ./boot.sh --no-backend   # skip the platform API
#   ./boot.sh --no-seed      # don't seed the operator directory or demo org
#   ./boot.sh --fresh        # drop and recreate the platform database first
#   ./boot.sh --no-fast-setup  # leave the devnet's 12-second blocks alone during setup
#
# ../signet-protocol is cloned automatically if it is missing. Point
# SIGNET_PROTOCOL_DIR at an existing checkout to use that instead.
#
# Ctrl-C stops everything it started, including the devnet and the Docker infra
# (named volumes persist, so your data survives).

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
LOG_DIR="$ROOT/logs"
mkdir -p "$LOG_DIR"

RUN_CHAIN=1 RUN_NODES=1 RUN_BUNDLER=1 RUN_BACKEND=1 RUN_WEB=1 RUN_SEED=1
WANT_KMS=1 USE_AUTH=0 FRESH_DB=0 FAST_SETUP=1
for arg in "$@"; do
  case "$arg" in
    --no-kms)     WANT_KMS=0 ;;
    --auth)       USE_AUTH=1 ;;
    --no-nodes)   RUN_NODES=0 ;;
    --no-chain)   RUN_CHAIN=0; RUN_NODES=0; RUN_BUNDLER=0 ;;
    --no-bundler) RUN_BUNDLER=0 ;;
    --no-backend) RUN_BACKEND=0 ;;
    --no-web)     RUN_WEB=0 ;;
    --no-seed)    RUN_SEED=0 ;;
    --fresh)      FRESH_DB=1 ;;
    --no-fast-setup) FAST_SETUP=0 ;;
    -h|--help)    sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $arg (try --help)"; exit 1 ;;
  esac
done

c_blue()  { printf "\033[1;34m%s\033[0m\n" "$1"; }
c_green() { printf "\033[1;32m%s\033[0m\n" "$1"; }
c_yellow(){ printf "\033[1;33m%s\033[0m\n" "$1"; }
c_red()   { printf "\033[1;31m%s\033[0m\n" "$1"; }

PID_FILE="$ROOT/.boot.pid"
if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE" 2>/dev/null)" 2>/dev/null; then
  printf "\033[1;31m%s\033[0m\n" "boot.sh is already running (pid $(cat "$PID_FILE"))."
  echo "Stop it with Ctrl-C, or: kill $(cat "$PID_FILE")"
  echo "Two runs would fight over the same ports and tear each other down."
  exit 1
fi
echo $$ > "$PID_FILE"

PIDS=()
CLEANED=0

# track <pid> — remember a service, and stop bash tracking it as a job. Bash
# announces every job it reaps ("Terminated: 15 …"), which is pure noise on a
# deliberate shutdown. The pid lives in PIDS either way, so cleanup is
# unaffected.
track() {
  PIDS+=("$1")
  disown "$1" 2>/dev/null || true
}
DEVNET_STARTED=0
DOCKER_DB=0

# kill_tree <SIG> <pid> — signal a process and all of its descendants, leaves
# first. This is what actually reaches the compiled binary that `go run` and
# `next dev` spawn as grandchildren; a plain kill of the tracked pid leaves
# those orphaned and still holding the port.
kill_tree() {
  local sig="$1" pid="$2" child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$sig" "$child"
  done
  kill -"$sig" "$pid" 2>/dev/null || true
}

cleanup() {
  [[ "$CLEANED" -eq 1 ]] && return   # idempotent
  CLEANED=1
  trap - INT TERM EXIT               # disarm so we never re-enter
  echo
  c_yellow "Shutting down…"

  for pid in ${PIDS[@]+"${PIDS[@]}"}; do
    kill_tree TERM "$pid"
  done
  # Give services ~3s to exit gracefully (the API does httpServer.Shutdown).
  for _ in 1 2 3; do
    local alive=0 pid
    for pid in ${PIDS[@]+"${PIDS[@]}"}; do
      kill -0 "$pid" 2>/dev/null && alive=1
    done
    [[ "$alive" -eq 0 ]] && break
    sleep 1
  done
  for pid in ${PIDS[@]+"${PIDS[@]}"}; do
    kill_tree KILL "$pid"
  done

  # The devnet owns anvil, the KMS instances, and the three nodes — let its own
  # stop script take them down so its .pids bookkeeping stays correct.
  if [[ "$DEVNET_STARTED" -eq 1 && -x "${PROTOCOL_DIR:-}/devnet/stop.sh" ]]; then
    c_yellow "  stopping the signet devnet…"
    ( "${PROTOCOL_DIR}/devnet/stop.sh" >>"$LOG_DIR/devnet.log" 2>&1 ) || true
  fi

  # Free the known ports as a backstop — never a blocking `wait`.
  # The devnet's stop.sh owns these, but it cannot run if this script was
  # killed hard — so sweep by name too rather than leaving a locked database
  # for the next boot to trip over.
  pkill -f 'kms-tss' 2>/dev/null || true

  local p pids
  for p in "${BACKEND_PORT:-8088}" "${WEB_PORT:-3000}" "${ANVIL_PORT:-8545}" 8080 8081 8082 4337; do
    pids=$(lsof -ti tcp:"$p" 2>/dev/null || true)
    [[ -n "$pids" ]] && kill -9 $pids 2>/dev/null || true
  done

  if [[ "$DOCKER_DB" -eq 1 ]] && docker info >/dev/null 2>&1; then
    c_yellow "  stopping docker postgres…"
    ( cd "$ROOT" && docker compose stop postgres >/dev/null 2>&1 ) || true
  fi

  rm -f "$PID_FILE"
  c_green "Done. (database preserved — './boot.sh --fresh' to start from empty)"
  exit 0
}
trap cleanup INT TERM EXIT

wait_for() { # wait_for <url> <name> <tries> [pid] — up = accepting connections.
  # When <pid> is given, stop early (return 2) the moment that process exits,
  # instead of polling a dead process for the full timeout.
  local url="$1" name="$2" tries="${3:-40}" pid="${4:-}"
  for ((i = 0; i < tries; i++)); do
    if curl -s -o /dev/null "$url" 2>/dev/null; then
      c_green "  $name is up ($url)"
      return 0
    fi
    if [[ -n "$pid" ]] && ! kill -0 "$pid" 2>/dev/null; then
      c_red "  $name exited before becoming ready"
      return 2
    fi
    sleep 1
  done
  c_red "  $name did not become ready at $url"
  return 1
}

free_port() { # free_port <port> <name> — force-free a port, then WAIT until it is
              # actually released, so a new bind cannot race a dying process.
  local port="$1" name="$2" pids i announced=0
  for i in $(seq 1 20); do
    pids=$(lsof -ti tcp:"$port" 2>/dev/null || true)
    [[ -z "$pids" ]] && return 0
    if [[ "$announced" -eq 0 ]]; then
      c_yellow "  port $port in use — stopping existing $name"
      announced=1
    fi
    kill -9 $pids 2>/dev/null || true
    sleep 0.25
  done
  pids=$(lsof -ti tcp:"$port" 2>/dev/null || true)
  [[ -n "$pids" ]] && c_red "  could not free port $port (still held by: $pids)"
  return 0
}

# Silence the Foundry nightly-build banner that otherwise spams every cast call.
export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

for tool in go npm curl; do
  command -v "$tool" >/dev/null || { c_red "missing required tool: $tool"; exit 1; }
done
if [[ "$RUN_CHAIN" -eq 1 ]]; then
  for tool in anvil forge cast jq git; do
    command -v "$tool" >/dev/null || {
      c_red "missing required tool: $tool (Foundry: https://getfoundry.sh)"
      c_yellow "  or run ./boot.sh --no-chain to boot the platform without a chain"
      exit 1
    }
  done
fi

# ----------------------------------------------------------------------------
# 0. Boot config (.env.boot — auto-created; every value has a working default)
# ----------------------------------------------------------------------------
if [[ ! -f .env.boot ]]; then
  cp .env.boot.example .env.boot
  c_yellow "created .env.boot from .env.boot.example (dev defaults auto-filled)"
fi
set -a; source .env.boot; set +a

# An empty value in .env.boot means "not configured", not "set to empty".
# Left exported it would shadow the same key in a component's own .env —
# godotenv does not override a variable that is already present in the
# environment, even when its value is empty, so the component would see the
# blank and fail its required-variable check.
while IFS= read -r key; do
  [[ -n "${!key:-}" ]] || unset "$key"
done < <(sed -n 's/^[[:space:]]*\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env.boot)

ANVIL_PORT="${ANVIL_PORT:-8545}"
RPC_URL="http://127.0.0.1:${ANVIL_PORT}"
# 8080-8082 belong to signetd in a devnet, so the platform API sits above them.
BACKEND_PORT="${BACKEND_PORT:-8088}"
WEB_PORT="${WEB_PORT:-3000}"
BUNDLER_PORT="${BUNDLER_PORT:-4337}"
PROTOCOL_DIR="${SIGNET_PROTOCOL_DIR:-$(cd "$ROOT/.." && pwd)/signet-protocol}"
WALLET_DIR="${SIGNET_WALLET_DIR:-$(cd "$ROOT/.." && pwd)/signet-wallet}"
BUNDLER_DIR="${SIGNET_BUNDLER_DIR:-$(cd "$ROOT/.." && pwd)/signet-min-bundler}"
# EntryPoint v0.7 is not on a fresh anvil, and the bundler's setup copies its
# bytecode from a live chain. A public endpoint keeps `./boot.sh` self-contained;
# override it if you would rather use your own.
TESTNET_RPC="${TESTNET_RPC:-https://ethereum-sepolia-rpc.publicnode.com}"

DB_URL_DEFAULT="${DATABASE_URL:-postgres://signet:signet@localhost:5433/signet_platform?sslmode=disable}"

# ----------------------------------------------------------------------------
# 1. Infra (Postgres)
# ----------------------------------------------------------------------------
c_blue "[1/7] Infra (Postgres)"
if docker info >/dev/null 2>&1; then
  DOCKER_DB=1
  ( cd "$ROOT" && docker compose up -d postgres >/dev/null 2>&1 ) \
    || { c_red "  docker compose up postgres failed"; exit 1; }
  for ((i = 0; i < 40; i++)); do
    if docker compose -f "$ROOT/docker-compose.yml" exec -T postgres \
         pg_isready -U signet -d signet_platform >/dev/null 2>&1; then
      c_green "  postgres is ready (docker, :5433)"
      break
    fi
    [[ $i -eq 39 ]] && { c_red "  postgres did not become ready"; exit 1; }
    sleep 1
  done
  if [[ "$FRESH_DB" -eq 1 ]]; then
    c_yellow "  --fresh: recreating the database…"
    # WITH (FORCE) terminates existing connections. Without it the drop fails
    # whenever anything is still connected — a previous run's API, an open psql
    # — and a swallowed failure means --fresh silently does nothing.
    if ! docker compose -f "$ROOT/docker-compose.yml" exec -T postgres \
           psql -U signet -d postgres -v ON_ERROR_STOP=1 \
             -c 'DROP DATABASE IF EXISTS signet_platform WITH (FORCE)' \
             -c 'CREATE DATABASE signet_platform' >"$LOG_DIR/freshdb.log" 2>&1; then
      c_red "  --fresh failed — see logs/freshdb.log"
      tail -n 5 "$LOG_DIR/freshdb.log"
      exit 1
    fi
    c_green "  database recreated"
  fi
elif command -v psql >/dev/null; then
  # No Docker — fall back to a local server. This is common on a dev machine
  # that already runs Postgres, and avoids demanding Docker for no reason.
  DB_URL_DEFAULT="${DATABASE_URL:-postgres://$(whoami)@localhost:5432/signet_platform?sslmode=disable}"
  if ! psql "$DB_URL_DEFAULT" -c 'SELECT 1' >/dev/null 2>&1; then
    c_yellow "  creating database signet_platform on the local server…"
    createdb signet_platform >/dev/null 2>&1 || true
  fi
  if [[ "$FRESH_DB" -eq 1 ]]; then
    c_yellow "  --fresh: recreating the database…"
    # --force terminates existing connections (Postgres 13+). Without it the
    # drop fails whenever anything is still connected, and swallowing that
    # error makes --fresh a silent no-op that leaves stale data behind.
    if ! { dropdb --force --if-exists signet_platform && createdb signet_platform; } \
           >"$LOG_DIR/freshdb.log" 2>&1; then
      c_red "  --fresh failed — see logs/freshdb.log"
      tail -n 5 "$LOG_DIR/freshdb.log"
      c_yellow "  something is still connected; stop any running API and retry"
      exit 1
    fi
    c_green "  database recreated"
  fi
  psql "$DB_URL_DEFAULT" -c 'SELECT 1' >/dev/null 2>&1 \
    && c_green "  postgres is ready (local, :5432)" \
    || { c_red "  could not reach Postgres at $DB_URL_DEFAULT"; exit 1; }
else
  c_red "  neither Docker nor psql is available — one of them is required."
  exit 1
fi

# ----------------------------------------------------------------------------
# 2. Chain, contracts, and nodes (delegated to the protocol's own devnet)
# ----------------------------------------------------------------------------
FACTORY_ADDRESS="" ACCOUNT_FACTORY_ADDRESS=""
BOOTSTRAP_GROUP="" BOOTSTRAP_NODES="" CHAIN_ID="${CHAIN_ID:-31337}"
NODE_ADDRS="" NODE_APIS="" KMS_ACTIVE="false"

if [[ "$RUN_CHAIN" -eq 0 ]]; then
  c_blue "[2/7] Chain — skipped (--no-chain)"
  c_yellow "  the console will report chain reads as unconfigured; everything else works"
else
  # The stack spans four repositories. signet-wallet must be present *before*
  # the devnet runs: its start.sh deploys the account factory and the FROST
  # validator only when it can find it, and without those there are no smart
  # wallets to send UserOperations from.
  clone_sibling() { # clone_sibling <dir> <repo> <why>
    [[ -d "$1" ]] && return 0
    c_yellow "  cloning oleary-labs/$2 → $1 ($3)"
    git clone --quiet --recurse-submodules "https://github.com/oleary-labs/$2" "$1" \
      || { c_red "  clone of $2 failed"; return 1; }
  }
  c_blue "[2/7] Sibling repositories"
  clone_sibling "$PROTOCOL_DIR" signet-protocol "chain, contracts, nodes" || exit 1
  clone_sibling "$WALLET_DIR"   signet-wallet   "smart account + FROST validator" || exit 1
  if [[ "$RUN_BUNDLER" -eq 1 ]]; then
    clone_sibling "$BUNDLER_DIR" signet-min-bundler "ERC-4337 bundler + paymaster" \
      || { c_yellow "  continuing without a bundler"; RUN_BUNDLER=0; }
  fi
  c_green "  siblings ready"

  if [[ "$RUN_NODES" -eq 1 ]]; then
    c_blue "[2/7] Devnet (anvil + contracts + 3 signetd nodes)"
    # start.sh refuses to run when a previous devnet is still recorded.
    [[ -f "$PROTOCOL_DIR/devnet/.pids" ]] && {
      c_yellow "  a devnet is already recorded — stopping it first"
      ( "$PROTOCOL_DIR/devnet/stop.sh" >>"$LOG_DIR/devnet.log" 2>&1 ) || true
    }
    free_port "$ANVIL_PORT" anvil
    for p in 8080 8081 8082; do free_port "$p" "signetd"; done
    # Node and KMS processes hold no port between them that `free_port` would
    # catch — the KMS listens on a unix socket and keeps a sled lock on its
    # database. A stranded one fails the next boot with a lock error that names
    # nothing useful, so clear them by name.
    if pgrep -f 'kms-tss|build/signetd' >/dev/null 2>&1; then
      c_yellow "  clearing stranded node/KMS processes from a previous run"
      pkill -f 'kms-tss' 2>/dev/null || true
      pkill -f 'build/signetd' 2>/dev/null || true
      sleep 1
    fi

    # The Rust KMS is the protocol's production default, and building it needs
    # a Rust toolchain plus protoc (prost-build shells out to it). Check before
    # starting rather than after a couple of minutes of compilation — and fall
    # back to the in-process Go path, since a working stack with a loud note
    # beats a failed boot.
    if [[ "$WANT_KMS" -eq 1 ]]; then
      KMS_MISSING=()
      command -v cargo  >/dev/null || KMS_MISSING+=("cargo (https://rustup.rs)")
      command -v protoc >/dev/null || KMS_MISSING+=("protoc (brew install protobuf)")
      if [[ ${#KMS_MISSING[@]} -gt 0 ]]; then
        c_yellow "  the Rust KMS needs: ${KMS_MISSING[*]}"
        c_yellow "  falling back to the in-process Go TSS (same as --no-kms)"
        c_yellow "  install the above and re-run without --no-kms to use the production KMS path"
        WANT_KMS=0
      fi
    fi

    DEVNET_ARGS=()
    [[ "$WANT_KMS" -eq 0 ]] && DEVNET_ARGS+=(--no-kms)
    [[ "$USE_AUTH" -eq 1 ]] && DEVNET_ARGS+=(--auth)
    c_yellow "  running devnet/start.sh ${DEVNET_ARGS[*]:-} (first run compiles signetd; logs: logs/devnet.log)"

    # The devnet runs anvil with 12-second blocks, and its setup sends seven
    # transactions that each wait for the next one — three funding sends, three
    # registerNode calls, and createGroup — so a minute and a half of the boot
    # is spent idle. Switching anvil to mine-on-transaction for the duration
    # removes that wait entirely; interval mining is restored afterwards so the
    # chain still ticks the way the devnet intends (removal timelocks need
    # block.timestamp to advance on its own).
    #
    # This runs in the background because devnet/start.sh owns anvil and starts
    # it itself. Losing the race only means saving less time — mining faster is
    # never unsafe.
    if [[ "$FAST_SETUP" -eq 1 ]]; then
      (
        # Wait for the contract deploys to finish before touching the mining
        # mode. `forge script --broadcast` sends a batch and polls for receipts,
        # and switching anvil out from under it is a race worth not running —
        # one boot did fail with a malformed receipt at exactly that point.
        # The seven slow transactions all come after this line prints, so
        # waiting for it costs nothing.
        #
        # This reads the devnet's own log text. If that string ever changes we
        # simply never accelerate, which is the right way for this to break.
        for _ in $(seq 1 600); do
          if grep -q "Funding and registering nodes" "$LOG_DIR/devnet.log" 2>/dev/null; then
            # Order matters: turn interval mining off *before* turning automine
            # on. With both active the interval timer and the per-transaction
            # miner each produce blocks, a transaction can land in one that is
            # then superseded, and the receipt comes back in a shape the
            # devnet's `jq` cannot parse.
            cast rpc --rpc-url "$RPC_URL" anvil_setIntervalMining 0 >/dev/null 2>&1
            cast rpc --rpc-url "$RPC_URL" evm_setAutomine true >/dev/null 2>&1
            exit 0
          fi
          sleep 0.25
        done
      ) &
      track $!
      c_yellow "  mining on-demand during node setup (--no-fast-setup to keep 12s blocks)"
    fi

    if ( cd "$PROTOCOL_DIR" && ./devnet/start.sh ${DEVNET_ARGS[@]+"${DEVNET_ARGS[@]}"} ) \
         >"$LOG_DIR/devnet.log" 2>&1; then
      DEVNET_STARTED=1
      # Hand the chain back to interval mining, matching how the devnet starts
      # anvil, so anything that depends on time advancing still works.
      if [[ "$FAST_SETUP" -eq 1 ]]; then
        cast rpc --rpc-url "$RPC_URL" anvil_setIntervalMining 12 >/dev/null 2>&1 || true
      fi
      c_green "  devnet is up (anvil, contracts, 3 nodes)"
    else
      [[ "$FAST_SETUP" -eq 1 ]] && \
        cast rpc --rpc-url "$RPC_URL" anvil_setIntervalMining 12 >/dev/null 2>&1 || true
      c_red "  devnet/start.sh failed — last log lines:"
      tail -n 25 "$LOG_DIR/devnet.log"
      if grep -q "Could not find \`protoc\`" "$LOG_DIR/devnet.log" 2>/dev/null; then
        c_yellow "  cause: the Rust KMS build needs protoc — 'brew install protobuf', or run ./boot.sh --no-kms"
      elif grep -qi "cargo: command not found\|no such file or directory: cargo" "$LOG_DIR/devnet.log" 2>/dev/null; then
        c_yellow "  cause: the Rust KMS build needs a Rust toolchain — https://rustup.rs, or run ./boot.sh --no-kms"
      else
        c_yellow "  tip: ./boot.sh --no-kms avoids the Rust KMS build entirely"
      fi
      exit 1
    fi

    # shellcheck disable=SC1091
    source "$PROTOCOL_DIR/devnet/.env"
    # devnet/.env reports which signing backend the nodes actually came up on
    # ("true"/"false"). That is the truthful value to display — it reflects what
    # is running, not what we asked for.
    KMS_ACTIVE="${USE_KMS:-false}"
    ACCOUNT_FACTORY_ADDRESS="${ACCOUNT_FACTORY:-}"
    BOOTSTRAP_GROUP="$GROUP_ADDRESS"
    BOOTSTRAP_NODES="${NODE1_API},${NODE2_API},${NODE3_API}"
    NODE_ADDRS="${NODE1_ETH},${NODE2_ETH},${NODE3_ETH}"
    NODE_APIS="${NODE1_API},${NODE2_API},${NODE3_API}"
    CHAIN_ID="$(cast chain-id --rpc-url "$RPC_URL" 2>/dev/null || echo 31337)"
    c_green "  factory $FACTORY_ADDRESS"
    c_green "  group   $BOOTSTRAP_GROUP  (2-of-3)"
  else
    # Contracts-only: enough for the console's marketplace, group screens, and
    # chain indexer, without compiling signetd or the Rust KMS.
    c_blue "[2/7] Chain (anvil + contracts only, no nodes)"
    free_port "$ANVIL_PORT" anvil
    anvil --port "$ANVIL_PORT" --silent >"$LOG_DIR/anvil.log" 2>&1 &
    track $!
    for ((i = 0; i < 60; i++)); do
      cast block-number --rpc-url "$RPC_URL" >/dev/null 2>&1 && break
      [[ $i -eq 59 ]] && { c_red "  anvil did not come up"; tail -n 20 "$LOG_DIR/anvil.log"; exit 1; }
      sleep 0.5
    done
    CHAIN_ID="$(cast chain-id --rpc-url "$RPC_URL")"
    c_green "  anvil is up (chain id $CHAIN_ID)"

    DEPLOYER_PK="${DEPLOYER_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"
    DEPLOYER_ADDR="$(cast wallet address --private-key "$DEPLOYER_PK")"

    c_yellow "  deploying SignetFactory…"
    if ! ( cd "$PROTOCOL_DIR/contracts" && ADMIN_ADDRESS="$DEPLOYER_ADDR" \
             forge script script/DeployFactory.s.sol \
               --rpc-url "$RPC_URL" --private-key "$DEPLOYER_PK" --broadcast \
         ) >"$LOG_DIR/deploy.log" 2>&1; then
      c_red "  deploy failed — last log lines:"; tail -n 20 "$LOG_DIR/deploy.log"; exit 1
    fi
    FACTORY_ADDRESS="$(grep -o 'DEPLOY:factory=0x[0-9a-fA-F]*' "$LOG_DIR/deploy.log" | tail -1 | cut -d= -f2)"
    [[ -z "$FACTORY_ADDRESS" ]] && { c_red "  could not parse the factory address"; exit 1; }
    c_green "  factory $FACTORY_ADDRESS"

    # Three anvil accounts register themselves. registerNode must come from the
    # node's own address, so each one signs its own registration.
    NODE_KEYS="${NODE_PRIVATE_KEYS:-0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d,0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a,0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6}"
    IFS=',' read -r -a NKEYS <<< "$NODE_KEYS"
    ADDRS=()
    for k in "${NKEYS[@]}"; do
      a="$(cast wallet address --private-key "$k")"
      pub="0x04$(cast wallet public-key --private-key "$k" | sed 's/^0x//')"
      cast send --rpc-url "$RPC_URL" --private-key "$k" "$FACTORY_ADDRESS" \
        "registerNode(bytes,bool,address)" "$pub" true \
        "0x0000000000000000000000000000000000000000" >/dev/null 2>&1 \
        || { c_red "  registerNode failed for $a"; exit 1; }
      ADDRS+=("$a")
    done
    c_green "  registered ${#ADDRS[@]} nodes"

    if [[ "$USE_AUTH" -eq 1 ]]; then
      ISSUERS='[("https://accounts.google.com",["'"${GOOGLE_CLIENT_ID:-demo-client.apps.googleusercontent.com}"'"])]'
    else
      ISSUERS='[]'
    fi
    GROUP_TOPIC="$(cast keccak 'GroupCreated(address,address,uint256)')"
    RECEIPT="$(cast send --rpc-url "$RPC_URL" --private-key "$DEPLOYER_PK" "$FACTORY_ADDRESS" \
      "createGroup(address[],uint256,uint256,(string,string[])[],bytes[])" \
      "[${ADDRS[0]},${ADDRS[1]},${ADDRS[2]}]" 2 86400 "$ISSUERS" '[]' --json 2>>"$LOG_DIR/deploy.log")"
    RAW="$(echo "$RECEIPT" | jq -r --arg t "$GROUP_TOPIC" \
      '.logs[] | select(.topics[0] == $t) | .topics[1]' | head -1)"
    BOOTSTRAP_GROUP="0x${RAW: -40}"
    [[ -z "$RAW" ]] && { c_red "  could not parse the group address"; exit 1; }
    c_green "  group   $BOOTSTRAP_GROUP  (2-of-3)"

    NODE_ADDRS="$(IFS=,; echo "${ADDRS[*]}")"
    NODE_APIS="http://localhost:8080,http://localhost:8081,http://localhost:8082"
    c_yellow "  no signetd running — the console's Signet sign-in route needs nodes; use SIWE"
  fi
fi

# ----------------------------------------------------------------------------
# 2b. Platform authorization key
#
#     Signing in does not by itself give a developer a Signet key. The console
#     asks the nodes for one on their behalf, presenting a certificate signed by
#     this key — which is how a SIWE user ends up holding a Signet key after a
#     single MetaMask prompt, and how an OAuth user gets one without a wallet at
#     all.
#
#     The nodes reject a certificate from a key the group does not trust
#     ("untrusted authorization key"), so registering it here is what makes the
#     whole signup path work. Without this step the console can authenticate
#     people and then fail to give them a key, which is the confusing half-state
#     worth spending twenty lines to avoid.
# ----------------------------------------------------------------------------
PLATFORM_AUTH_KEY_PUB=""
PLATFORM_AUTH_KEY="${PLATFORM_AUTH_KEY:-}"

if [[ "$RUN_CHAIN" -eq 1 && -n "$BOOTSTRAP_GROUP" ]]; then
  c_blue "[2b] Platform authorization key"

  # Deterministic by default: a fixed devnet key means restarts keep the same
  # trusted key, so a database of already-issued Signet keys stays valid.
  [[ -z "$PLATFORM_AUTH_KEY" ]] && \
    PLATFORM_AUTH_KEY="0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

  # The node expects 34 bytes: a scheme prefix (0x00 = ECDSA) followed by the
  # 33-byte compressed secp256k1 key. cast gives the uncompressed x||y form, so
  # compress it here — the prefix is 0x02 or 0x03 by the parity of y.
  AUTH_UNCOMPRESSED="$(cast wallet public-key --private-key "$PLATFORM_AUTH_KEY" 2>/dev/null | sed 's/^0x//')"
  if [[ ${#AUTH_UNCOMPRESSED} -ne 128 ]]; then
    c_red "  could not derive the platform auth key's public half"
    exit 1
  fi
  AUTH_X="${AUTH_UNCOMPRESSED:0:64}"
  AUTH_Y="${AUTH_UNCOMPRESSED:64:64}"
  if (( 0x${AUTH_Y: -1} % 2 == 0 )); then AUTH_PREFIX="02"; else AUTH_PREFIX="03"; fi
  PLATFORM_AUTH_KEY_PUB="0x00${AUTH_PREFIX}${AUTH_X}"

  AUTH_KEY_HASH="$(cast keccak "$PLATFORM_AUTH_KEY_PUB")"
  MANAGER_PK="${DEPLOYER_PRIVATE_KEY:-0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80}"

  if [[ "$(cast call --rpc-url "$RPC_URL" "$BOOTSTRAP_GROUP" \
             'isAuthKeyTrusted(bytes32)(bool)' "$AUTH_KEY_HASH" 2>/dev/null)" == "true" ]]; then
    c_green "  already trusted  ${PLATFORM_AUTH_KEY_PUB:0:18}…"
  elif cast send --rpc-url "$RPC_URL" --private-key "$MANAGER_PK" "$BOOTSTRAP_GROUP" \
         'addAuthKey(bytes)' "$PLATFORM_AUTH_KEY_PUB" >>"$LOG_DIR/deploy.log" 2>&1; then
    c_green "  registered  ${PLATFORM_AUTH_KEY_PUB:0:18}…  on $BOOTSTRAP_GROUP"
  else
    # Not fatal: the console still runs, it just cannot issue Signet keys. Say
    # so plainly rather than letting it surface later as a signup failure.
    c_yellow "  addAuthKey failed — see $LOG_DIR/deploy.log"
    c_yellow "  the console will authenticate users but cannot issue them Signet keys"
    PLATFORM_AUTH_KEY_PUB=""
  fi
fi

# ----------------------------------------------------------------------------
# 3. Bundler + paymaster
#
#    Every on-chain action the console takes is a UserOperation from the user's
#    smart wallet, so the bundler is not optional infrastructure here — without
#    it the console cannot create a group or configure auth at all.
#
#    The bundler's own devnet-setup deploys EntryPoint v0.7 (copied from a live
#    chain, since anvil has none), funds the bundler key, and deploys
#    SignetPaymaster wired to the protocol's factory. That paymaster's
#    shouldSponsor() only sponsors calls whose target is a group the factory
#    deployed — an on-chain guard underneath the platform's own.
# ----------------------------------------------------------------------------
PAYMASTER_ADDRESS="" ENTRYPOINT_ADDRESS="${ENTRYPOINT_ADDRESS:-0x0000000071727De22E5E9d8BAf0edAc6f37da032}"

if [[ "$RUN_BUNDLER" -eq 1 && "$RUN_CHAIN" -eq 1 && -d "$BUNDLER_DIR" ]]; then
  c_blue "[3/7] Bundler + paymaster (:${BUNDLER_PORT})"
  free_port "$BUNDLER_PORT" bundler

  if ( cd "$BUNDLER_DIR" && \
         TESTNET_RPC="$TESTNET_RPC" \
         ANVIL_RPC="$RPC_URL" \
         SIGNET_PROTOCOL_ENV="$PROTOCOL_DIR/devnet/.env" \
         ./scripts/devnet-setup.sh ) >"$LOG_DIR/bundler-setup.log" 2>&1; then
    PAYMASTER_ADDRESS="$(cat "$BUNDLER_DIR/.devnet/paymaster.addr" 2>/dev/null || true)"
    c_green "  entrypoint  ${ENTRYPOINT_ADDRESS}"
    [[ -n "$PAYMASTER_ADDRESS" ]] && c_green "  paymaster   ${PAYMASTER_ADDRESS}"
  else
    c_red "  bundler setup failed — last log lines:"
    tail -n 15 "$LOG_DIR/bundler-setup.log"
    if grep -q "EntryPoint" "$LOG_DIR/bundler-setup.log" 2>/dev/null; then
      c_yellow "  cause: EntryPoint v0.7 could not be copied. Set TESTNET_RPC to an RPC you trust."
    fi
    c_yellow "  continuing without a bundler — the console cannot send on-chain actions"
    RUN_BUNDLER=0
  fi
fi

if [[ "$RUN_BUNDLER" -eq 1 ]]; then
  BUNDLER_CONFIG="$BUNDLER_DIR/.devnet/bundler.toml"
  if [[ -f "$BUNDLER_CONFIG" ]]; then
    if ( cd "$BUNDLER_DIR" && go build -o ./bundler ./cmd/bundler ) >"$LOG_DIR/bundler-build.log" 2>&1; then
      ( cd "$BUNDLER_DIR" && BUNDLER_KEYSTORE_PASSWORD="devnet-insecure" \
          ./bundler --config .devnet/bundler.toml >"$LOG_DIR/bundler.log" 2>&1 ) &
      track $!
      c_yellow "  bundler logs: logs/bundler.log"
      wait_for "http://127.0.0.1:${BUNDLER_PORT}" "bundler" 40 \
        || c_yellow "  bundler did not answer — see logs/bundler.log"
    else
      c_yellow "  bundler build failed (logs/bundler-build.log)"
      RUN_BUNDLER=0
    fi
  else
    c_yellow "  no bundler config at $BUNDLER_CONFIG"
    RUN_BUNDLER=0
  fi
elif [[ "$RUN_CHAIN" -eq 1 ]]; then
  c_blue "[3/7] Bundler — skipped"
  c_yellow "  the console cannot create groups or configure auth without one"
else
  c_blue "[3/7] Bundler — skipped (no chain)"
fi

# ----------------------------------------------------------------------------
# 4. Runtime environment
#
#    force <arr> KEY VAL        — boot-owned wiring. Contract addresses and
#                                 inter-service URLs change every run, so a
#                                 stale value in a .env must never win.
#    add   <arr> file KEY VAL   — user config. Injected only where the file
#                                 leaves KEY unset, so a hand-set secret wins.
# ----------------------------------------------------------------------------
c_blue "[4/7] Runtime environment (forcing boot wiring; preserving your config)"

key_in_file() { # true when <file> defines <KEY>
  [[ -f "$1" ]] || return 1
  grep -qE "^[[:space:]]*(export[[:space:]]+)?$2=" "$1"
}
force() { local arr="$1" key="$2" val="$3"; eval "$arr+=(\"\$key=\$val\")"; }
add() {
  local arr="$1" file="$2" key="$3" val="$4"
  key_in_file "$ROOT/$file" "$key" && return 0
  eval "$arr+=(\"\$key=\$val\")"
}
add_opt() { [[ -n "$4" ]] && add "$1" "$2" "$3" "$4" || true; }

BACKEND_ENV=()
force   BACKEND_ENV PORT "$BACKEND_PORT"
force   BACKEND_ENV CORS_ORIGINS "http://localhost:${WEB_PORT}"
force   BACKEND_ENV PUBLIC_WEB_URL "http://localhost:${WEB_PORT}"
force   BACKEND_ENV SIWE_DOMAIN "localhost:${WEB_PORT}"
force   BACKEND_ENV CHAIN_ID "$CHAIN_ID"
force   BACKEND_ENV RPC_URL "$RPC_URL"
force   BACKEND_ENV PUBLIC_RPC_URL "$RPC_URL"   # anvil: nothing to keep back
force   BACKEND_ENV FACTORY_ADDRESS "$FACTORY_ADDRESS"                 # fresh each deploy
force   BACKEND_ENV ACCOUNT_FACTORY_ADDRESS "$ACCOUNT_FACTORY_ADDRESS"
force   BACKEND_ENV ENTRYPOINT_ADDRESS "$ENTRYPOINT_ADDRESS"
force   BACKEND_ENV PAYMASTER_ADDRESS "$PAYMASTER_ADDRESS"
force   BACKEND_ENV BOOTSTRAP_GROUP "$BOOTSTRAP_GROUP"
force   BACKEND_ENV BOOTSTRAP_NODES "$BOOTSTRAP_NODES"
force   BACKEND_ENV BUNDLER_URL "http://127.0.0.1:${BUNDLER_PORT}"
# Group-creation gas is sponsored through the paymaster the bundler serves.
# With no bundler running there is nothing to ask, so the platform advertises
# sponsorship as off rather than having the console attach paymaster data that
# nothing can sign.
if [[ "$RUN_BUNDLER" -eq 1 ]]; then
  force BACKEND_ENV PAYMASTER_URL "http://127.0.0.1:${BUNDLER_PORT}"
else
  force BACKEND_ENV PAYMASTER_URL ""
fi
add     BACKEND_ENV backend/.env SPONSOR_GROUP_CREATION true
# The key certificates are signed with. It has to be the one section 2b
# registered on the group, or the nodes reject every certificate it issues —
# so this is forced, not merely defaulted.
force   BACKEND_ENV PLATFORM_AUTH_KEY "${PLATFORM_AUTH_KEY_PUB:+$PLATFORM_AUTH_KEY}"
# Sent as X-API-Key on every submission. signet-min-bundler does not check it
# on eth_sendUserOperation yet (see FEATURE_EXPANSION.md); wiring it now means
# nothing has to change here when it does.
add     BACKEND_ENV backend/.env BUNDLER_API_KEY "${BUNDLER_API_KEY:-dev-bundler-key}"
add     BACKEND_ENV backend/.env DATABASE_URL "$DB_URL_DEFAULT"        # your value wins
add     BACKEND_ENV backend/.env ENV development
add     BACKEND_ENV backend/.env IN_PRODUCTION false
add     BACKEND_ENV backend/.env LOCAL_UPLOAD_DIR uploads
add     BACKEND_ENV backend/.env SESSION_SECRET "${SESSION_SECRET:-dev-only-session-secret-not-for-production-use}"
add     BACKEND_ENV backend/.env INGEST_KEY "${INGEST_KEY:-dev-ingest-key}"
add     BACKEND_ENV backend/.env HEALTH_PROBE_SECONDS 120
add     BACKEND_ENV backend/.env CHAIN_SYNC_SECONDS 20
add_opt BACKEND_ENV backend/.env STAFF_SUBJECTS "${STAFF_SUBJECTS:-}"

WEB_ENV=()
force   WEB_ENV NEXT_PUBLIC_API_URL "http://localhost:${BACKEND_PORT}"
force   WEB_ENV NEXT_PUBLIC_SITE_URL "http://localhost:${WEB_PORT}"
force   WEB_ENV NEXT_PUBLIC_BUNDLER_URL "http://127.0.0.1:${BUNDLER_PORT}"
force   WEB_ENV NEXT_PUBLIC_ENTRYPOINT_ADDRESS "$ENTRYPOINT_ADDRESS"
force   WEB_ENV NEXT_PUBLIC_ACCOUNT_FACTORY_ADDRESS "$ACCOUNT_FACTORY_ADDRESS"
force   WEB_ENV NEXT_PUBLIC_FACTORY_ADDRESS "$FACTORY_ADDRESS"
force   WEB_ENV NEXT_PUBLIC_CHAIN_ID "$CHAIN_ID"
force   WEB_ENV NEXT_PUBLIC_RPC_URL "$RPC_URL"
# The console builds and signs UserOperations, so it needs the paymaster on to
# get them sponsored — the whole point of the smart-wallet path is that a
# developer never funds a wallet to try the product.
if [[ "$RUN_BUNDLER" -eq 1 ]]; then
  force WEB_ENV NEXT_PUBLIC_USE_PAYMASTER true
else
  force WEB_ENV NEXT_PUBLIC_USE_PAYMASTER false
fi
add_opt WEB_ENV web/.env.local NEXT_PUBLIC_GOOGLE_CLIENT_ID "${GOOGLE_CLIENT_ID:-}"
add_opt WEB_ENV web/.env.local GOOGLE_CLIENT_SECRET "${GOOGLE_CLIENT_SECRET:-}"
add_opt WEB_ENV web/.env.local PROVER_API_KEY "${PROVER_API_KEY:-}"

c_green "  wiring ready; injecting only keys each env file leaves unset (files untouched)"
[[ "$RUN_CHAIN" -eq 1 && "$RUN_NODES" -eq 1 && -z "${GOOGLE_CLIENT_ID:-}" ]] && \
  c_yellow "  note: GOOGLE_CLIENT_ID unset in .env.boot — Signet sign-in is unavailable, SIWE still works"

# ----------------------------------------------------------------------------
# 5. Platform API
# ----------------------------------------------------------------------------
c_blue "[5/7] Platform API (:${BACKEND_PORT})"
if [[ "$RUN_BACKEND" -eq 1 ]]; then
  free_port "$BACKEND_PORT" backend
  ( cd "$ROOT/backend" && env ${BACKEND_ENV[@]+"${BACKEND_ENV[@]}"} go run ./cmd/server \
      >"$LOG_DIR/backend.log" 2>&1 ) &
  BACKEND_PID=$!
  track "$BACKEND_PID"
  c_yellow "  backend logs: logs/backend.log"
  wait_for "http://localhost:${BACKEND_PORT}/health" "backend" 90 "$BACKEND_PID" \
    || { c_red "  backend failed — last log lines:"; tail -n 20 "$LOG_DIR/backend.log"; exit 1; }
else
  c_yellow "  skipped (--no-backend)"
fi

# ----------------------------------------------------------------------------
# 6. Seed (operator directory + a demo organization)
# ----------------------------------------------------------------------------
c_blue "[6/7] Seed data"
if [[ "$RUN_SEED" -eq 1 && "$RUN_BACKEND" -eq 1 ]]; then
  SEED_ARGS=()
  # When the chain is up, give the operator identities to the addresses that
  # actually registered, so the marketplace lists the real local operators
  # instead of placeholders sitting next to them.
  [[ -n "$NODE_ADDRS" ]] && SEED_ARGS+=(-nodes "$NODE_ADDRS" -node-apis "$NODE_APIS")
  # Point the demo app at the group that actually exists, so the console's
  # group and wallet screens have live data on first boot instead of an
  # empty-state walkthrough.
  #
  # Development, with the devnet's mixed operator set. That is a legitimate
  # state: the first-party restriction governs what the platform will *deploy*,
  # not who a developer may later invite — which is exactly the route to
  # production, and what this demo shows.
  if [[ -n "$BOOTSTRAP_GROUP" ]]; then
    SEED_ARGS+=(-group "$BOOTSTRAP_GROUP")
  fi
  SEED_ARGS+=(-subject "eth:${DEMO_SUBJECT_ADDRESS:-0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266}")

  if ( cd "$ROOT/backend" && env ${BACKEND_ENV[@]+"${BACKEND_ENV[@]}"} \
         go run ./cmd/seed "${SEED_ARGS[@]}" ) >"$LOG_DIR/seed.log" 2>&1; then
    c_green "  operator directory and demo organization seeded (logs/seed.log)"
  else
    # An existing demo org makes the seeder fail on a second run; the stack is
    # perfectly usable either way, so this never blocks boot.
    c_yellow "  seed skipped or partially applied — see logs/seed.log (stack still runs)"
  fi
else
  c_yellow "  skipped"
fi

# ----------------------------------------------------------------------------
# 7. Console
# ----------------------------------------------------------------------------
c_blue "[7/7] Console (:${WEB_PORT})"
if [[ "$RUN_WEB" -eq 1 ]]; then
  free_port "$WEB_PORT" web
  if [[ ! -d "$ROOT/web/node_modules" ]]; then
    c_yellow "  installing web deps…"
    ( cd "$ROOT/web" && npm install --no-audit --no-fund >"$LOG_DIR/web-install.log" 2>&1 ) \
      || { c_red "  npm install failed — see logs/web-install.log"; exit 1; }
  fi
  # Start from a clean build cache — a stale .next from a production build
  # causes phantom "__webpack_modules__ is not a function" errors in dev.
  rm -rf "$ROOT/web/.next" "$ROOT/web/node_modules/.cache"
  ( cd "$ROOT/web" && env ${WEB_ENV[@]+"${WEB_ENV[@]}"} npx next dev -p "$WEB_PORT" \
      >"$LOG_DIR/web.log" 2>&1 ) &
  WEB_PID=$!
  track "$WEB_PID"
  c_yellow "  web logs: logs/web.log"
  wait_for "http://localhost:${WEB_PORT}" "web" 90 "$WEB_PID" \
    || c_yellow "  web still starting — check logs/web.log"
else
  c_yellow "  skipped (--no-web)"
fi

# ----------------------------------------------------------------------------
echo
c_green "Signet platform is up:"
[[ "$RUN_WEB" -eq 1 ]]     && echo "  Console    http://localhost:${WEB_PORT}"
[[ "$RUN_BACKEND" -eq 1 ]] && echo "  API        http://localhost:${BACKEND_PORT}"
if [[ "$RUN_CHAIN" -eq 1 ]]; then
echo "  Chain      ${RPC_URL} (chain id ${CHAIN_ID})"
echo "  Factory    ${FACTORY_ADDRESS}"
echo "  Group      ${BOOTSTRAP_GROUP} (2-of-3)"
if [[ "$RUN_NODES" -eq 1 ]]; then
echo "  Nodes      ${BOOTSTRAP_NODES}"
if [[ "${KMS_ACTIVE:-false}" == "true" ]]; then
echo "  KMS        Rust kms-tss"
else
echo "  KMS        in-process Go TSS (install cargo + protoc for the production path)"
fi
fi
if [[ "$RUN_BUNDLER" -eq 1 ]]; then
echo "  Bundler    http://127.0.0.1:${BUNDLER_PORT}"
echo "  EntryPoint ${ENTRYPOINT_ADDRESS}"
[[ -n "$PAYMASTER_ADDRESS" ]] && echo "  Paymaster  ${PAYMASTER_ADDRESS}"
fi
[[ -n "$ACCOUNT_FACTORY_ADDRESS" ]] && echo "  AcctFactory ${ACCOUNT_FACTORY_ADDRESS}"
fi
echo
echo "  Sign in    http://localhost:${WEB_PORT}/login"
if [[ "$RUN_NODES" -eq 1 && -n "${GOOGLE_CLIENT_ID:-}" ]]; then
echo "             both routes available (Signet via the bootstrap group, or SIWE)"
else
echo "             use Sign-In with Ethereum — the Signet route needs signetd + GOOGLE_CLIENT_ID"
fi
echo "  Demo org   owned by ${DEMO_SUBJECT_ADDRESS:-0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266} (anvil account 0)"
echo

c_blue "Running. Press Ctrl-C to stop everything."
TAIL_LOGS=()
[[ "$RUN_BACKEND" -eq 1 ]] && TAIL_LOGS+=("$LOG_DIR/backend.log")
[[ "$RUN_WEB" -eq 1 ]]     && TAIL_LOGS+=("$LOG_DIR/web.log")
# `tail` runs in the background and we block on `wait`, not on tail itself.
# Bash defers a trap while a foreground child is running, so a foreground
# `tail -f` would swallow SIGTERM entirely and only Ctrl-C — which signals the
# whole process group — would ever shut the stack down.
if [[ ${#TAIL_LOGS[@]} -gt 0 ]]; then
  tail -f ${TAIL_LOGS[@]+"${TAIL_LOGS[@]}"} &
else
  sleep 2147483647 &
fi
track $!
# Block in a loop of short sleeps rather than `wait`. A foreground child defers
# the trap until it exits, so waiting on `tail -f` would swallow SIGTERM
# entirely; a one-second sleep bounds that delay to something imperceptible,
# and keeps `tail` disowned so bash does not announce reaping it.
while :; do sleep 1; done

// Package userop validates and forwards ERC-4337 UserOperations.
//
// Every on-chain action the console takes is a UserOperation from the
// developer's own smart wallet, signed by their Signet key and submitted to the
// bundler *by this backend*. That indirection exists for one reason:
// sponsorship. The paymaster pays, so the platform has to be sure it is only
// paying for the action a developer actually asked the console to perform.
//
// # The guard
//
// A signed UserOperation is an opaque blob until someone decodes it. Forwarding
// one because the request that carried it looked plausible would sponsor
// whatever the caller put inside. So each route declares an Intent — the
// contract it expects to be called and the method — and this package decodes
// the operation's callData and refuses anything that does not match:
//
//   - `sender` must be the caller's own smart wallet
//   - the wrapped call must be `execute(dest, value, func)`
//   - `dest` must be the exact contract the route named
//   - the selector must be one the route allows
//   - `value` must be zero; the platform sponsors configuration, never transfers
//
// # What this does not protect
//
// The decode above governs what the *platform* will submit. It is not a guard
// on the bundler itself: signet-min-bundler today enforces its X-API-Key on the
// prover endpoint only, not on eth_sendUserOperation or pm_getPaymasterData, so
// anyone who can reach the bundler can ask for sponsorship directly. What still
// holds them back is the paymaster's own check — SignetPaymaster only signs for
// calls whose target is the factory, a factory-deployed group, or the sender
// itself — and the bundler's optional sponsor whitelist. Neither is a
// substitute for the API key: see FEATURE_EXPANSION.md, "Bundler submission
// authentication". This client sends the header already, so it will be enforced
// the moment the bundler checks it.
package userop

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Packed is the wire form of an ERC-4337 v0.7 UserOperation, as the console
// builds it and the bundler expects it.
type Packed struct {
	Sender                        string `json:"sender"`
	Nonce                         string `json:"nonce"`
	Factory                       string `json:"factory,omitempty"`
	FactoryData                   string `json:"factoryData,omitempty"`
	CallData                      string `json:"callData"`
	CallGasLimit                  string `json:"callGasLimit"`
	VerificationGasLimit          string `json:"verificationGasLimit"`
	PreVerificationGas            string `json:"preVerificationGas"`
	MaxFeePerGas                  string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas          string `json:"maxPriorityFeePerGas"`
	Paymaster                     string `json:"paymaster,omitempty"`
	PaymasterVerificationGasLimit string `json:"paymasterVerificationGasLimit,omitempty"`
	PaymasterPostOpGasLimit       string `json:"paymasterPostOpGasLimit,omitempty"`
	PaymasterData                 string `json:"paymasterData,omitempty"`
	Signature                     string `json:"signature"`
}

// Intent is what a route expects the operation to do.
type Intent struct {
	// Action names the route, for error messages and the audit trail.
	Action string
	// Dest is the only contract the operation may call.
	Dest string
	// Selectors are the 4-byte method selectors the route allows, keyed by
	// method name so a refusal can say what was expected.
	Selectors map[string]string
	// RefusePaymaster rejects an operation that asks the platform to pay.
	//
	// Set by routes that will forward an operation but will not sponsor it —
	// creating a production group, for instance. Without it, a developer could
	// get the same sponsorship for production that the platform offers only for
	// development, simply by attaching paymaster data the platform never
	// inspected.
	RefusePaymaster bool
}

// executeSelector is `execute(address,uint256,bytes)` on the smart account —
// the only entry point the console ever wraps a call in.
const executeSelector = "b61d27f6"

// Decoded is what the guard understood the operation to be doing.
type Decoded struct {
	Sender   string
	Dest     string
	Value    *big.Int
	Selector string
	Method   string
	Inner    []byte
}

// Validate decodes an operation and checks it against an intent.
//
// It returns an error naming the mismatch rather than a generic refusal: a
// developer whose console sent the wrong thing needs to know which part was
// wrong, and an operator reading the logs needs the same.
func Validate(op *Packed, intent Intent, expectedSender string) (*Decoded, error) {
	if op == nil {
		return nil, fmt.Errorf("no user operation was supplied")
	}
	if !strings.EqualFold(strings.TrimSpace(op.Sender), strings.TrimSpace(expectedSender)) {
		return nil, fmt.Errorf(
			"the operation is from %s, but your smart wallet is %s — the platform only sponsors your own account",
			op.Sender, expectedSender)
	}
	if strings.TrimSpace(op.Signature) == "" || op.Signature == "0x" {
		return nil, fmt.Errorf("the operation is unsigned")
	}

	call, err := hexBytes(op.CallData)
	if err != nil {
		return nil, fmt.Errorf("callData is not hex: %w", err)
	}
	if len(call) < 4 {
		return nil, fmt.Errorf("callData is too short to contain a selector")
	}
	if hex.EncodeToString(call[:4]) != executeSelector {
		return nil, fmt.Errorf(
			"the operation does not wrap execute(address,uint256,bytes); the platform only sponsors that form")
	}

	// execute(address dest, uint256 value, bytes func): three head words, with
	// the third an offset to the inner calldata.
	args := call[4:]
	if len(args) < 96 {
		return nil, fmt.Errorf("execute() arguments are truncated")
	}
	dest := "0x" + hex.EncodeToString(args[12:32])
	value := new(big.Int).SetBytes(args[32:64])

	offset := new(big.Int).SetBytes(args[64:96])
	if !offset.IsInt64() || offset.Int64() < 0 || offset.Int64()+32 > int64(len(args)) {
		return nil, fmt.Errorf("the inner call offset is outside the calldata")
	}
	at := offset.Int64()
	length := new(big.Int).SetBytes(args[at : at+32])
	if !length.IsInt64() || at+32+length.Int64() > int64(len(args)) {
		return nil, fmt.Errorf("the inner call length is outside the calldata")
	}
	inner := args[at+32 : at+32+length.Int64()]

	if intent.RefusePaymaster && hasPaymaster(op) {
		return nil, fmt.Errorf(
			"%s is not sponsored, but the operation carries paymaster data — rebuild it without a paymaster",
			intent.Action)
	}
	return checkCall(op.Sender, dest, value, inner, intent)
}

// ValidateTransaction checks a plain transaction sent from the caller's own
// wallet against the same intent a user operation would face.
//
// The difference from Validate is only in shape. A user operation wraps the
// call in execute(address,uint256,bytes) because a smart account has to be told
// what to do; a transaction from an EOA carries the destination in `to` and the
// selector in the first four bytes of its input. What the route is willing to
// authorize is identical, so the rules live in checkCall and both paths reach
// them.
//
// `from` matters as much as the call does. Nothing stops someone quoting a
// transaction hash they did not send, so a route that recorded metadata on the
// strength of a hash alone would let a stranger's transaction stand in for the
// caller's.
//
// It checks only that the sender is an account this caller controls, not that
// it is the group's manager. The contract already decided that: these calls are
// onlyManager, so a receipt saying the transaction succeeded is proof the
// sender was the manager at the time it ran. Re-deriving the rule here would
// duplicate it, and a duplicate can disagree — with the chain, or with a
// manager transfer that landed in between.
//
// RefusePaymaster is not consulted: the sender paid.
func ValidateTransaction(from, to string, input []byte, value *big.Int, intent Intent, allowedSenders []string) (*Decoded, error) {
	if !anyAddressMatches(from, allowedSenders) {
		return nil, fmt.Errorf(
			"the transaction was sent by %s, which is not an account you control — quote a transaction you sent yourself",
			from)
	}
	if value == nil {
		value = new(big.Int)
	}
	return checkCall(from, to, value, input, intent)
}

// checkCall holds every rule that is about the call itself rather than how it
// reached the chain, so a user operation and a transaction cannot drift into
// authorizing different things.
func checkCall(sender, dest string, value *big.Int, inner []byte, intent Intent) (*Decoded, error) {
	if value.Sign() != 0 {
		return nil, fmt.Errorf(
			"the call moves %s wei; the platform records configuration calls, never transfers", value)
	}
	if !strings.EqualFold(dest, strings.TrimSpace(intent.Dest)) {
		return nil, fmt.Errorf(
			"the call is to %s, but %s acts on %s", dest, intent.Action, intent.Dest)
	}
	if len(inner) < 4 {
		return nil, fmt.Errorf("the call has no selector")
	}
	selector := hex.EncodeToString(inner[:4])

	method := ""
	for name, want := range intent.Selectors {
		if strings.EqualFold(selector, strings.TrimPrefix(want, "0x")) {
			method = name
			break
		}
	}
	if method == "" {
		allowed := make([]string, 0, len(intent.Selectors))
		for name := range intent.Selectors {
			allowed = append(allowed, name)
		}
		// Map order is random; sort so the same refusal reads the same way
		// twice, in a log and in a test.
		sort.Strings(allowed)
		return nil, fmt.Errorf(
			"the call uses selector 0x%s, but %s allows only: %s",
			selector, intent.Action, strings.Join(allowed, ", "))
	}

	return &Decoded{
		Sender:   sender,
		Dest:     dest,
		Value:    value,
		Selector: selector,
		Method:   method,
		Inner:    inner,
	}, nil
}

// hasPaymaster reports whether an operation asks a paymaster to pay for it.
// Both the address and the data are checked: either alone is enough for the
// EntryPoint to route the operation to a paymaster.
func hasPaymaster(op *Packed) bool {
	return isSet(op.Paymaster) || isSet(op.PaymasterData)
}

func isSet(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "0x" && v != "0X"
}

// Word returns the i-th 32-byte argument word of the inner call, for routes
// that need to check *which* record a call acts on and not merely which method
// it is. Reports false when the call is too short to have that word.
func (d *Decoded) Word(i int) ([32]byte, bool) {
	var w [32]byte
	start := 4 + i*32
	if i < 0 || start+32 > len(d.Inner) {
		return w, false
	}
	copy(w[:], d.Inner[start:start+32])
	return w, true
}

// BytesArg returns the i-th argument of the inner call when that argument is a
// dynamic `bytes`, following the ABI offset to the data. Reports false if the
// encoding does not describe a whole, in-bounds value.
func (d *Decoded) BytesArg(i int) ([]byte, bool) {
	head, ok := d.Word(i)
	if !ok {
		return nil, false
	}
	off := new(big.Int).SetBytes(head[:])
	if !off.IsInt64() {
		return nil, false
	}
	args := d.Inner[4:]
	at := off.Int64()
	if at < 0 || at+32 > int64(len(args)) {
		return nil, false
	}
	n := new(big.Int).SetBytes(args[at : at+32])
	if !n.IsInt64() || at+32+n.Int64() > int64(len(args)) {
		return nil, false
	}
	return args[at+32 : at+32+n.Int64()], true
}

// Client submits validated operations to the bundler.
type Client struct {
	url        string
	entryPoint string
	apiKey     string
	http       *http.Client
}

// NewClient builds a bundler client. An empty URL returns nil, meaning the
// platform cannot submit — callers report that rather than crashing.
func NewClient(url, entryPoint, apiKey string) *Client {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	return &Client{
		url:        strings.TrimRight(url, "/"),
		entryPoint: entryPoint,
		apiKey:     apiKey,
		// Bundling waits for inclusion, so this is generous by design.
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Receipt is the outcome of a submitted operation.
type Receipt struct {
	UserOpHash      string `json:"userOpHash"`
	TransactionHash string `json:"transactionHash"`
	Success         bool   `json:"success"`
	// RevertReason is what the contract said when it refused. Reporting it is
	// the difference between "the transaction failed" and a developer knowing
	// they are not the group's manager.
	RevertReason string `json:"revertReason,omitempty"`
}

// Send submits an operation and waits for its receipt.
func (c *Client) Send(ctx context.Context, op *Packed) (*Receipt, error) {
	var hash string
	if err := c.call(ctx, "eth_sendUserOperation", []any{op, c.entryPoint}, &hash); err != nil {
		return nil, err
	}

	// Poll rather than assume: a bundler that accepted an operation has not
	// yet included it, and reporting success before inclusion would have the
	// console show a group that does not exist.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var rcpt *Receipt
		if err := c.call(ctx, "eth_getUserOperationReceipt", []any{hash}, &rcpt); err != nil {
			return nil, err
		}
		if rcpt != nil && rcpt.TransactionHash != "" {
			if !rcpt.Success {
				if rcpt.RevertReason != "" {
					return rcpt, fmt.Errorf("the operation reverted: %s (%s)",
						rcpt.RevertReason, rcpt.TransactionHash)
				}
				return rcpt, fmt.Errorf("the operation was included but reverted (%s)", rcpt.TransactionHash)
			}
			return rcpt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, fmt.Errorf("the bundler accepted operation %s but it was not included in time", hash)
}

// Sponsorship asks the bundler's paymaster to sponsor an operation, returning
// the fields to merge into it before signing.
func (c *Client) Sponsorship(ctx context.Context, method string, op *Packed, chainID int64, context map[string]any) (json.RawMessage, error) {
	if context == nil {
		context = map[string]any{}
	}
	var out json.RawMessage
	err := c.call(ctx, method, []any{op, c.entryPoint, hexUint(chainID), context}, &out)
	return out, err
}

func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		// The bundler accepts submissions only from a caller presenting this,
		// so reaching the endpoint is not enough to spend the paymaster.
		req.Header.Set("X-API-Key", c.apiKey)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("the bundler rejected this backend's API key")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("%s: bundler returned %d: %s", method, res.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if parsed.Error != nil {
		return fmt.Errorf("%s: %s", method, parsed.Error.Message)
	}
	if out != nil && len(parsed.Result) > 0 {
		return json.Unmarshal(parsed.Result, out)
	}
	return nil
}

func hexBytes(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(s), "0x"))
}

func hexUint(v int64) string { return fmt.Sprintf("0x%x", v) }

// anyAddressMatches reports whether addr is one of the candidates, ignoring
// case and blank entries. A user has up to two addresses — a smart wallet and
// the EOA they signed in with — and which one sends depends on how the group
// was created, not on anything the console decides.
func anyAddressMatches(addr string, candidates []string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" && strings.EqualFold(addr, c) {
			return true
		}
	}
	return false
}

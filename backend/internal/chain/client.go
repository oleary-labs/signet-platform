package chain

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/sha3"
)

// Client performs read-only `eth_call`s against the Signet contracts.
//
// The platform never writes to the chain. Every state-changing action a
// developer takes in the console is a UserOperation built, threshold-signed,
// and submitted from their own browser — the platform can display group state
// and cache it, but it cannot change it. That is a deliberate property: a
// compromised platform must not be able to alter anyone's signing group.
type Client struct {
	rpcURL  string
	factory string
	chainID int64
	http    *http.Client
	nextID  atomic.Int64
}

// New builds a chain client. A nil client (rpcURL or factory empty) is a valid
// state meaning "chain reads are not configured", and callers check Enabled
// rather than crashing a server that is running without an RPC endpoint.
func New(rpcURL, factoryAddress string, chainID int64) *Client {
	return &Client{
		rpcURL:  strings.TrimSpace(rpcURL),
		factory: strings.ToLower(strings.TrimSpace(factoryAddress)),
		chainID: chainID,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled reports whether chain reads are configured.
func (c *Client) Enabled() bool { return c != nil && c.rpcURL != "" && c.factory != "" }

// FactoryAddress is the configured SignetFactory address.
func (c *Client) FactoryAddress() string { return c.factory }

// ChainID is the configured chain.
func (c *Client) ChainID() int64 { return c.chainID }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
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

func (c *Client) call(ctx context.Context, to string, data []byte) ([]byte, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("chain reads are not configured (set RPC_URL and FACTORY_ADDRESS)")
	}
	var hexResult string
	err := c.rpc(ctx, "eth_call", []any{
		map[string]string{"to": to, "data": "0x" + encodeHex(data)},
		"latest",
	}, &hexResult)
	if err != nil {
		return nil, err
	}
	return decodeHex(hexResult)
}

// rpc performs one JSON-RPC call and decodes its result into out.
func (c *Client) rpc(ctx context.Context, method string, params []any, out any) error {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read rpc response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("rpc status %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var resp rpcResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if len(resp.Result) == 0 {
		// A node that answers with neither result nor error is answering
		// "null" — which for a receipt lookup means "not mined yet".
		resp.Result = []byte("null")
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("unexpected %s result: %w", method, err)
	}
	return nil
}

// ─────────────────────────── SignetFactory ───────────────────────────

// NodeInfo mirrors ISignetFactory.NodeInfo.
type NodeInfo struct {
	Address      string `json:"address"`
	PubKey       string `json:"pubkey"`
	IsOpen       bool   `json:"is_open"`
	Registered   bool   `json:"registered"`
	RegisteredAt int64  `json:"registered_at"`
	Operator     string `json:"operator"`
}

// RegisteredNodes returns every node address ever registered with the factory.
func (c *Client) RegisteredNodes(ctx context.Context) ([]string, error) {
	raw, err := c.call(ctx, c.factory, encodeCall("getRegisteredNodes()"))
	if err != nil {
		return nil, err
	}
	return decoder{raw}.addressArray()
}

// Groups returns every group the factory has deployed.
func (c *Client) Groups(ctx context.Context) ([]string, error) {
	raw, err := c.call(ctx, c.factory, encodeCall("getGroups()"))
	if err != nil {
		return nil, err
	}
	return decoder{raw}.addressArray()
}

// GroupsByManager returns the groups an address manages — the query behind a
// developer's app list when they connect a fresh account.
func (c *Client) GroupsByManager(ctx context.Context, manager string) ([]string, error) {
	arg, err := addressWord(manager)
	if err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, c.factory, encodeCall("getGroupsByManager(address)", arg))
	if err != nil {
		return nil, err
	}
	return decoder{raw}.addressArray()
}

// NodeGroups returns the groups a node participates in.
func (c *Client) NodeGroups(ctx context.Context, node string) ([]string, error) {
	arg, err := addressWord(node)
	if err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, c.factory, encodeCall("getNodeGroups(address)", arg))
	if err != nil {
		return nil, err
	}
	return decoder{raw}.addressArray()
}

// Node reads a node's registry entry.
func (c *Client) Node(ctx context.Context, node string) (*NodeInfo, error) {
	arg, err := addressWord(node)
	if err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, c.factory, encodeCall("getNode(address)", arg))
	if err != nil {
		return nil, err
	}
	d := decoder{raw}
	// A struct return is encoded as a tuple: because NodeInfo contains a
	// dynamic `bytes`, the whole tuple is dynamic and the first word is an
	// offset to its body.
	base, err := d.offsetAt(0)
	if err != nil {
		return nil, err
	}
	pubOff, err := d.offsetAt(base + 0*wordSize)
	if err != nil {
		return nil, err
	}
	pubkey, err := d.bytesAt(base + pubOff)
	if err != nil {
		return nil, err
	}
	isOpen, err := d.boolAt(base + 1*wordSize)
	if err != nil {
		return nil, err
	}
	registered, err := d.boolAt(base + 2*wordSize)
	if err != nil {
		return nil, err
	}
	registeredAt, err := d.intAt(base + 3*wordSize)
	if err != nil {
		return nil, err
	}
	operator, err := d.addressAt(base + 4*wordSize)
	if err != nil {
		return nil, err
	}
	addr, err := normalize(node)
	if err != nil {
		return nil, err
	}
	return &NodeInfo{
		Address:      addr,
		PubKey:       "0x" + encodeHex(pubkey),
		IsOpen:       isOpen,
		Registered:   registered,
		RegisteredAt: registeredAt,
		Operator:     operator,
	}, nil
}

// ─────────────────────────── SignetGroup ───────────────────────────

// Issuer mirrors ISignetGroup.OAuthIssuer.
type Issuer struct {
	Issuer    string   `json:"issuer"`
	ClientIDs []string `json:"client_ids"`
}

// RemovalRequest mirrors ISignetGroup.RemovalRequest.
type RemovalRequest struct {
	Node         string `json:"node"`
	ExecuteAfter int64  `json:"execute_after"`
	Initiator    string `json:"initiator"`
}

// GroupState is a full read of one signing group, assembled from the several
// view calls the contract exposes.
type GroupState struct {
	Address         string           `json:"address"`
	Manager         string           `json:"manager"`
	Threshold       int64            `json:"threshold"`
	RemovalDelay    int64            `json:"removal_delay"`
	IsOperational   bool             `json:"is_operational"`
	ActiveNodes     []string         `json:"active_nodes"`
	PendingNodes    []string         `json:"pending_nodes"`
	PendingRemovals []RemovalRequest `json:"pending_removals"`
	Issuers         []Issuer         `json:"issuers"`
	AuthKeys        []string         `json:"auth_keys"`
}

// GroupState reads everything the console shows for a group. The calls are
// sequential rather than batched because the platform talks to a plain
// JSON-RPC endpoint and cannot assume a multicall contract is deployed.
func (c *Client) GroupState(ctx context.Context, group string) (*GroupState, error) {
	addr, err := normalize(group)
	if err != nil {
		return nil, err
	}
	// Every slice starts empty rather than nil: a JSON API that returns `null`
	// where the client's type says "array" turns a harmless empty case into a
	// TypeError on the page.
	st := &GroupState{
		Address:         addr,
		ActiveNodes:     []string{},
		PendingNodes:    []string{},
		PendingRemovals: []RemovalRequest{},
		Issuers:         []Issuer{},
		AuthKeys:        []string{},
	}

	// view reads one view function and hands the return data to a decoder.
	view := func(sig string) (decoder, error) {
		raw, err := c.call(ctx, addr, encodeCall(sig))
		if err != nil {
			return decoder{}, fmt.Errorf("%s: %w", sig, err)
		}
		return decoder{raw}, nil
	}

	d, err := view("manager()")
	if err != nil {
		return nil, err
	}
	if st.Manager, err = d.addressAt(0); err != nil {
		return nil, err
	}

	if d, err = view("threshold()"); err != nil {
		return nil, err
	}
	if st.Threshold, err = d.intAt(0); err != nil {
		return nil, err
	}

	if d, err = view("removalDelay()"); err != nil {
		return nil, err
	}
	if st.RemovalDelay, err = d.intAt(0); err != nil {
		return nil, err
	}

	if d, err = view("isOperational()"); err != nil {
		return nil, err
	}
	if st.IsOperational, err = d.boolAt(0); err != nil {
		return nil, err
	}

	if d, err = view("getActiveNodes()"); err != nil {
		return nil, err
	}
	if st.ActiveNodes, err = d.addressArray(); err != nil {
		return nil, err
	}

	if d, err = view("getPendingNodes()"); err != nil {
		return nil, err
	}
	if st.PendingNodes, err = d.addressArray(); err != nil {
		return nil, err
	}

	if d, err = view("getPendingRemovals()"); err != nil {
		return nil, err
	}
	removalNodes, err := d.addressArray()
	if err != nil {
		return nil, err
	}
	for _, node := range removalNodes {
		req, err := c.RemovalRequest(ctx, addr, node)
		if err != nil {
			return nil, err
		}
		st.PendingRemovals = append(st.PendingRemovals, *req)
	}

	if st.Issuers, err = c.Issuers(ctx, addr); err != nil {
		return nil, err
	}
	if st.AuthKeys, err = c.AuthKeys(ctx, addr); err != nil {
		return nil, err
	}
	return st, nil
}

// RemovalRequest reads the timelock entry for a queued node removal.
func (c *Client) RemovalRequest(ctx context.Context, group, node string) (*RemovalRequest, error) {
	arg, err := addressWord(node)
	if err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, group, encodeCall("removalRequests(address)", arg))
	if err != nil {
		return nil, fmt.Errorf("removalRequests: %w", err)
	}
	d := decoder{raw}
	// RemovalRequest is a fully static tuple, so it is inlined in the return
	// data rather than pointed to by an offset word.
	executeAfter, err := d.intAt(0)
	if err != nil {
		return nil, err
	}
	initiator, err := d.addressAt(wordSize)
	if err != nil {
		return nil, err
	}
	return &RemovalRequest{Node: node, ExecuteAfter: executeAfter, Initiator: initiator}, nil
}

// Issuers reads the group's trusted OAuth issuers.
func (c *Client) Issuers(ctx context.Context, group string) ([]Issuer, error) {
	raw, err := c.call(ctx, group, encodeCall("getIssuers()"))
	if err != nil {
		return nil, fmt.Errorf("getIssuers: %w", err)
	}
	d := decoder{raw}
	arrayBase, err := d.offsetAt(0)
	if err != nil {
		return nil, err
	}
	n, err := d.intAt(arrayBase)
	if err != nil {
		return nil, err
	}
	items := arrayBase + wordSize
	out := make([]Issuer, 0, n)
	for i := int64(0); i < n; i++ {
		// Each element of a dynamic-tuple array is itself dynamic, so the slot
		// holds an offset relative to the start of the element region.
		rel, err := d.offsetAt(items + int(i)*wordSize)
		if err != nil {
			return nil, err
		}
		tuple := items + rel
		issuerOff, err := d.offsetAt(tuple + 0*wordSize)
		if err != nil {
			return nil, err
		}
		issuer, err := d.stringAt(tuple + issuerOff)
		if err != nil {
			return nil, err
		}
		clientsOff, err := d.offsetAt(tuple + 1*wordSize)
		if err != nil {
			return nil, err
		}
		clientIDs, err := d.stringArrayAt(tuple + clientsOff)
		if err != nil {
			return nil, err
		}
		out = append(out, Issuer{Issuer: issuer, ClientIDs: clientIDs})
	}
	return out, nil
}

// AuthKeys reads the group's trusted authorization keys as hex strings.
func (c *Client) AuthKeys(ctx context.Context, group string) ([]string, error) {
	raw, err := c.call(ctx, group, encodeCall("getAuthKeys()"))
	if err != nil {
		return nil, fmt.Errorf("getAuthKeys: %w", err)
	}
	keys, err := decoder{raw}.bytesArray()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, "0x"+encodeHex(k))
	}
	return out, nil
}

// IssuerHash is keccak256(abi.encodePacked(issuer)) — the key the group
// contract stores issuers under, and the argument removeIssuer takes.
func IssuerHash(issuer string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(issuer))
	return "0x" + encodeHex(h.Sum(nil))
}

// AuthKeyHash is keccak256(pubkey) — the handle removeAuthKey takes.
func AuthKeyHash(pubkeyHex string) (string, error) {
	raw, err := decodeHex(pubkeyHex)
	if err != nil {
		return "", err
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(raw)
	return "0x" + encodeHex(h.Sum(nil)), nil
}

func normalize(addr string) (string, error) {
	raw, err := decodeHex(addr)
	if err != nil {
		return "", err
	}
	if len(raw) != 20 {
		return "", fmt.Errorf("address must be 20 bytes, got %d", len(raw))
	}
	return "0x" + encodeHex(raw), nil
}

// ─────────────────────────── Receipts ───────────────────────────

// eventTopic is keccak256 of an event signature — the topic0 a log carries.
func eventTopic(signature string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(signature))
	return "0x" + encodeHex(h.Sum(nil))
}

type txLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

type txReceipt struct {
	Status string  `json:"status"`
	Logs   []txLog `json:"logs"`
}

// GroupCreatedIn reads a transaction receipt and returns the group the factory
// created in it.
//
// The bundler's UserOperation receipt reports only success and a transaction
// hash, so the group address has to come from the chain. Reading it here also
// means the address the console records is the one the factory actually
// emitted, rather than one predicted before the call.
func (c *Client) GroupCreatedIn(ctx context.Context, txHash string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("chain reads are not configured (set RPC_URL and FACTORY_ADDRESS)")
	}
	var rcpt *txReceipt
	if err := c.rpc(ctx, "eth_getTransactionReceipt", []any{txHash}, &rcpt); err != nil {
		return "", err
	}
	if rcpt == nil {
		return "", fmt.Errorf("transaction %s has no receipt yet", txHash)
	}
	if rcpt.Status != "" && rcpt.Status != "0x1" {
		return "", fmt.Errorf("transaction %s reverted", txHash)
	}

	want := eventTopic("GroupCreated(address,address,uint256)")
	for _, l := range rcpt.Logs {
		if !strings.EqualFold(strings.TrimSpace(l.Address), c.factory) {
			continue
		}
		if len(l.Topics) < 2 || !strings.EqualFold(l.Topics[0], want) {
			continue
		}
		// topic1 is the indexed group address, left-padded to 32 bytes.
		raw, err := decodeHex(l.Topics[1])
		if err != nil || len(raw) != 32 {
			return "", fmt.Errorf("malformed GroupCreated log in %s", txHash)
		}
		return "0x" + encodeHex(raw[12:]), nil
	}
	return "", fmt.Errorf(
		"%s did not create a group — no GroupCreated event from the factory at %s", txHash, c.factory)
}

// ─────────────────────────── SignetAccountFactory ───────────────────────────

// SmartAccountAddress asks the account factory for the CREATE2 address a group
// public key maps to.
//
// The console reports this address when it provisions a developer's key, and
// the platform then uses it to decide which operations it will sponsor — so it
// cannot be taken on trust. Deriving it here turns a claim into a fact: the
// factory computes the same address the EntryPoint will deploy to, and a
// mismatch means the console reported something it did not derive.
func (c *Client) SmartAccountAddress(
	ctx context.Context, accountFactory, entryPoint, groupPublicKey string,
) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("chain reads are not configured (set RPC_URL and FACTORY_ADDRESS)")
	}
	if strings.TrimSpace(accountFactory) == "" {
		return "", fmt.Errorf("no account factory address is configured")
	}
	epWord, err := addressWord(entryPoint)
	if err != nil {
		return "", fmt.Errorf("entry point: %w", err)
	}
	key, err := decodeHex(groupPublicKey)
	if err != nil {
		return "", fmt.Errorf("group public key is not hex: %w", err)
	}
	if len(key) == 0 {
		return "", fmt.Errorf("group public key is empty")
	}

	// getAddress(address,bytes,uint256): the dynamic argument sits after the
	// three head words, so its offset is 0x60.
	data := encodeCall("getAddress(address,bytes,uint256)",
		epWord,
		bigWord(96), // offset to the bytes argument
		nil,         // salt 0
	)
	data = append(data, padWord(bigWord(int64(len(key))))...)
	data = append(data, rightPad(key)...)

	raw, err := c.call(ctx, accountFactory, data)
	if err != nil {
		return "", err
	}
	return decoder{raw}.addressAt(0)
}

func bigWord(v int64) []byte {
	return new(big.Int).SetInt64(v).Bytes()
}

func padWord(b []byte) []byte {
	w := make([]byte, wordSize)
	copy(w[wordSize-len(b):], b)
	return w
}

// rightPad pads dynamic bytes up to a whole number of ABI words.
func rightPad(b []byte) []byte {
	n := len(b)
	if n%wordSize != 0 {
		n += wordSize - (n % wordSize)
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}

// ─────────────────────────── Transactions ───────────────────────────
//
// The console can reach the chain two ways. A user operation goes out through
// the bundler, which hands back a receipt, so the platform learns the outcome
// by submitting it. A transaction sent from the developer's own wallet never
// passes through here at all — the browser submits it and the platform is told
// a hash afterwards. These two reads are how that hash becomes evidence rather
// than a claim.

// SentTransaction is the part of a transaction the platform needs in order to
// decide whether it authorizes what the caller says it does.
type SentTransaction struct {
	From     string
	To       string
	Input    []byte
	Value    *big.Int
	Mined    bool
	Success  bool
	BlockNum uint64
}

type rpcTransaction struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Input string `json:"input"`
	Value string `json:"value"`
}

type rpcReceipt struct {
	Status      string `json:"status"`
	BlockNumber string `json:"blockNumber"`
}

// Transaction reads a transaction and its receipt.
//
// Both, because either alone is insufficient: the transaction carries what was
// called and by whom, and the receipt says whether it succeeded. A reverted
// transaction is on-chain and readable and changed nothing, so recording
// metadata off the back of one would leave the console asserting a state the
// operators never saw.
//
// Mined is false rather than an error when the transaction is still pending —
// the caller decides whether to wait, and "not yet" is not a failure.
func (c *Client) Transaction(ctx context.Context, hash string) (*SentTransaction, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("chain reads are not configured")
	}
	hash = strings.TrimSpace(hash)
	if !isTxHash(hash) {
		return nil, fmt.Errorf("%q is not a transaction hash", hash)
	}

	var tx *rpcTransaction
	if err := c.rpc(ctx, "eth_getTransactionByHash", []any{hash}, &tx); err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, fmt.Errorf("no transaction with that hash on chain %d", c.chainID)
	}

	input, err := decodeHexBytes(tx.Input)
	if err != nil {
		return nil, fmt.Errorf("transaction input is not hex: %w", err)
	}
	out := &SentTransaction{
		From:  strings.ToLower(tx.From),
		To:    strings.ToLower(tx.To),
		Input: input,
		Value: hexToBig(tx.Value),
	}

	var receipt *rpcReceipt
	if err := c.rpc(ctx, "eth_getTransactionReceipt", []any{hash}, &receipt); err != nil {
		return nil, err
	}
	if receipt == nil {
		return out, nil // still pending
	}
	out.Mined = true
	out.Success = hexToBig(receipt.Status).Sign() == 1
	out.BlockNum = hexToBig(receipt.BlockNumber).Uint64()
	return out, nil
}

func isTxHash(s string) bool {
	if len(s) != 66 || !strings.HasPrefix(s, "0x") {
		return false
	}
	_, err := hex.DecodeString(s[2:])
	return err == nil
}

func decodeHexBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	if s == "" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

func hexToBig(s string) *big.Int {
	n, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimSpace(s), "0x"), 16)
	if !ok {
		return new(big.Int)
	}
	return n
}

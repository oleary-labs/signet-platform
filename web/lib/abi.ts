/**
 * Contract ABIs, hand-narrowed to the functions the console actually calls.
 *
 * Full Foundry artifacts are not vendored here: the console only ever calls a
 * handful of functions, and a narrow ABI makes it obvious at review time what
 * this app is capable of asking a wallet to sign.
 *
 * Source: signet-protocol/contracts/contracts/{SignetFactory,SignetGroup}.sol
 * and signet-wallet's account factory.
 */

export const signetFactoryAbi = [
  {
    type: "function",
    name: "createGroup",
    stateMutability: "nonpayable",
    inputs: [
      { name: "nodeAddrs", type: "address[]" },
      { name: "threshold", type: "uint256" },
      { name: "removalDelay", type: "uint256" },
      {
        name: "initialIssuers",
        type: "tuple[]",
        components: [
          { name: "issuer", type: "string" },
          { name: "clientIds", type: "string[]" },
        ],
      },
      { name: "initialAuthKeys", type: "bytes[]" },
    ],
    outputs: [{ name: "group", type: "address" }],
  },
  {
    type: "function",
    name: "getGroupsByManager",
    stateMutability: "view",
    inputs: [{ name: "mgr", type: "address" }],
    outputs: [{ name: "", type: "address[]" }],
  },
  {
    type: "event",
    name: "GroupCreated",
    inputs: [
      { name: "group", type: "address", indexed: true },
      { name: "creator", type: "address", indexed: true },
      { name: "threshold", type: "uint256", indexed: false },
    ],
  },
] as const;

export const signetGroupAbi = [
  { type: "function", name: "inviteNode", stateMutability: "nonpayable", inputs: [{ name: "node", type: "address" }], outputs: [] },
  { type: "function", name: "queueRemoval", stateMutability: "nonpayable", inputs: [{ name: "node", type: "address" }], outputs: [] },
  { type: "function", name: "cancelRemoval", stateMutability: "nonpayable", inputs: [{ name: "node", type: "address" }], outputs: [] },
  { type: "function", name: "executeRemoval", stateMutability: "nonpayable", inputs: [{ name: "node", type: "address" }], outputs: [] },
  {
    type: "function",
    name: "addIssuer",
    stateMutability: "nonpayable",
    inputs: [
      { name: "issuer", type: "string" },
      { name: "clientIds", type: "string[]" },
    ],
    outputs: [],
  },
  { type: "function", name: "removeIssuer", stateMutability: "nonpayable", inputs: [{ name: "issuerHash", type: "bytes32" }], outputs: [] },
  { type: "function", name: "addAuthKey", stateMutability: "nonpayable", inputs: [{ name: "pubkey", type: "bytes" }], outputs: [] },
  { type: "function", name: "removeAuthKey", stateMutability: "nonpayable", inputs: [{ name: "keyHash", type: "bytes32" }], outputs: [] },
  { type: "function", name: "transferManager", stateMutability: "nonpayable", inputs: [{ name: "newManager", type: "address" }], outputs: [] },
  { type: "function", name: "requestReshare", stateMutability: "nonpayable", inputs: [], outputs: [] },
  {
    type: "function",
    name: "queueAuthResolver",
    stateMutability: "nonpayable",
    inputs: [
      { name: "chainId", type: "uint64" },
      { name: "resolver", type: "address" },
      { name: "requireCanonicalSubject", type: "bool" },
    ],
    outputs: [],
  },
  { type: "function", name: "cancelAuthResolver", stateMutability: "nonpayable", inputs: [], outputs: [] },
  { type: "function", name: "executeAuthResolver", stateMutability: "nonpayable", inputs: [], outputs: [] },
] as const;

export const signetAccountFactoryAbi = [
  {
    type: "function",
    name: "getAddress",
    stateMutability: "view",
    inputs: [
      { name: "entryPoint", type: "address" },
      { name: "groupPublicKey", type: "bytes" },
      { name: "salt", type: "uint256" },
    ],
    outputs: [{ name: "", type: "address" }],
  },
  {
    type: "function",
    name: "createAccount",
    stateMutability: "nonpayable",
    inputs: [
      { name: "entryPoint", type: "address" },
      { name: "groupPublicKey", type: "bytes" },
      { name: "salt", type: "uint256" },
    ],
    outputs: [{ name: "", type: "address" }],
  },
] as const;

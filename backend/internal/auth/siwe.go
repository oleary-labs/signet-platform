package auth

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SIWEMessage is the parsed form of an ERC-4361 "Sign-In with Ethereum"
// message. Only the fields the platform actually checks are retained.
type SIWEMessage struct {
	Domain         string
	Address        string
	Statement      string
	URI            string
	Version        string
	ChainID        int64
	Nonce          string
	IssuedAt       time.Time
	ExpirationTime *time.Time
	NotBefore      *time.Time
}

// ParseSIWE parses an ERC-4361 message. It is deliberately strict: a field it
// cannot parse is an error rather than a silently ignored line, because every
// unparsed line is a claim the signer made that the server never checked.
func ParseSIWE(raw string) (*SIWEMessage, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	if len(lines) < 6 {
		return nil, fmt.Errorf("message is too short to be ERC-4361")
	}

	const wantSuffix = " wants you to sign in with your Ethereum account:"
	if !strings.HasSuffix(lines[0], wantSuffix) {
		return nil, fmt.Errorf("first line is not the ERC-4361 preamble")
	}
	msg := &SIWEMessage{Domain: strings.TrimSuffix(lines[0], wantSuffix)}
	if msg.Domain == "" {
		return nil, fmt.Errorf("preamble carries no domain")
	}

	addr, err := NormalizeAddress(lines[1])
	if err != nil {
		return nil, fmt.Errorf("address line: %w", err)
	}
	msg.Address = addr

	// Line 2 is blank. An optional statement occupies line 3, followed by
	// another blank line; when absent, the fields start straight after.
	i := 2
	if i < len(lines) && lines[i] == "" {
		i++
	}
	if i < len(lines) && lines[i] != "" && !strings.Contains(lines[i], ": ") {
		msg.Statement = lines[i]
		i++
		if i < len(lines) && lines[i] == "" {
			i++
		}
	}

	inResources := false
	for ; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		if inResources && strings.HasPrefix(line, "- ") {
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			if strings.TrimSpace(line) == "Resources:" {
				inResources = true
				continue
			}
			return nil, fmt.Errorf("unparsable line %q", line)
		}
		inResources = false
		switch key {
		case "URI":
			msg.URI = value
		case "Version":
			msg.Version = value
		case "Chain ID":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("chain id %q is not a number", value)
			}
			msg.ChainID = n
		case "Nonce":
			msg.Nonce = value
		case "Issued At":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, fmt.Errorf("issued-at %q is not RFC 3339", value)
			}
			msg.IssuedAt = t
		case "Expiration Time":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, fmt.Errorf("expiration %q is not RFC 3339", value)
			}
			msg.ExpirationTime = &t
		case "Not Before":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return nil, fmt.Errorf("not-before %q is not RFC 3339", value)
			}
			msg.NotBefore = &t
		case "Request ID":
			// Recorded by the client, not meaningful to the platform.
		default:
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}

	if msg.Nonce == "" {
		return nil, fmt.Errorf("message has no nonce")
	}
	if msg.Version != "1" {
		return nil, fmt.Errorf("unsupported ERC-4361 version %q", msg.Version)
	}
	return msg, nil
}

// SIWEExpectation is what the server requires of a SIWE message before it will
// accept the signature as a login.
type SIWEExpectation struct {
	Domain  string
	Nonce   string
	ChainID int64 // 0 means any chain is acceptable
	Now     time.Time
}

// VerifySIWE parses the message, checks it against the server's expectations,
// and recovers the signing address. The recovered address is returned only
// when every check passes, so a caller cannot accidentally trust a partially
// validated message.
func VerifySIWE(raw string, sig []byte, want SIWEExpectation) (*SIWEMessage, error) {
	msg, err := ParseSIWE(raw)
	if err != nil {
		return nil, err
	}
	if want.Domain == "" {
		return nil, fmt.Errorf("siwe login is not configured on this server")
	}
	if !strings.EqualFold(msg.Domain, want.Domain) {
		return nil, fmt.Errorf("message domain %q does not match this server", msg.Domain)
	}
	if msg.Nonce != want.Nonce {
		return nil, fmt.Errorf("message nonce does not match the issued challenge")
	}
	if want.ChainID != 0 && msg.ChainID != want.ChainID {
		return nil, fmt.Errorf("message is for chain %d, expected %d", msg.ChainID, want.ChainID)
	}
	now := want.Now
	if now.IsZero() {
		now = time.Now()
	}
	if msg.ExpirationTime != nil && !now.Before(*msg.ExpirationTime) {
		return nil, fmt.Errorf("message has expired")
	}
	if msg.NotBefore != nil && now.Before(*msg.NotBefore) {
		return nil, fmt.Errorf("message is not yet valid")
	}

	recovered, err := RecoverPersonalSigner([]byte(raw), sig)
	if err != nil {
		return nil, err
	}
	if recovered != msg.Address {
		return nil, fmt.Errorf("%w: signature recovers to %s, message claims %s", ErrInvalidSignature, recovered, msg.Address)
	}
	return msg, nil
}

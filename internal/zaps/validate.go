package zaps

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// ValidatedZap contains extracted information from a valid zap receipt.
type ValidatedZap struct {
	SenderNpub string // Npub of the zapper
	AmountSats int64  // Amount in sats (from bolt11)
	ZapEventID string // Event ID of the zap receipt
}

// ErrInvalidZapReceipt indicates the zap receipt is malformed or invalid.
var ErrInvalidZapReceipt = errors.New("invalid zap receipt")

// ErrUnauthorizedZapProvider indicates the zap was signed by an unexpected key.
var ErrUnauthorizedZapProvider = errors.New("unauthorized zap provider")

// ValidateZapReceipt validates a NIP-57 zap receipt and extracts payment info.
// lnurlPubkeyHex is the expected LNURL provider's pubkey that should sign zap receipts.
// If lnurlPubkeyHex is empty, the provider check is skipped.
func ValidateZapReceipt(event *nostr.Event, lnurlPubkeyHex string) (*ValidatedZap, error) {
	// Verify event kind
	if event.Kind != nostr.KindZap {
		return nil, fmt.Errorf("%w: expected kind %d, got %d", ErrInvalidZapReceipt, nostr.KindZap, event.Kind)
	}

	// Verify signature
	ok, err := event.CheckSignature()
	if err != nil || !ok {
		return nil, fmt.Errorf("%w: invalid signature", ErrInvalidZapReceipt)
	}

	// Verify zap provider if configured
	if lnurlPubkeyHex != "" && event.PubKey != lnurlPubkeyHex {
		// Convert hex to npub for human-readable error message
		expectedNpub, _ := nip19.EncodePublicKey(lnurlPubkeyHex)
		gotNpub, _ := nip19.EncodePublicKey(event.PubKey)
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrUnauthorizedZapProvider, expectedNpub, gotNpub)
	}

	// Extract description tag (contains serialized zap request)
	descTag := event.Tags.Find("description")
	if len(descTag) < 2 {
		return nil, fmt.Errorf("%w: missing description tag", ErrInvalidZapReceipt)
	}
	descJSON := descTag[1]

	// Parse zap request from description
	var zapRequest nostr.Event
	if err := json.Unmarshal([]byte(descJSON), &zapRequest); err != nil {
		return nil, fmt.Errorf("%w: invalid zap request JSON: %v", ErrInvalidZapReceipt, err)
	}

	// Verify zap request is kind 9734
	if zapRequest.Kind != nostr.KindZapRequest {
		return nil, fmt.Errorf("%w: zap request kind is %d, expected %d", ErrInvalidZapReceipt, zapRequest.Kind, nostr.KindZapRequest)
	}

	// Sender pubkey is the pubkey of the zap request
	senderPubkeyHex := zapRequest.PubKey
	if senderPubkeyHex == "" {
		return nil, fmt.Errorf("%w: zap request missing pubkey", ErrInvalidZapReceipt)
	}

	// Extract amount from bolt11 tag
	bolt11Tag := event.Tags.Find("bolt11")
	if len(bolt11Tag) < 2 {
		return nil, fmt.Errorf("%w: missing bolt11 tag", ErrInvalidZapReceipt)
	}
	bolt11 := bolt11Tag[1]

	amountMsats, err := extractAmountFromBolt11(bolt11)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidZapReceipt, err)
	}

	// Convert millisats to sats (integer division, round down)
	amountSats := amountMsats / 1000

	// Encode sender pubkey as npub
	senderNpub, err := nip19.EncodePublicKey(senderPubkeyHex)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to encode sender npub: %v", ErrInvalidZapReceipt, err)
	}

	return &ValidatedZap{
		SenderNpub: senderNpub,
		AmountSats: amountSats,
		ZapEventID: event.ID,
	}, nil
}

// extractAmountFromBolt11 extracts the amount in millisats from a BOLT11 invoice.
// BOLT11 format: lnbc<amount><multiplier>1<data>
// Multipliers: m = milli (10^-3), u = micro (10^-6), n = nano (10^-9), p = pico (10^-12)
func extractAmountFromBolt11(invoice string) (int64, error) {
	invoice = strings.ToLower(invoice)

	// Find the prefix (lnbc, lntb, lnbcrt, etc.)
	var amountStart int
	if strings.HasPrefix(invoice, "lnbcrt") {
		amountStart = 6
	} else if strings.HasPrefix(invoice, "lnbc") {
		amountStart = 4
	} else if strings.HasPrefix(invoice, "lntb") {
		amountStart = 4
	} else {
		return 0, fmt.Errorf("unrecognized invoice prefix")
	}

	// The separator '1' divides HRP from data. The data part uses bech32 which
	// doesn't include '1', so we find the last '1' to get the separator.
	// Format: ln<network><amount>[multiplier]1<timestamp><tagged_fields><signature>
	sepIndex := strings.LastIndex(invoice, "1")
	if sepIndex == -1 || sepIndex <= amountStart {
		return 0, fmt.Errorf("invalid invoice format: no separator found")
	}

	amountPart := invoice[amountStart:sepIndex]
	if amountPart == "" {
		return 0, fmt.Errorf("no amount in invoice")
	}

	// Find the multiplier (last char if it's a letter)
	var multiplier int64
	var numStr string

	lastChar := amountPart[len(amountPart)-1]
	if lastChar >= '0' && lastChar <= '9' {
		// No multiplier, amount is in BTC
		numStr = amountPart
		multiplier = 100_000_000_000 // BTC to msats
	} else {
		numStr = amountPart[:len(amountPart)-1]
		switch lastChar {
		case 'm': // milli-BTC = 100,000 sats = 100,000,000 msats
			multiplier = 100_000_000
		case 'u': // micro-BTC = 100 sats = 100,000 msats
			multiplier = 100_000
		case 'n': // nano-BTC = 0.1 sat = 100 msats
			multiplier = 100
		case 'p': // pico-BTC = 0.0001 sat = 0.1 msats (round to 0)
			multiplier = 0 // Will round to 0 msats
		default:
			return 0, fmt.Errorf("unknown multiplier: %c", lastChar)
		}
	}

	amount, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount number: %v", err)
	}

	return amount * multiplier, nil
}

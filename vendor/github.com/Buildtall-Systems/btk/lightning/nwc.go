package lightning

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/keyer"
	"github.com/nbd-wtf/go-nostr/nip04"
)

const (
	nwcRequestTimeout = 15 * time.Second
	nwcLookupTimeout  = 10 * time.Second
)

type nwcRequest struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

type nwcResponse struct {
	Error      *nwcError       `json:"error,omitempty"`
	ResultType string          `json:"result_type"`
	Result     json.RawMessage `json:"result,omitempty"`
}

type nwcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type makeInvoiceParams struct {
	Description string `json:"description,omitempty"`
	Amount      int64  `json:"amount"`
}

type makeInvoiceResult struct {
	Invoice     string `json:"invoice"`
	PaymentHash string `json:"payment_hash"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}

type lookupInvoiceParams struct {
	PaymentHash string `json:"payment_hash"`
}

type lookupInvoiceResult struct {
	Invoice     string `json:"invoice"`
	PaymentHash string `json:"payment_hash"`
	Preimage    string `json:"preimage,omitempty"`
	State       string `json:"state,omitempty"`
	SettledAt   int64  `json:"settled_at,omitempty"`
}

type NWCBackend struct {
	walletPubkey string
	appKey       string
	relayURL     string
	pool         *nostr.SimplePool
	ownsPool     bool
	encryption   Encryption

	sharedSecret []byte
	keyer        nostr.Keyer
}

func NewNWCBackend(connectionString string, opts ...NWCOption) (*NWCBackend, error) {
	params, err := ParseConnectionString(connectionString)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	b := &NWCBackend{
		walletPubkey: params.WalletPubkey,
		appKey:       params.AppKey,
		relayURL:     params.RelayURL,
	}

	for _, opt := range opts {
		opt(b)
	}

	if b.pool == nil {
		b.pool = nostr.NewSimplePool(context.Background())
		b.ownsPool = true
	}

	switch b.encryption {
	case EncryptionNIP04:
		ss, err := nip04.ComputeSharedSecret(params.WalletPubkey, params.AppKey)
		if err != nil {
			return nil, fmt.Errorf("computing shared secret: %w", err)
		}
		b.sharedSecret = ss
	case EncryptionNIP44:
		ks, err := keyer.NewPlainKeySigner(params.AppKey)
		if err != nil {
			return nil, fmt.Errorf("creating keysigner: %w", err)
		}
		b.keyer = ks
	default:
		return nil, fmt.Errorf("unsupported encryption mode: %d", b.encryption)
	}

	return b, nil
}

func (n *NWCBackend) CreateInvoice(ctx context.Context, amountSats int64, memo string) (*Invoice, error) {
	req := nwcRequest{
		Method: "make_invoice",
		Params: makeInvoiceParams{
			Amount:      amountSats * 1000,
			Description: memo,
		},
	}

	resultBytes, err := n.sendRequest(ctx, req, nwcRequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("make_invoice: %w", err)
	}

	var result makeInvoiceResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing make_invoice result: %w", err)
	}

	return &Invoice{
		PaymentHash:    result.PaymentHash,
		PaymentRequest: result.Invoice,
		Memo:           memo,
		AmountSats:     amountSats,
		ExpiresAt:      result.ExpiresAt,
	}, nil
}

func (n *NWCBackend) CheckInvoicePaid(ctx context.Context, paymentHash string) (bool, error) {
	req := nwcRequest{
		Method: "lookup_invoice",
		Params: lookupInvoiceParams{
			PaymentHash: paymentHash,
		},
	}

	resultBytes, err := n.sendRequest(ctx, req, nwcLookupTimeout)
	if err != nil {
		return false, fmt.Errorf("lookup_invoice: %w", err)
	}

	var result lookupInvoiceResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return false, fmt.Errorf("parsing lookup_invoice result: %w", err)
	}

	return result.State == "settled" || result.SettledAt > 0, nil
}

func (n *NWCBackend) sendRequest(ctx context.Context, req nwcRequest, timeout time.Duration) (json.RawMessage, error) {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	var encrypted string
	switch n.encryption {
	case EncryptionNIP04:
		encrypted, err = nip04.Encrypt(string(reqJSON), n.sharedSecret)
	case EncryptionNIP44:
		encrypted, err = n.keyer.Encrypt(ctx, string(reqJSON), n.walletPubkey)
	}
	if err != nil {
		return nil, fmt.Errorf("encrypting request: %w", err)
	}

	appPubkey, err := nostr.GetPublicKey(n.appKey)
	if err != nil {
		return nil, fmt.Errorf("deriving app pubkey: %w", err)
	}

	evt := nostr.Event{
		PubKey:    appPubkey,
		CreatedAt: nostr.Now(),
		Kind:      nostr.KindNWCWalletRequest,
		Content:   encrypted,
		Tags:      nostr.Tags{nostr.Tag{"p", n.walletPubkey}},
	}

	if n.encryption == EncryptionNIP44 {
		evt.Tags = append(evt.Tags, nostr.Tag{"encryption", "nip44_v2"})
	}

	switch n.encryption {
	case EncryptionNIP04:
		if signErr := evt.Sign(n.appKey); signErr != nil {
			return nil, fmt.Errorf("signing event: %w", signErr)
		}
	case EncryptionNIP44:
		if signErr := n.keyer.SignEvent(ctx, &evt); signErr != nil {
			return nil, fmt.Errorf("signing event: %w", signErr)
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	subCtx, subCancel := context.WithCancel(timeoutCtx)
	defer subCancel()

	respChan := n.pool.SubscribeMany(subCtx, []string{n.relayURL}, nostr.Filter{
		Kinds: []int{nostr.KindNWCWalletResponse},
		Tags:  nostr.TagMap{"e": {evt.ID}},
	})

	results := n.pool.PublishMany(timeoutCtx, []string{n.relayURL}, evt)
	for res := range results {
		if res.Error != nil {
			return nil, fmt.Errorf("publishing to %s: %w", res.RelayURL, res.Error)
		}
	}

	select {
	case relayEvt, ok := <-respChan:
		if !ok || relayEvt.Event == nil {
			return nil, fmt.Errorf("subscription closed without response")
		}

		var decrypted string
		switch n.encryption {
		case EncryptionNIP04:
			decrypted, err = nip04.Decrypt(relayEvt.Content, n.sharedSecret)
		case EncryptionNIP44:
			decrypted, err = n.keyer.Decrypt(ctx, relayEvt.Content, n.walletPubkey)
		}
		if err != nil {
			return nil, fmt.Errorf("decrypting response: %w", err)
		}

		var resp nwcResponse
		if err := json.Unmarshal([]byte(decrypted), &resp); err != nil {
			return nil, fmt.Errorf("unmarshaling response: %w", err)
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("wallet error [%s]: %s", resp.Error.Code, resp.Error.Message)
		}

		if resp.Result == nil {
			return nil, fmt.Errorf("response missing result for %s", resp.ResultType)
		}

		return resp.Result, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timed out waiting for wallet response")
	}
}

package lightning

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
)

const (
	KindNWCRequest  = 23194
	KindNWCResponse = 23195

	nwcRequestTimeout = 15 * time.Second
	nwcLookupTimeout  = 10 * time.Second
)

type nwcRequest struct {
	Params any    `json:"params"`
	Method string `json:"method"`
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
}

type lookupInvoiceParams struct {
	PaymentHash string `json:"payment_hash"`
}

type lookupInvoiceResult struct {
	Invoice     string `json:"invoice"`
	PaymentHash string `json:"payment_hash"`
	Preimage    string `json:"preimage,omitempty"`
	State       string `json:"state,omitempty"`
}

type NWCBackend struct {
	walletPubkey string
	appKey       string
	relayURL     string
	sharedSecret []byte
}

func NewNWCBackend(connectionString string) (*NWCBackend, error) {
	params, err := ParseNWCConnectionString(connectionString)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	ss, err := nip04.ComputeSharedSecret(params.WalletPubkey, params.AppKey)
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}

	return &NWCBackend{
		walletPubkey: params.WalletPubkey,
		appKey:       params.AppKey,
		relayURL:     params.RelayURL,
		sharedSecret: ss,
	}, nil
}

func (n *NWCBackend) CreateInvoice(ctx context.Context, amountSats int64, memo string) (*Invoice, error) {
	req := nwcRequest{
		Method: "make_invoice",
		Params: makeInvoiceParams{
			Amount:      amountSats * 1000,
			Description: memo,
		},
	}

	respBytes, err := n.sendRequest(ctx, req, nwcRequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("make_invoice: %w", err)
	}

	var resp nwcResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("parsing make_invoice response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("make_invoice error [%s]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result makeInvoiceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parsing make_invoice result: %w", err)
	}

	return &Invoice{
		PaymentHash:    result.PaymentHash,
		PaymentRequest: result.Invoice,
		AmountSats:     amountSats,
		Memo:           memo,
		Paid:           false,
	}, nil
}

func (n *NWCBackend) CheckInvoicePaid(ctx context.Context, paymentHash string) (bool, error) {
	req := nwcRequest{
		Method: "lookup_invoice",
		Params: lookupInvoiceParams{
			PaymentHash: paymentHash,
		},
	}

	respBytes, err := n.sendRequest(ctx, req, nwcLookupTimeout)
	if err != nil {
		return false, fmt.Errorf("lookup_invoice: %w", err)
	}

	var resp nwcResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return false, fmt.Errorf("parsing lookup_invoice response: %w", err)
	}

	if resp.Error != nil {
		return false, fmt.Errorf("lookup_invoice error [%s]: %s", resp.Error.Code, resp.Error.Message)
	}

	var result lookupInvoiceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return false, fmt.Errorf("parsing lookup_invoice result: %w", err)
	}

	return result.State == "settled", nil
}

func (n *NWCBackend) sendRequest(ctx context.Context, req nwcRequest, timeout time.Duration) ([]byte, error) {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	encrypted, err := nip04.Encrypt(string(reqJSON), n.sharedSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting request: %w", err)
	}

	appPubkey, err := nostr.GetPublicKey(n.appKey)
	if err != nil {
		return nil, fmt.Errorf("deriving app pubkey: %w", err)
	}

	ev := nostr.Event{
		PubKey:    appPubkey,
		CreatedAt: nostr.Now(),
		Kind:      KindNWCRequest,
		Content:   encrypted,
		Tags:      nostr.Tags{nostr.Tag{"p", n.walletPubkey}},
	}

	if signErr := ev.Sign(n.appKey); signErr != nil {
		return nil, fmt.Errorf("signing event: %w", signErr)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	relay, err := nostr.RelayConnect(timeoutCtx, n.relayURL)
	if err != nil {
		return nil, fmt.Errorf("connecting to relay: %w", err)
	}
	defer func() {
		if closeErr := relay.Close(); closeErr != nil {
			return
		}
	}()

	sub, err := relay.Subscribe(timeoutCtx, nostr.Filters{{
		Kinds:   []int{KindNWCResponse},
		Authors: []string{n.walletPubkey},
		Tags:    nostr.TagMap{"e": []string{ev.ID}},
		Limit:   1,
	}})
	if err != nil {
		return nil, fmt.Errorf("subscribing for response: %w", err)
	}
	defer sub.Unsub()

	if err := relay.Publish(timeoutCtx, ev); err != nil {
		return nil, fmt.Errorf("publishing request: %w", err)
	}

	select {
	case respEvent := <-sub.Events:
		if respEvent == nil {
			return nil, fmt.Errorf("subscription closed without response")
		}
		decrypted, decErr := nip04.Decrypt(respEvent.Content, n.sharedSecret)
		if decErr != nil {
			return nil, fmt.Errorf("decrypting response: %w", decErr)
		}
		return []byte(decrypted), nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timed out waiting for wallet response")
	}
}

package lightning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client handles LNURL-pay operations for generating bolt11 invoices.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new LNURL-pay client with reasonable defaults.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewClientWithHTTP creates a client with a custom http.Client (for testing).
func NewClientWithHTTP(c *http.Client) *Client {
	return &Client{httpClient: c}
}

// LNURLPayMetadata contains response from LNURL-pay well-known endpoint.
type LNURLPayMetadata struct {
	Callback       string `json:"callback"`
	MinSendable    int64  `json:"minSendable"`    // millisats
	MaxSendable    int64  `json:"maxSendable"`    // millisats
	CommentAllowed int    `json:"commentAllowed"` // max comment length, 0 = not allowed
	Tag            string `json:"tag"`            // should be "payRequest"
}

// InvoiceResponse contains the bolt11 invoice from callback.
type InvoiceResponse struct {
	PR     string `json:"pr"`     // bolt11 invoice
	Routes []any  `json:"routes"` // routing hints (unused)
}

// FetchMetadata retrieves LNURL-pay metadata for a lightning address.
// lightningAddress format: "user@domain.com"
func (c *Client) FetchMetadata(ctx context.Context, lightningAddress string) (*LNURLPayMetadata, error) {
	// Parse lightning address: user@domain -> https://domain/.well-known/lnurlp/user
	parts := strings.SplitN(lightningAddress, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("%w: expected user@domain format", ErrInvalidLightningAddress)
	}
	user, domain := parts[0], parts[1]

	url := fmt.Sprintf("https://%s/.well-known/lnurlp/%s", domain, user)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: creating request: %v", ErrLNURLMetadataFetch, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLNURLMetadataFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrLNURLMetadataFetch, resp.StatusCode)
	}

	var meta LNURLPayMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrLNURLMetadataFetch, err)
	}

	if meta.Callback == "" {
		return nil, fmt.Errorf("%w: missing callback URL", ErrLNURLMetadataFetch)
	}

	return &meta, nil
}

// RequestInvoice requests a bolt11 invoice for the given amount.
// amountSats is the invoice amount in satoshis.
// Returns the bolt11 invoice string (e.g., "lnbc32000n1...").
func (c *Client) RequestInvoice(ctx context.Context, lightningAddress string, amountSats int64) (string, error) {
	meta, err := c.FetchMetadata(ctx, lightningAddress)
	if err != nil {
		return "", err
	}

	amountMsats := amountSats * 1000

	// Validate amount bounds
	if amountMsats < meta.MinSendable {
		return "", fmt.Errorf("%w: %d sats below minimum %d sats",
			ErrInvoiceAmountOutOfRange,
			amountSats,
			meta.MinSendable/1000)
	}
	if amountMsats > meta.MaxSendable {
		return "", fmt.Errorf("%w: %d sats above maximum %d sats",
			ErrInvoiceAmountOutOfRange,
			amountSats,
			meta.MaxSendable/1000)
	}

	// Request invoice from callback URL
	// Callback URL may already have query params, so we need to handle that
	separator := "?"
	if strings.Contains(meta.Callback, "?") {
		separator = "&"
	}
	callbackURL := fmt.Sprintf("%s%samount=%d", meta.Callback, separator, amountMsats)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: creating request: %v", ErrLNURLInvoiceRequest, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrLNURLInvoiceRequest, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: HTTP %d", ErrLNURLInvoiceRequest, resp.StatusCode)
	}

	var invoiceResp InvoiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&invoiceResp); err != nil {
		return "", fmt.Errorf("%w: invalid JSON: %v", ErrLNURLInvoiceRequest, err)
	}

	if invoiceResp.PR == "" {
		return "", fmt.Errorf("%w: empty invoice returned", ErrLNURLInvoiceRequest)
	}

	return invoiceResp.PR, nil
}

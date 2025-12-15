package lightning

import "errors"

// ErrLNURLMetadataFetch indicates failure to fetch LNURL-pay metadata.
var ErrLNURLMetadataFetch = errors.New("failed to fetch LNURL metadata")

// ErrLNURLInvoiceRequest indicates failure to request invoice from callback.
var ErrLNURLInvoiceRequest = errors.New("failed to request LNURL invoice")

// ErrInvoiceAmountOutOfRange indicates requested amount is outside min/max bounds.
var ErrInvoiceAmountOutOfRange = errors.New("amount outside LNURL pay range")

// ErrInvalidLightningAddress indicates the lightning address format is invalid.
var ErrInvalidLightningAddress = errors.New("invalid lightning address format")

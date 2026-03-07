package lightning

import "errors"

var ErrNWCConnectionFailed = errors.New("NWC connection failed")

var ErrNWCRequestFailed = errors.New("NWC request failed")

var ErrInvalidConnectionString = errors.New("invalid NWC connection string")

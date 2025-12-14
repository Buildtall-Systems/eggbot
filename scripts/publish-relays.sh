#!/usr/bin/env bash
# Publish eggbot relay list (kind:10002 / NIP-65) to Nostr relays
# Requires: nak, NSEC environment variable

set -euo pipefail

if [[ -z "${NSEC:-}" ]]; then
    echo "Error: NSEC environment variable not set" >&2
    exit 1
fi

RELAYS=(
    "wss://relay.damus.io"
    "wss://nos.lol"
    "wss://relay.nostr.band"
    "wss://relay.primal.net"
)

echo "Publishing relay list (NIP-65) for eggbot..."
# r tags without marker = both read and write
nak event -k 10002 -c "" \
    -t r="wss://relay.damus.io" \
    -t r="wss://nos.lol" \
    -t r="wss://relay.nostr.band" \
    -t r="wss://relay.primal.net" \
    --sec "$NSEC" "${RELAYS[@]}"
echo "Done."

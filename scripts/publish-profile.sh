#!/usr/bin/env bash
# Publish eggbot profile (kind:0) to Nostr relays
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

PROFILE='{
  "name": "eggbot",
  "about": "Fresh eggs from buildtall farm. DM '\''help'\'' for commands. Accepts Bitcoin via Lightning.",
  "picture": "https://buildtall.com/img/eggbot-avatar.png",
  "nip05": "eggbot@buildtall.com",
  "lud16": "eggbot@getalby.com"
}'

# Compact JSON to single line
CONTENT=$(echo "$PROFILE" | jq -c .)

echo "Publishing profile for eggbot..."
nak event -k 0 -c "$CONTENT" --sec "$NSEC" "${RELAYS[@]}"
echo "Done."

# Eggbot

Nostr egg sales bot. Manages inventory, customer orders, and accepts Lightning zap payments via encrypted DMs.

## Overview

Eggbot is a Nostr bot that operates via NIP-17 encrypted direct messages. Customers send commands to order eggs and check their balance. The bot accepts Lightning zaps for payment and automatically credits customer accounts.

**Features**:
- Encrypted DM communication (NIP-17 gift wrap)
- Lightning zap payment acceptance (NIP-57)
- SQLite database for inventory, orders, and transactions
- Customer whitelist for access control
- Admin commands for inventory and order management

## Build

```bash
nix develop
make build
```

Binary output: `bin/eggbot`

## Configuration

### Config File

Create `/etc/eggbot/config.yaml`:

```yaml
verbose: true

database:
  path: "/var/lib/eggbot/eggbot.db"

nostr:
  relays:
    - "wss://relay.damus.io"
    - "wss://nos.lol"
  bot_npub: "npub1..."  # Bot's public key

lightning:
  # LNURL provider pubkey that signs zap receipts
  # Leave empty to accept zaps from any provider (less secure)
  lnurl_pubkey: "npub1..."  # e.g., getalby's npub

pricing:
  sats_per_half_dozen: 3200

# Admin npubs (can manage inventory, customers, orders)
admins:
  - "npub1..."
```

### Environment File

Create `/etc/eggbot/eggbot.env`:

```bash
# Bot's secret key (REQUIRED)
EGGBOT_NSEC=nsec1...
```

**Security**: Keep `eggbot.env` readable only by the eggbot user. Never commit nsec to version control.

### Generate Bot Identity

```bash
# Generate keypair using nak
SK=$(nak key generate)
NSEC=$(nak encode nsec $SK)
NPUB=$(nak encode npub $(echo $SK | nak key public))

echo "npub: $NPUB"
echo "nsec: $NSEC"  # Store securely in eggbot.env
```

### Publish Bot Profile

```bash
nak event -k 0 --sec $SK -c '{
  "name": "eggbot",
  "about": "Egg sales bot. DM me: help",
  "lud16": "eggbot@getalby.com"
}' wss://relay.damus.io
```

## Installation

### Manual Installation

```bash
# Build
make build

# Install binary
sudo cp bin/eggbot /usr/local/bin/

# Create directories
sudo mkdir -p /etc/eggbot /var/lib/eggbot

# Create eggbot user
sudo useradd -r -s /sbin/nologin -d /var/lib/eggbot eggbot
sudo chown eggbot:eggbot /var/lib/eggbot

# Copy config files
sudo cp configs/dev.yaml /etc/eggbot/config.yaml
sudo touch /etc/eggbot/eggbot.env
sudo chmod 600 /etc/eggbot/eggbot.env
sudo chown eggbot:eggbot /etc/eggbot/eggbot.env
# Edit /etc/eggbot/eggbot.env and add EGGBOT_NSEC=nsec1...

# Install systemd service
sudo cp eggbot.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### NixOS

Add to your NixOS configuration:

```nix
# In flake.nix inputs
eggbot.url = "github:Buildtall-Systems/eggbot";

# In configuration.nix
systemd.services.eggbot = {
  description = "Eggbot Nostr egg sales bot";
  after = [ "network-online.target" ];
  wants = [ "network-online.target" ];
  wantedBy = [ "multi-user.target" ];

  serviceConfig = {
    Type = "simple";
    User = "eggbot";
    Group = "eggbot";
    ExecStart = "${pkgs.eggbot}/bin/eggbot run --config /etc/eggbot/config.yaml";
    EnvironmentFile = "/etc/eggbot/eggbot.env";
    WorkingDirectory = "/var/lib/eggbot";
    Restart = "on-failure";
    RestartSec = 5;
  };
};
```

## Running

### Development

```bash
# Set environment
export EGGBOT_NSEC=nsec1...

# Run with dev config
./bin/eggbot run --config configs/dev.yaml
```

### Production (systemd)

```bash
# Start service
sudo systemctl start eggbot

# Enable on boot
sudo systemctl enable eggbot

# Check status
sudo systemctl status eggbot

# View logs
journalctl -u eggbot -f
```

## Commands

### Customer Commands

Any registered customer can use:

| Command | Description |
|---------|-------------|
| `inventory` | Check egg availability |
| `order <qty>` | Order eggs (qty must be positive integer) |
| `balance` | Check payment balance (received - spent) |
| `history` | View last 5 orders |
| `help` | Show available commands |

### Admin Commands

Only npubs in the `admins` config list can use:

| Command | Description |
|---------|-------------|
| `add <qty>` | Add eggs to inventory |
| `deliver <npub>` | Fulfill customer's pending orders |
| `payment <npub> <sats>` | Record manual payment |
| `adjust <npub> <sats>` | Adjust customer balance (+/-) |
| `customers` | List all registered customers |
| `addcustomer <npub>` | Register new customer |
| `removecustomer <npub>` | Remove customer |

## Payment Flow

1. Customer sends `order 12` via encrypted DM
2. Bot creates pending order and responds with price
3. Customer zaps the bot's npub for the amount
4. Bot receives zap receipt, validates it, credits balance
5. If balance covers pending orders, orders are marked as paid
6. Admin uses `deliver <npub>` when physically delivering eggs

## Testing

```bash
# Run all tests with race detector
make test

# Run linter
make lint

# Run specific test
go test -v ./internal/commands/...
```

### Send Test DM

Using nak:

```bash
# Get bot's hex pubkey
BOT_HEX=$(nak decode npub1... | jq -r .data)

# Send encrypted DM (NIP-17)
echo "inventory" | nak event -k 14 --sec $YOUR_SK -p $BOT_HEX | \
  nak encrypt -k 14 --sec $YOUR_SK $BOT_HEX | \
  nak event -k 1059 --sec $(nak key generate) | \
  nak publish wss://relay.damus.io
```

Or use a Nostr client that supports NIP-17 DMs (Coracle, Amethyst, etc.).

## Troubleshooting

### Bot not receiving messages

1. Check relay connectivity:
   ```bash
   journalctl -u eggbot | grep "connected\|error"
   ```

2. Verify bot pubkey matches config:
   ```bash
   nak decode $BOT_NPUB
   ```

3. Ensure sender is registered as customer

### Zaps not credited

1. Verify `lightning.lnurl_pubkey` matches your LNURL provider
2. Check zap validation errors in logs:
   ```bash
   journalctl -u eggbot | grep -i zap
   ```

3. Ensure sender is a registered customer

### Database errors

1. Check file permissions:
   ```bash
   ls -la /var/lib/eggbot/
   ```

2. Verify database path is writable by eggbot user

### Service won't start

1. Check config syntax:
   ```bash
   eggbot run --config /etc/eggbot/config.yaml --verbose
   ```

2. Verify EGGBOT_NSEC is set:
   ```bash
   sudo -u eggbot cat /etc/eggbot/eggbot.env
   ```

## License

MIT

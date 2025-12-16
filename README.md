# Eggbot

A sales bot for small-scale egg producers who want to accept Bitcoin payments without payment processors, platform fees, or middlemen.

## Overview

Eggbot enables peer-to-peer commerce over Nostr, a decentralized social protocol. Customers interact with the bot through encrypted direct messages to check inventory, place orders, and view their payment history. When a customer places an order, they pay with Bitcoin over the Lightning Network. The bot tracks payments automatically and updates order status in real time.

The system combines three technologies:

- **Nostr** is a decentralized protocol for communication. Messages are cryptographically signed and delivered through relays (servers that forward messages). There's no central authority; anyone can run a relay, and users control their own identity through public/private key pairs.

- **Encrypted DMs** ensure that conversations between customers and the bot remain private. Even the relays that forward messages cannot read their contents.

- **Lightning zaps** are Bitcoin micropayments native to Nostr. When a customer "zaps" the bot, the payment creates a cryptographic receipt that the bot can verify and credit to their account.

Because everything runs on open protocols, there's no platform risk. The bot operator controls their own infrastructure, customers control their own identity, and payments flow directly between parties.

## Commands

Customers interact with Eggbot by sending text commands via direct message. Any Nostr client that supports encrypted DMs will work (Amethyst, Coracle, Damus, and others).

### Customer Commands

Registered customers can use these commands:

| Command | Description |
|---------|-------------|
| `help` | Show available commands |
| `inventory` | Check how many eggs are available |
| `order 6` or `order 12` | Order a half-dozen or dozen eggs |
| `balance` | Check your payment balance |
| `history` | View your last 25 orders |
| `cancel <order_id>` | Cancel a pending order |

### Admin Commands

Administrators have additional commands for managing inventory, customers, and orders. Admin status is granted by adding a user's public key to the bot's configuration.

**Inventory management:**

| Command | Description |
|---------|-------------|
| `inventory` | Show detailed breakdown: available, reserved, sold, on-hand |
| `inventory add <qty>` | Add eggs to inventory |
| `inventory set <qty>` | Set inventory to exact count |

**Order fulfillment:**

| Command | Description |
|---------|-------------|
| `orders` | List all orders across all customers |
| `deliver <order_id>` | Mark an order as delivered |

**Customer management:**

| Command | Description |
|---------|-------------|
| `customers` | List all registered customers |
| `addcustomer <npub>` | Register a new customer by their public key |
| `removecustomer <npub>` | Remove a customer |

**Payment adjustments:**

| Command | Description |
|---------|-------------|
| `sales` | Show total sales in satoshis |
| `payment <npub> <sats>` | Record a manual payment |
| `adjust <npub> <sats>` | Adjust a customer's balance (positive or negative) |

## Payment Flow

When a customer places an order, the bot initiates a payment and fulfillment cycle. Understanding this flow is essential for both customers and operators.

### The Order-to-Delivery Cycle

1. **Order placed**: Customer sends `order 12` via encrypted DM. The bot creates a pending order, reserves the eggs from inventory, and responds with an order summary, the price in satoshis, and payment instructions.

2. **Payment instructions**: The response includes two payment options:
   - A Lightning invoice (a one-time payment request that can be paid from any Lightning wallet)
   - The bot's public key for zap payments

3. **Customer pays**: The customer sends Bitcoin via Lightning. Zaps are the recommended method because they generate a cryptographic receipt (defined by the NIP-57 protocol) that the bot can automatically verify and credit.

4. **Balance credited**: When the bot receives a valid zap receipt, it credits the customer's account. The customer's balance represents the difference between what they've paid and what they've spent on orders.

5. **Order marked paid**: Once a customer's balance covers their pending orders, those orders are automatically marked as paid.

6. **Physical delivery**: The operator delivers the eggs and uses `deliver <order_id>` to mark the order complete. This moves the eggs from "sold" to "delivered" in inventory tracking.

### Why Zaps Over Direct Invoice Payment

Lightning invoices can be paid directly from any wallet, but these payments are invisible to the bot. They go to the operator's Lightning address without generating a Nostr receipt. The operator would need to manually credit the customer using the `payment` command.

Zaps, by contrast, create a signed receipt on Nostr that proves who paid, how much, and when. The bot subscribes to these receipts and automatically credits payments. This is the recommended workflow.

## Build

The project uses Nix for reproducible builds:

```bash
nix develop
make build
```

This produces the binary at `bin/eggbot`.

## Configuration

Eggbot requires two configuration files: a YAML config for settings and an environment file for secrets.

### Understanding Nostr Keys

Before configuring the bot, it helps to understand Nostr's key system:

- **npub** (public key): A user's public identity, shareable with anyone. Customers use this to send messages to the bot.
- **nsec** (secret key): The private key that proves ownership of an npub. Never share this. The bot needs its nsec to sign messages and decrypt DMs.

Both are bech32-encoded strings (npub1... and nsec1...) derived from the same underlying keypair.

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
  lnurl_pubkey: "npub1..."  # e.g., Alby's npub
  # Lightning address for invoice generation (optional)
  # If set, order confirmations include a clickable Lightning invoice
  address: "eggbot@getalby.com"

pricing:
  sats_per_half_dozen: 3200

# Admin public keys (can manage inventory, customers, orders)
admins:
  - "npub1..."
```

The `lightning.lnurl_pubkey` setting is a security measure. When set, the bot only accepts zap receipts signed by that specific Lightning provider (like Alby). This prevents spoofed zap receipts. Leave it empty to accept zaps from any provider, but understand this is less secure.

### Environment File

Create `/etc/eggbot/eggbot.env`:

```bash
# Bot's secret key (REQUIRED)
EGGBOT_NSEC=nsec1...
```

**Security**: This file contains the bot's private key. Keep it readable only by the eggbot user (`chmod 600`). Never commit it to version control.

### Generating a Bot Identity

Use `nak` (a Nostr command-line tool) to generate a new keypair:

```bash
SK=$(nak key generate)
NSEC=$(nak encode nsec $SK)
NPUB=$(nak encode npub $(echo $SK | nak key public))

echo "npub: $NPUB"
echo "nsec: $NSEC"  # Store securely in eggbot.env
```

### Publishing the Bot's Profile

Once you have keys, publish a profile so customers can find the bot:

```bash
nak event -k 0 --sec $SK -c '{
  "name": "eggbot",
  "about": "Egg sales bot. DM me: help",
  "lud16": "eggbot@getalby.com"
}' wss://relay.damus.io
```

The `lud16` field is the bot's Lightning address, enabling zap payments directly from Nostr clients.

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

1. Add eggbot to your flake inputs:

```nix
# flake.nix
{
  inputs = {
    eggbot.url = "git+ssh://git@github.com/Buildtall-Systems/eggbot.git";
    # ... other inputs
  };

  outputs = { nixpkgs, eggbot, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      specialArgs = { inherit eggbot; };
      modules = [ ./configuration.nix ];
    };
  };
}
```

2. Create a module with service options:

```nix
# eggbot-module.nix
{ config, lib, pkgs, eggbot, ... }:

let
  cfg = config.services.eggbot;
in
{
  options.services.eggbot = {
    enable = lib.mkEnableOption "eggbot Nostr egg sales bot";

    configFile = lib.mkOption {
      type = lib.types.str;
      default = "/etc/eggbot/config.yaml";
      description = "Path to eggbot config file";
    };

    environmentFile = lib.mkOption {
      type = lib.types.str;
      default = "/etc/eggbot/eggbot.env";
      description = "Environment file containing EGGBOT_NSEC";
    };

    dataDir = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/eggbot";
      description = "Directory for eggbot data (database)";
    };
  };

  config = lib.mkIf cfg.enable {
    users.users.eggbot = {
      isSystemUser = true;
      group = "eggbot";
      home = cfg.dataDir;
      createHome = true;
    };

    users.groups.eggbot = {};

    systemd.services.eggbot = {
      description = "Eggbot Nostr egg sales bot";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        User = "eggbot";
        Group = "eggbot";
        ExecStart = "${eggbot.packages.${pkgs.system}.default}/bin/eggbot run --config ${cfg.configFile}";
        EnvironmentFile = cfg.environmentFile;
        WorkingDirectory = cfg.dataDir;
        Restart = "on-failure";
        RestartSec = "10s";

        # Hardening
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ReadWritePaths = [ cfg.dataDir ];
        PrivateTmp = true;
      };
    };
  };
}
```

3. Enable in your configuration:

```nix
# configuration.nix
{
  services.eggbot.enable = true;
}
```

4. Create config files (must be done manually after first deploy):

```bash
sudo mkdir -p /etc/eggbot

# Create config file (see Configuration section)
sudo vim /etc/eggbot/config.yaml

# Create env file with nsec
sudo touch /etc/eggbot/eggbot.env
sudo chmod 600 /etc/eggbot/eggbot.env
echo "EGGBOT_NSEC=nsec1..." | sudo tee /etc/eggbot/eggbot.env

sudo chown eggbot:eggbot /etc/eggbot/config.yaml
```

5. Rebuild and start:

```bash
sudo nixos-rebuild switch
sudo systemctl status eggbot
```

## Running

### Development

```bash
export EGGBOT_NSEC=nsec1...
./bin/eggbot run --config configs/dev.yaml
```

### Production (systemd)

```bash
sudo systemctl start eggbot
sudo systemctl enable eggbot   # Start on boot
sudo systemctl status eggbot   # Check status
journalctl -u eggbot -f        # Follow logs
```

## Testing

```bash
make test    # Run all tests with race detector
make lint    # Run linter
go test -v ./internal/commands/...  # Run specific tests
```

### Sending a Test DM

Using `nak`:

```bash
# Get bot's hex pubkey from its npub
BOT_HEX=$(nak decode npub1... | jq -r .data)

# Send encrypted DM (NIP-17 gift wrap format)
echo "inventory" | nak event -k 14 --sec $YOUR_SK -p $BOT_HEX | \
  nak encrypt -k 14 --sec $YOUR_SK $BOT_HEX | \
  nak event -k 1059 --sec $(nak key generate) | \
  nak publish wss://relay.damus.io
```

Or use any Nostr client that supports NIP-17 encrypted DMs (Coracle, Amethyst, etc.).

## Troubleshooting

### Bot not receiving messages

1. Check relay connectivity:
   ```bash
   journalctl -u eggbot | grep "connected\|error"
   ```

2. Verify the bot's public key in config matches its actual identity:
   ```bash
   nak decode $BOT_NPUB
   ```

3. Ensure the sender is a registered customer.

### Zaps not credited

1. Verify `lightning.lnurl_pubkey` matches your Lightning provider's public key.

2. Check for zap validation errors:
   ```bash
   journalctl -u eggbot | grep -i zap
   ```

3. Ensure the sender is a registered customer (zaps from unregistered users are ignored).

### Database errors

1. Check file permissions:
   ```bash
   ls -la /var/lib/eggbot/
   ```

2. Verify the database path is writable by the eggbot user.

### Service won't start

1. Test config syntax by running manually:
   ```bash
   eggbot run --config /etc/eggbot/config.yaml --verbose
   ```

2. Verify the nsec is set:
   ```bash
   sudo -u eggbot cat /etc/eggbot/eggbot.env
   ```

## License

MIT

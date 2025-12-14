# Status: eggbot

Daily work log. Add entries under date headers (## YYYY-MM-DD) after each unit of work.

See `docs/operations/status-spec.md` for format specification.

## 2025-12-13

### Phase 1: Project Scaffolding
- Created project via `/go-cli-project` with Nostr support
- Customized config schema (Viper + env var binding for EGGBOT_NSEC)
- Created migration `001_initial.sql` with inventory, customers, orders, transactions tables
- Added db package with Open, Migrate, Close methods

### Phase 2: Relay Connection & Subscriptions
- Created `internal/nostr/relay.go` - RelayManager with connect/reconnect/close
- Implemented dual subscription: kind:1059 (DMs) and kind:9735 (zaps)
- Event deduplication by ID
- Graceful shutdown on context cancellation

### Phase 3: DM Decryption & Command Parsing
- Created `internal/dm/unwrap.go` - NIP-17 gift unwrap using go-nostr nip59
- Created `internal/commands/parse.go` - Command parser with customer/admin command types
- Created `internal/commands/permissions.go` - Access control (admin list, customer whitelist)

### Phase 4: Command Execution
- Created `internal/commands/customer_commands.go` - inventory, order, balance, history, help
- Created `internal/commands/admin_commands.go` - add, deliver, payment, adjust, customers, addcustomer, removecustomer
- Created `internal/commands/dispatch.go` - Command dispatcher
- Order pricing: sats_per_half_dozen per 6 eggs (rounded up)
- Constraint: one pending order per customer

### Phase 5: Zap Processing & Response Sending
- Created `internal/zaps/validate.go` - NIP-57 zap receipt validation
- BOLT11 invoice parsing with multiplier support (m/u/n/p)
- Created `internal/zaps/processor.go` - Process zaps, credit balance, auto-mark orders paid
- Created `internal/dm/wrap.go` - NIP-17 response wrapping via nip59.GiftWrap
- Integrated zap handling and DM response sending in run.go
- 44 tests passing with race detector, lint clean

### Phase 6: Systemd Service & Documentation
- Created `eggbot.service` - systemd unit with security hardening
- Created `README.md` - Installation, configuration, commands, troubleshooting
- All 44 tests passing, build successful

---
name: eggbot
description: This skill provides the command interface for interacting with eggbot, a Nostr-native egg sales bot operated by buildtall.systems. Use when a customer's agent needs to check inventory, place orders, manage payments, or set up notifications via Nostr DM.
---

# Eggbot

## Overview

Eggbot is a Nostr bot that sells farm eggs for bitcoin (Lightning). Interact with it by sending encrypted DMs containing text commands. It responds using the same DM protocol (NIP-04 or NIP-17) that was used to send the message.

## Bot Identity

**npub**: `npub1srcs6w4mmkjdkm6n4dh69smakmauv09vxtfrap73gr8ampwzcc8sdutrts`

## Relays

Connect to eggbot through these relays:

| Relay | Notes |
|-------|-------|
| `wss://relay.nostr.io` | **Preferred.** Requires NIP-42 authentication and whitelist approval. |
| `wss://relay.drss.io` | Requires NIP-42 authentication. |
| `wss://relay.damus.io` | Public. No auth required. May be rate-limited under load. |

If using `relay.nostr.io`, the connecting identity must be on the relay's whitelist and the client must support NIP-42 (automatic AUTH challenge signing).

## Commands

Send any command as the body of an encrypted DM to eggbot's npub. Commands are case-insensitive.

### Check Inventory

```
inventory
```

Returns the number of eggs currently available for purchase.

### Place an Order

```
order 6
order 12
```

Orders must be exactly 6 or 12 eggs. Placing an order reserves the eggs immediately. Only one pending (unpaid) order is allowed at a time. Response includes payment instructions.

**Pricing**: Set by the operator. Current rate is per half-dozen (6 eggs).

### Cancel an Order

```
cancel <order_id>
```

Cancels a pending order and releases the reserved eggs back to inventory. Only works while the order status is "pending" (unpaid).

### Check Balance

```
balance
```

Returns sats received, sats spent, and current balance.

### Order History

```
history
```

Returns the 25 most recent orders with ID, quantity, price, and status.

### Inventory Notifications

```
notify 6
notify 12
```

Subscribe to a one-time DM alert when available inventory reaches the specified threshold (6 or 12). The notification fires once and then the subscription is removed.

```
notify
```

Check current notification subscription status.

```
notify off
```

Cancel an active notification subscription.

### Help

```
help
```

Returns the list of available commands.

## Payment

After placing an order, pay using one of two methods:

1. **Lightning invoice** — The order response may include a Lightning invoice. Pay it from any Lightning wallet. The operator may need to manually confirm receipt.

2. **Nostr zap** (recommended) — Zap the bot's profile for the order amount. Eggbot automatically detects the zap receipt, credits the balance, and marks the order as paid.

## Order Lifecycle

```
PENDING ──(pay)──> PAID ──(operator delivers)──> FULFILLED
   │
   └──(cancel)──> CANCELLED
```

- **pending**: Eggs reserved, awaiting payment
- **paid**: Payment confirmed, awaiting physical delivery
- **fulfilled**: Delivered by operator
- **cancelled**: Cancelled before payment, eggs released

## Typical Flow

1. `inventory` — check availability
2. `order 12` — place order (eggs reserved)
3. Zap the bot's profile for the stated amount
4. Bot confirms payment automatically
5. Operator delivers eggs and marks order fulfilled
6. `history` — verify order status

## Error Messages

| Situation | Response |
|-----------|----------|
| Not a registered customer | "you are not a registered customer" |
| Unpaid order exists | "you have 1 unpaid order(s) - please pay or cancel before ordering more" |
| Invalid quantity | "quantity must be 6 or 12" |
| Insufficient inventory | "only N eggs available, cannot order M" |
| Cancel non-pending order | "order N cannot be cancelled (status: paid)" |
| Unknown command | "Unknown command: X. Send 'help' for available commands." |

## Prerequisites

To use eggbot, a customer's npub must be registered by an operator. Unregistered npubs receive a rejection message. Contact the operator to request registration.

package commands

import (
	"context"

	"github.com/buildtall-systems/eggbot/internal/db"
)

// ExecuteConfig holds configuration needed for command execution.
type ExecuteConfig struct {
	SatsPerHalfDozen int
	Admins           []string
	LightningAddress string
}

// Execute runs the command and returns a result.
// senderNpub is the sender's public key in npub format.
func Execute(ctx context.Context, database *db.DB, cmd *Command, senderNpub string, cfg ExecuteConfig) Result {
	isAdmin := IsAdmin(senderNpub, cfg.Admins)

	switch cmd.Name {
	// Customer commands
	case CmdInventory:
		return InventoryCmd(ctx, database)

	case CmdOrder:
		return OrderCmd(ctx, database, senderNpub, cmd.Args, cfg.SatsPerHalfDozen, cfg.LightningAddress)

	case CmdBalance:
		return BalanceCmd(ctx, database, senderNpub)

	case CmdHistory:
		return HistoryCmd(ctx, database, senderNpub)

	case CmdHelp:
		return HelpCmd(isAdmin)

	// Admin commands
	case CmdAdd:
		return AddEggsCmd(ctx, database, cmd.Args)

	case CmdDeliver:
		return DeliverCmd(ctx, database, cmd.Args)

	case CmdPayment:
		return PaymentCmd(ctx, database, cmd.Args)

	case CmdAdjust:
		return AdjustCmd(ctx, database, cmd.Args)

	case CmdCustomers:
		return CustomersCmd(ctx, database)

	case CmdAddCustomer:
		return AddCustomerCmd(ctx, database, cmd.Args)

	case CmdRemoveCustomer:
		return RemoveCustomerCmd(ctx, database, cmd.Args)

	default:
		return HelpCmd(isAdmin)
	}
}

package commands

import (
	"context"

	"github.com/Buildtall-Systems/btk/lightning"

	"github.com/buildtall-systems/eggbot/internal/db"
)

type ExecuteConfig struct {
	LightningBackend lightning.Backend
	BotNpub          string
	Admins           []string
	SatsPerHalfDozen int
}

// Execute runs the command and returns a result.
// senderNpub is the sender's public key in npub format.
func Execute(ctx context.Context, database *db.DB, cmd *Command, senderNpub string, cfg ExecuteConfig) Result {
	isAdmin := IsAdmin(senderNpub, cfg.Admins)

	switch cmd.Name {
	// Customer commands (with admin subcommands)
	case CmdInventory:
		return InventoryCmd(ctx, database, cmd.Args, isAdmin)

	case CmdOrder:
		return OrderCmd(ctx, database, senderNpub, cmd.Args, cfg.SatsPerHalfDozen, cfg.BotNpub, cfg.LightningBackend)

	case CmdCancel:
		return CancelOrderCmd(ctx, database, senderNpub, cmd.Args)

	case CmdBalance:
		return BalanceCmd(ctx, database, senderNpub)

	case CmdHistory:
		return HistoryCmd(ctx, database, senderNpub)

	case CmdHelp:
		return HelpCmd(isAdmin)

	case CmdNotify:
		return NotifyCmd(ctx, database, senderNpub, cmd.Args)

	// Admin commands
	case CmdDeliver:
		return DeliverCmd(ctx, database, cmd.Args)

	case CmdMarkpaid:
		return MarkpaidCmd(ctx, database, cmd.Args)

	case CmdAdjust:
		return AdjustCmd(ctx, database, cmd.Args)

	case CmdOrders:
		return OrdersCmd(ctx, database)

	case CmdCustomers:
		return CustomersCmd(ctx, database)

	case CmdAddCustomer:
		return AddCustomerCmd(ctx, database, cmd.Args)

	case CmdRemoveCustomer:
		return RemoveCustomerCmd(ctx, database, cmd.Args)

	case CmdSales:
		return SalesCmd(ctx, database)

	case CmdSell:
		return SellCmd(ctx, database, cmd.Args, cfg.SatsPerHalfDozen)

	default:
		return HelpCmd(isAdmin)
	}
}

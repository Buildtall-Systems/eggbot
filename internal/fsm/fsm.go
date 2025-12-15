package fsm

const (
	OrderStatePending   = "pending"
	OrderStatePaid      = "paid"
	OrderStateFulfilled = "fulfilled"
	OrderStateCancelled = "cancelled"
)

const (
	OrderEventPay     = "pay"
	OrderEventCancel  = "cancel"
	OrderEventFulfill = "fulfill"
)

const (
	InventoryStateAvailable = "available"
	InventoryStateReserved  = "reserved"
	InventoryStateConsumed  = "consumed"
)

const (
	InventoryEventReserve = "reserve"
	InventoryEventConsume = "consume"
	InventoryEventRestore = "restore"
)

const (
	ProcessorStateIdle            = "idle"
	ProcessorStateProcessingDM    = "processing_dm"
	ProcessorStateProcessingZap   = "processing_zap"
	ProcessorStateSendingResponse = "sending_response"
)

const (
	ProcessorEventDMReceived       = "dm_received"
	ProcessorEventZapReceived      = "zap_received"
	ProcessorEventCommandProcessed = "command_processed"
	ProcessorEventResponseSent     = "response_sent"
	ProcessorEventError            = "error"
)

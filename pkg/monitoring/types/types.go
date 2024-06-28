package types

// types.go contains simple types, more complex types should have a separate file
const (
	SlotHeightType   = "slot_height"
	SlotHeightMetric = "sol_" + SlotHeightType

	NetworkFeesType   = "network_fees"
	NetworkFeesMetric = "sol_" + NetworkFeesType
)

// SlotHeight type wraps the uint64 type returned by the RPC call
// this helps to delineate types when sending to the exporter
type SlotHeight uint64

package stubs

type Stub string

var SendWorldData Stub = "GOLWorker.ReceiveWorldData"
var CalculateNTurns Stub = "GOLWorker.CalculateForTurns"
var SendCellCount Stub = "GOLWorker.SendCellCount"

type Response struct {
	Message string
}

type Request struct {
	Message string
}

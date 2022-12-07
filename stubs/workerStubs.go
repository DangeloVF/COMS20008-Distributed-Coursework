package stubs

type Stub string

var SendWorldData Stub = "GOLWorker.ReceiveWorldData"
var CalculateNTurns Stub = "GOLWorker.CalculateForTurns"
var SendCellCount Stub = "GOLWorker.SendCellCount"
var SendTurnCount Stub = "GOLWorker.SendTurnCount"

var PauseCalculations Stub = "GOLWorker.PauseCalculations"
var UnPauseCalculations Stub = "GOLWorker.UnPauseCalculations"

var StopCalculations Stub = "GOLWorker.StopCalculations"
var SendCurrentState Stub = "GOLWorker.SendCurrent"

type Response struct {
	Message string
}

type Request struct {
	Message string
}

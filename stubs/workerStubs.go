package stubs

type Stub string

var SendWorldData Stub = "GOLWorker.ReceiveWorldData"
var CalculateNTurns Stub = "GOLWorker.CalculateForTurns"
var SendAliveCells Stub = "GOLWorker.SendAliveCells"
var CountCells Stub = "GOLWorker.CountCells"
var GetTurn Stub = "GOLWorker.GetTurn"

type Response struct {
	Message string
}

type Request struct {
	Message string
}

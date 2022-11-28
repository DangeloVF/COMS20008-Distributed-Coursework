package stubs

type Stub string

var SendWorldData Stub = "GOLWorker.ReceiveWorldData"
var CalculateNTurns Stub = "GOLWorker.CalculateForTurns"

type Response struct {
	Message string
}

type Request struct {
	Message string
}

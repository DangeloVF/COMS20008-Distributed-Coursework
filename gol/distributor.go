package gol

import (
	"fmt"
	"net/rpc"
	"strings"
	"time"

	"uk.ac.bris.cs/gameoflife/golUtils"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

const serverIP string = "127.0.0.1"
const serverPort string = "8030"

func makeCall(client *rpc.Client, message string, callType stubs.Stub) string {
	request := stubs.Request{Message: message}
	response := new(stubs.Response)
	calltype := string(callType)
	client.Call(calltype, request, response)
	fmt.Println("Responded: " + response.Message)
	return response.Message
}

func makeAsyncCall(client *rpc.Client, message string, callType stubs.Stub) (done *rpc.Call, response *stubs.Response) {
	request := stubs.Request{Message: message}
	response = new(stubs.Response)
	calltype := string(callType)
	done = client.Go(calltype, request, response, nil)
	return
}

func worldToString(p Params, w golUtils.World) string {
	param := fmt.Sprintf("%d,%d,%d,%d", p.ImageHeight, p.ImageWidth, p.Threads, p.Turns)
	fmt.Println("sending params:" + param)
	var world string
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world = world + fmt.Sprintf("%d,", w[x][y])
		}
	}
	out := param + ";" + world
	return out
}

// Height,Length;cell0,cell1,...

func parseOutput(p Params, s string) (w golUtils.World, t int, err error) {
	err = nil
	sSplit := strings.Split(s, ";")

	if _, err := fmt.Sscan(sSplit[0], &t); err != nil {
		return nil, 0, err
	}

	w = golUtils.MakeWorld(p.ImageHeight, p.ImageWidth)
	wSplit := strings.Split(sSplit[1], ",")
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			fmt.Sscan(wSplit[x+y*p.ImageHeight], &w[x][y])
		}
	}
	return
}

func tiktok(s int, finish chan bool, tick chan bool) {
	ticker := time.NewTicker(time.Duration(s) * time.Second)
	for {
		select {
		case <-finish:
			return
		case <-ticker.C:
			tick <- true
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// Create a 2D slice to store the world.
	worldSlice := golUtils.MakeWorld(p.ImageHeight, p.ImageWidth)

	// Check IO is idle before attempting to read
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Tell IO to read file then put that read into the slice
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			worldSlice[x][y] = <-c.ioInput
		}
	}

	// Connect to server
	server := fmt.Sprintf("%s:%s", serverIP, serverPort)
	client, _ := rpc.Dial("tcp", server)

	worldString := worldToString(p, worldSlice)

	// Send world and parameters to server
	makeCall(client, worldString, stubs.SendWorldData)

	tickerEnd := make(chan bool)
	tickerNotify := make(chan bool)
	go tiktok(2, tickerEnd, tickerNotify)

	elapsedTurns := 0
	aliveCells := 0
	// Tell server to calculate
	workerFin, workerResponse := makeAsyncCall(client, fmt.Sprint(p.Turns), stubs.CalculateNTurns)

	golFinish := false
	for !golFinish {
		select {
		case <-tickerNotify:
			receivedCellCount := makeCall(client, "", stubs.SendCellCount)
			parseData := strings.Split(receivedCellCount, ",")
			fmt.Sscan(parseData[0], &elapsedTurns)
			fmt.Sscan(parseData[1], &aliveCells)
			c.events <- AliveCellsCount{elapsedTurns, aliveCells}
		case <-workerFin.Done:
			golFinish = true
		}
	}

	// parse the final calculated state\
	worldSlice, turn, _ := parseOutput(p, workerResponse.Message)

	//close server connection
	client.Close()

	// Report the final state using FinalTurnComplete event.

	// Turn worldSlice into slice of util.cells
	cellSlice := make([]util.Cell, 0)
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if worldSlice[x][y] == golUtils.LiveCell {
				cellSlice = append(cellSlice, util.Cell{X: x, Y: y})
			}
		}
	}

	// Send FinalTurnComplete event to channel
	c.events <- FinalTurnComplete{turn, cellSlice}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

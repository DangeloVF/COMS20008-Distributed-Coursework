package gol

import (
	"fmt"
	"net/rpc"
	"strconv"
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
	// fmt.Println("Responded: " + response.Message)
	return response.Message
}

func makeAsyncCall(client *rpc.Client, message string, response *stubs.Response, callType stubs.Stub) chan *rpc.Call {
	request := stubs.Request{Message: message}
	calltype := string(callType)
	call := client.Go(calltype, request, response, nil)
	// fmt.Println("Responded: " + response.Message)
	return call.Done
}

func worldToString(p Params, w golUtils.World) string {
	param := fmt.Sprintf("%d,%d", p.ImageHeight, p.ImageWidth)
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

func tick(finish chan bool, tick chan bool) {
	ticker := time.NewTicker(2 * time.Second)
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

	// Tell server to calculate
	response := new(stubs.Response)
	done := makeAsyncCall(client, fmt.Sprint(p.Turns), response, stubs.CalculateNTurns)

	// golFinish := false
	// for !golFinish {
	// 	select {
	// 	case <-tickerNotify:

	// 		c.events <- AliveCellsCount{turn, countCells(worldSlice, p)}
	// 	case <- workerfin:

	// 		golFinish=true
	// 	}
	// }

	tickerEnd := make(chan bool)
	tickerNotify := make(chan bool)
	go tick(tickerEnd, tickerNotify)

	golFinish := false

	for !golFinish {
		select {
		case <-done:
			golFinish = true
		case <-tickerNotify:
			fmt.Println("Tick")
			turn, _ := strconv.Atoi(makeCall(client, "", stubs.GetTurn))
			aliveCells, _ := strconv.Atoi(makeCall(client, "", stubs.CountCells))
			c.events <- AliveCellsCount{turn, aliveCells}
		}
	}

	// parse the final calculated state
	worldSlice, turn, _ := parseOutput(p, response.Message)

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

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
	keyPresses <-chan rune
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
	fmt.Println("Response from call " + callType)
	return response.Message
}

func sendInitialState(client *rpc.Client, intialState golUtils.GolState) (response *stubs.Response) {
	response = new(stubs.Response)

	client.Call(string(stubs.SendWorldData), intialState, response)
	fmt.Println("Received")

	return
}

func makeAsyncCall(client *rpc.Client, message string, callType stubs.Stub) (done *rpc.Call, response *stubs.Response) {
	request := stubs.Request{Message: message}
	response = new(stubs.Response)
	calltype := string(callType)
	done = client.Go(calltype, request, response, nil)
	fmt.Println("Response from call " + callType)

	return
}

func (c *distributorChannels) generatePGMFile(w golUtils.World, p Params, t int) {
	// Tell IO channel to output
	c.ioCommand <- ioOutput

	// Generate file name
	c.ioFilename <- fmt.Sprintf("%dx%dx%d",
		p.ImageWidth,
		p.ImageHeight,
		t)

	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			c.ioOutput <- w[y][x]
		}
	}

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

	// Send world and parameters to server
	sendInitialState(client, golUtils.GolState{golUtils.Params(p), worldSlice, 0})

	tickerEnd := make(chan bool)
	tickerNotify := make(chan bool)
	go tiktok(2, tickerEnd, tickerNotify)

	elapsedTurns := 0
	aliveCells := 0
	// Tell server to calculate
	workerFin, _ := makeAsyncCall(client, fmt.Sprint(p.Turns), stubs.CalculateNTurns)

	golFinish := false
	isPaused := false
	output := false
	for !golFinish {
		select {
		case <-tickerNotify:
			receivedCellCount := makeCall(client, "", stubs.SendCellCount)
			parseData := strings.Split(receivedCellCount, ",")
			fmt.Sscan(parseData[0], &elapsedTurns)
			fmt.Sscan(parseData[1], &aliveCells)
			c.events <- AliveCellsCount{elapsedTurns, aliveCells}
		case keyPress := <-c.keyPresses:
			switch keyPress {
			case 'p':

				if isPaused {
					makeCall(client, "", stubs.UnPauseCalculations)
				} else {
					makeCall(client, "", stubs.PauseCalculations)
				}
				isPaused = !isPaused
				continue
			case 's':
				currentState := makeCall(client, "", stubs.SendCurrentState)
				currentWorld, turn, _ := parseOutput(p, currentState)
				c.generatePGMFile(currentWorld, p, turn)
				continue
			case 'q':
				makeCall(client, "", stubs.StopCalculations)
				golFinish = true
			case 'k':
				makeCall(client, "", stubs.StopCalculations)
				golFinish = true
				output = true
			}
		case <-workerFin.Done:
			golFinish = true
			output = true
		}
	}

	finishedState := makeCall(client, "", stubs.SendCurrentState)
	// parse the final calculated state\
	worldSlice, turn, _ := parseOutput(p, finishedState)

	//close server connection
	client.Close()

	// Report the final state using FinalTurnComplete event.
	if output {
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
		c.generatePGMFile(worldSlice, p, p.Turns)
	}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

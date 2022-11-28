package gol

import (
	"fmt"
	"net/rpc"
	"strings"

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
	response := makeCall(client, fmt.Sprint(p.Turns), stubs.CalculateNTurns)

	// parse the final calculated state
	worldSlice, turn, _ := parseOutput(p, response)

	//close server connection
	client.Close()

	// Report the final state using FinalTurnComplete event.

	// Turn worldSlice into slice of util.cells
	cellSlice := make([]util.Cell, 0)
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if worldSlice[x][y] == live {
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

// Did D'Angelo get lazy and just copy-paste his code? yes.
// Allow it tho because there's more shit to do

const live byte = 255
const dead byte = 0

type cell struct {
	x, y int
}

func calculateAliveNeighbours(p Params, world [][]byte, x int, y int) int {
	var aliveNeighbours int
	xValues := [3]int{x - 1, x, x + 1}
	yValues := [3]int{y - 1, y, y + 1}

	if xValues[0] < 0 {
		xValues[0] = p.ImageWidth + xValues[0]
	}
	if xValues[2] >= p.ImageWidth {
		xValues[2] = p.ImageWidth - xValues[2]
	}

	if yValues[0] < 0 {
		yValues[0] = p.ImageHeight + yValues[0]
	}
	if yValues[2] >= p.ImageHeight {
		yValues[2] = p.ImageHeight - yValues[2]
	}

	for _, checkX := range xValues {
		for _, checkY := range yValues {
			if world[checkX][checkY] == live && !(checkX == x && checkY == y) {
				aliveNeighbours++
			}
		}
	}

	return aliveNeighbours
}

func calculateNextState(p Params, world [][]byte) [][]byte {
	newWorld := make([][]byte, p.ImageWidth)
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageHeight)
	}
	for x, first := range world {
		for y, v := range first {

			livingNeighbours := calculateAliveNeighbours(p, world, x, y)

			if livingNeighbours == 3 || (livingNeighbours == 2 && v == live) {
				newWorld[x][y] = live
			} else {
				newWorld[x][y] = dead
			}
		}
	}
	return newWorld
}

func calculateAliveCells(p Params, world [][]byte) []cell {
	var livingCells []cell
	for y, first := range world {
		for x, v := range first {
			if v == live {
				livingCells = append(livingCells, cell{x: x, y: y})
			}
		}
	}

	return livingCells
}

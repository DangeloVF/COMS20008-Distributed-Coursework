package gol

import (
	"fmt"

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

type world [][]byte

const liveCell byte = 255
const deadCell byte = 0

func makeWorld(height, width int) world {
	newWorld := make(world, height)
	for i := range newWorld {
		newWorld[i] = make([]uint8, width)
	}
	return newWorld
}

func makeImmutableWorld(w world) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return w[y][x]
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	//  Create a 2D slice to store the world.
	worldSlice := makeWorld(p.ImageHeight, p.ImageWidth)

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

	turn := 0

	// TODO: Execute all turns of the Game of Life.
	// Begin Server
	// Send world and parameters to worker(s)

	// Report the final state using FinalTurnCompleteEvent.
	// Turn worldSlice into slice of util.cells
	cellSlice := make([]util.Cell, 0)
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if worldSlice[x][y] == liveCell {
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

package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/golUtils"
	"uk.ac.bris.cs/gameoflife/stubs"
)

func parseWorldString(s string) (world golUtils.World, params golUtils.Params, err error) {
	sSplit := strings.Split(s, ";")
	pSplit := strings.Split(sSplit[0], ",")

	// TODO: optimise this crap
	// parse params section of received message
	// imgHeight
	if _, err = fmt.Sscan(pSplit[0], &params.ImageHeight); err != nil {
		return
	}
	// imgWidth
	if _, err = fmt.Sscan(pSplit[1], &params.ImageWidth); err != nil {
		return
	}
	// # of threads
	if _, err = fmt.Sscan(pSplit[2], &params.Threads); err != nil {
		return
	}
	// # of turns
	if _, err = fmt.Sscan(pSplit[3], &params.Turns); err != nil {
		return
	}

	// parse world section of received message
	wSplit := strings.Split(sSplit[1], ",")
	world = golUtils.MakeWorld(params.ImageHeight, params.ImageWidth)
	for y := 0; y < params.ImageHeight; y++ {
		for x := 0; x < params.ImageWidth; x++ {
			fmt.Sscan(wSplit[x+y*params.ImageHeight], &world[x][y])
		}
	}

	return
}

func worldToString(w golUtils.World, p golUtils.Params, turns int) string {
	s := fmt.Sprintf("%d;", turns)

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			s = s + fmt.Sprintf("%d,", w[x][y])
		}
	}

	return s
}

func calculateAliveNeighbours(p golUtils.Params, w golUtils.World, x int, y int) int {
	var aliveNeighbours int
	xValues := [3]int{x - 1, x, x + 1}
	yValues := [3]int{y - 1, y, y + 1}

	if xValues[0] == -1 {
		xValues[0] = p.ImageWidth - 1
	}
	if xValues[2] == p.ImageWidth {
		xValues[2] = 0
	}

	if yValues[0] == -1 {
		yValues[0] = p.ImageHeight - 1
	}
	if yValues[2] == p.ImageHeight {
		yValues[2] = 0
	}

	for _, checkX := range xValues {
		for _, checkY := range yValues {
			if w[checkX][checkY] == golUtils.LiveCell && !(checkX == x && checkY == y) {
				aliveNeighbours++
			}
		}
	}

	return aliveNeighbours
}

func calculateNextSectionState(p golUtils.Params, w golUtils.World, startCoords golUtils.CoOrds, endCoords golUtils.CoOrds) [][]byte {
	newWorldSlice := make([][]byte, endCoords.X-startCoords.X)
	for i := range newWorldSlice {
		newWorldSlice[i] = make([]byte, endCoords.Y-startCoords.Y)
	}
	for x := startCoords.X; x < endCoords.X; x++ {
		for y := startCoords.Y; y < endCoords.Y; y++ {

			livingNeighbours := calculateAliveNeighbours(p, w, x, y)

			if livingNeighbours == 3 || (livingNeighbours == 2 && w[x][y] == golUtils.LiveCell) {
				newWorldSlice[x-startCoords.X][y-startCoords.Y] = golUtils.LiveCell
			} else {
				newWorldSlice[x-startCoords.X][y-startCoords.Y] = golUtils.DeadCell
			}
		}
	}
	return newWorldSlice
}

func countCells(w golUtils.World, p golUtils.Params) int {
	liveCount := 0
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if w[x][y] == golUtils.LiveCell {
				liveCount++
			}
		}
	}
	return liveCount
}

// All the API functions that are visible
type GOLWorker struct {
	// Mutexes and semaphores
	isCalculating      bool
	stopCalculating    bool
	pauseCalculatingSP bool
	pauseMutex         sync.Mutex
	pauseCalculatingCV sync.Cond
	accessData         sync.Mutex

	// Critical data
	params      golUtils.Params
	world       golUtils.World
	currentTurn int
}

func (g *GOLWorker) PauseCalculations(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}
	if g.pauseCalculatingSP {
		err = errors.New("calculations already paused!")

	}

	fmt.Println("Pausing calculations!")
	// set pause semaphore
	g.pauseCalculatingSP = true

	return
}

func (g *GOLWorker) UnPauseCalculations(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}
	if !g.pauseCalculatingSP {
		err = errors.New("calculations aren't paused!")

	}
	fmt.Println("Unpausing calculations!")
	//unset pause semaphore and broadcast to waiting threads
	g.pauseCalculatingSP = false
	g.pauseCalculatingCV.Broadcast()
	return
}

func (g *GOLWorker) StopCalculations(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}

	fmt.Println("Stopping calculations!")

	// set stop calculating semaphore
	g.stopCalculating = true

	return
}

func (g *GOLWorker) SendCellCount(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}

	fmt.Println("Recieved request for cell count!")

	// try to copy worker state into local
	g.accessData.Lock()
	params := g.params
	turn := g.currentTurn
	currentWorld := golUtils.MakeWorld(params.ImageHeight, params.ImageWidth)
	copy(currentWorld, g.world)
	g.accessData.Unlock()

	NUMCELLS := countCells(currentWorld, params)

	fmt.Printf("a TURN %d, CELLS %d\n", turn, NUMCELLS)

	res.Message = fmt.Sprintf("%d,%d", turn, NUMCELLS)
	return
}

func (g *GOLWorker) SendTurnCount(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}

	fmt.Println("Recieved request for turn count!")

	// try to copy worker state into local
	g.accessData.Lock()
	turn := g.currentTurn
	g.accessData.Unlock()

	res.Message = fmt.Sprintf("%d", turn)
	return
}

func (g *GOLWorker) SendCurrent(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message != "" {
		err = errors.New("not expecting any data, was this called by accident?")
		return
	}

	fmt.Println("Recieved request for current state!")

	// try to copy worker state into local
	g.accessData.Lock()
	params := g.params
	turn := g.currentTurn
	currentWorld := golUtils.MakeWorld(params.ImageHeight, params.ImageWidth)
	copy(currentWorld, g.world)
	g.accessData.Unlock()

	res.Message = worldToString(currentWorld, params, turn)
	return
}

func (g *GOLWorker) CalculateForTurns(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message == "" {
		err = errors.New("no data recieved")
		return
	}

	fmt.Println("received request to calculate!")
	var turnsToCalculate int

	_, err = fmt.Sscan(req.Message, &turnsToCalculate)
	if err != nil {
		res.Message = "error"
		return
	}

	// Check calculations haven't already started
	if g.isCalculating {
		err = errors.New("worker is currently doing a calculation")
		return
	}

	g.isCalculating = true
	fmt.Println("beginning calculations!")

	// try to copy worker state into local
	currentWorld := golUtils.MakeWorld(g.params.ImageHeight, g.params.ImageWidth)
	g.accessData.Lock()
	copy(currentWorld, g.world)
	params := g.params
	turn := g.currentTurn
	g.accessData.Unlock()

	fmt.Printf("going to calculate, turn = %d, going to calculate %d turns \n", turn, turnsToCalculate)
	for (turn < params.Turns) && !g.stopCalculating {

		g.pauseCalculatingCV.L.Lock()
		for g.pauseCalculatingSP {
			g.pauseCalculatingCV.Wait()
		}
		g.pauseCalculatingCV.L.Unlock()
		newWorld := calculateNextSectionState(params, currentWorld, golUtils.CoOrds{X: 0, Y: 0}, golUtils.CoOrds{X: params.ImageWidth, Y: params.ImageHeight})
		turn++

		// push local into workerState
		g.accessData.Lock()
		copy(g.world, newWorld)
		g.currentTurn = turn
		g.accessData.Unlock()
		copy(currentWorld, newWorld)
	}

	// reset stuff
	g.isCalculating = false
	g.stopCalculating = false
	return
}

func (g *GOLWorker) ReceiveWorldData(req golUtils.GolState, res *stubs.Response) (err error) {

	fmt.Println("Received world data!")

	// Check calculations haven't already started
	if g.isCalculating {
		err = errors.New("worker is currently doing a calculation")
		return
	}

	// put world and params into GOLWorker and reset current turns
	g.accessData.Lock()
	g.world = req.World
	g.params = req.Params
	g.currentTurn = req.CurrentTurn
	g.accessData.Unlock()
	res.Message = "received"
	return
}

const port string = "8030"
const ip string = "127.0.0.1"

func main() {
	pAddr := port
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GOLWorker{isCalculating: false, currentTurn: 0, pauseCalculatingCV: *sync.NewCond(&sync.Mutex{})})
	listener, _ := net.Listen("tcp", ":"+pAddr)
	fmt.Println(listener.Addr())
	defer listener.Close()
	rpc.Accept(listener)
}

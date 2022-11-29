package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"strings"
	"time"

	"uk.ac.bris.cs/gameoflife/golUtils"
	"uk.ac.bris.cs/gameoflife/stubs"
)

func parseWorldString(s string) (golUtils.World, golUtils.Params, error) {
	sSplit := strings.Split(s, ";")
	pSplit := strings.Split(sSplit[0], ",")
	p := golUtils.Params{}
	if _, err := fmt.Sscan(pSplit[0], &p.ImageHeight); err != nil {
		return nil, golUtils.Params{Turns: 0, Threads: 0, ImageWidth: 0, ImageHeight: 0}, err
	}
	if _, err := fmt.Sscan(pSplit[1], &p.ImageWidth); err != nil {
		return nil, golUtils.Params{Turns: 0, Threads: 0, ImageWidth: 0, ImageHeight: 0}, err
	}

	wSplit := strings.Split(sSplit[1], ",")
	w := golUtils.MakeWorld(p.ImageHeight, p.ImageWidth)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			fmt.Sscan(wSplit[x+y*p.ImageHeight], &w[x][y])
		}
	}
	return w, p, nil
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

// func countCells(w world, p Params) int {
// 	liveCount := 0
// 	for y := 0; y < p.ImageHeight; y++ {
// 		for x := 0; x < p.ImageWidth; x++ {
// 			if w[x][y] == liveCell {
// 				liveCount++
// 			}
// 		}
// 	}
// 	return liveCount
// }

// All the API functions that are visible
type GOLWorker struct {
	p golUtils.Params
	w golUtils.World
}

// func (g *GOLWorker) SendAliveCells(req stubs.Request, res *stubs.Response) (err error) {
// }

func (g *GOLWorker) CalculateForTurns(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message == "" {
		err = errors.New("no data recieved")
		return
	}

	fmt.Println("Got Message: " + req.Message)

	var turns int
	_, err = fmt.Sscan(req.Message, &turns)
	if err != nil {
		res.Message = "error"
		return
	}
	fmt.Printf("Calculating for %d turns", turns)

	turn := 0
	for i := 0; i < turns; i++ {
		g.w = calculateNextSectionState(g.p, g.w, golUtils.CoOrds{X: 0, Y: 0}, golUtils.CoOrds{X: g.p.ImageWidth, Y: g.p.ImageHeight})
		turn++
	}

	res.Message = worldToString(g.w, g.p, turn)
	return
}

func (g *GOLWorker) ReceiveWorldData(req stubs.Request, res *stubs.Response) (err error) {
	if req.Message == "" {
		err = errors.New("no data recieved")
		return
	}
	fmt.Println("Got Message: " + req.Message)

	g.w, g.p, err = parseWorldString(req.Message)
	if err != nil {
		res.Message = "error"
		return
	}
	res.Message = "received"
	return
}

const port string = "8030"
const ip string = "127.0.0.1"

func main() {
	pAddr := port
	iAddr := ip
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GOLWorker{})
	listener, _ := net.Listen("tcp", iAddr+":"+pAddr)
	fmt.Println(listener.Addr())
	defer listener.Close()
	rpc.Accept(listener)
}

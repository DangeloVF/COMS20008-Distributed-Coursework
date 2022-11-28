package golUtils

const LiveCell byte = 255
const DeadCell byte = 0

type World [][]byte

type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type CoOrds struct {
	X int
	Y int
}

func MakeWorld(height, width int) World {
	newWorld := make(World, height)
	for i := range newWorld {
		newWorld[i] = make([]uint8, width)
	}
	return newWorld
}

func MakeImmutableWorld(w World) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return w[y][x]
	}
}

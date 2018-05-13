package mouse

import (
	"github.com/ivan1993spb/snake-server/engine"
	"github.com/ivan1993spb/snake-server/game"
)

type Mouse struct {
	world     game.World
	location  engine.Location
	direction engine.Direction
}

func NewMouse(world game.World) *Mouse {
	mouse := &Mouse{}
	location, err := world.CreateObjectRandomDot(mouse)
	if err != nil {
		// TODO: return error
		return nil
	}

	mouse.world = world
	mouse.location = location
	mouse.direction = engine.RandomDirection()

	return mouse
}
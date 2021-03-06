package game

import (
	"github.com/sirupsen/logrus"

	"github.com/ivan1993spb/snake-server/world"
)

type ObserverInterface interface {
	Observe(stop <-chan struct{}, world *world.World, logger logrus.FieldLogger)
}

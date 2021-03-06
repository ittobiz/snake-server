package observers

import (
	"github.com/sirupsen/logrus"

	"github.com/ivan1993spb/snake-server/world"
)

type LoggerObserver struct{}

func (LoggerObserver) Observe(stop <-chan struct{}, w *world.World, logger logrus.FieldLogger) {
	go func() {
		for event := range w.Events(stop, chanSnakeObserverEventsBuffer) {
			switch event.Type {
			case world.EventTypeError:
				if err, ok := event.Payload.(error); ok {
					logger.WithError(err).Error("world error")
				}
			case world.EventTypeObjectCreate, world.EventTypeObjectDelete, world.EventTypeObjectUpdate, world.EventTypeObjectChecked:
				logger.WithFields(logrus.Fields{
					"payload": event.Payload,
					"type":    event.Type,
				}).Debug("world event")
			}
		}
	}()
}

package game

import (
	"sync"
	"time"

	"github.com/ivan1993spb/snake-server/engine"
	"github.com/ivan1993spb/snake-server/playground"
)

const worldEventsBufferSize = 512

const worldEventsTimeout = time.Second

type World interface {
	ObjectExists(object interface{}) bool
	LocationExists(location engine.Location) bool
	EntityExists(object interface{}, location engine.Location) bool
	GetObjectByLocation(location engine.Location) interface{}
	GetObjectByDot(dot *engine.Dot) interface{}
	GetEntityByDot(dot *engine.Dot) (interface{}, engine.Location)
	GetObjectsByDots(dots []*engine.Dot) []interface{}
	CreateObject(object interface{}, location engine.Location) error
	CreateObjectAvailableDots(object interface{}, location engine.Location) (engine.Location, *playground.ErrCreateObjectAvailableDots)
	DeleteObject(object interface{}, location engine.Location) *playground.ErrDeleteObject
	UpdateObject(object interface{}, old, new engine.Location) *playground.ErrUpdateObject
	UpdateObjectAvailableDots(object interface{}, old, new engine.Location) (engine.Location, *playground.ErrUpdateObjectAvailableDots)
	CreateObjectRandomDot(object interface{}) (engine.Location, error)
	CreateObjectRandomRect(object interface{}, rw, rh uint8) (engine.Location, error)
	Navigate(dot *engine.Dot, dir engine.Direction, dis uint8) (*engine.Dot, error)
	Size() uint16
	Width() uint8
	Height() uint8
}

type world struct {
	pg      *playground.Playground
	chs     []chan Event
	chsMux  *sync.RWMutex
	stop    chan struct{}
	timeout time.Duration
}

func newWorld(pg *playground.Playground) *world {
	return &world{
		pg:      pg,
		chs:     make([]chan Event, 0),
		chsMux:  &sync.RWMutex{},
		stop:    make(chan struct{}, 0),
		timeout: worldEventsTimeout,
	}
}

func (w *world) event(event Event) {
	go func() {
		w.chsMux.RLock()

		wg := sync.WaitGroup{}
		wg.Add(len(w.chs))
		go func() {
			wg.Wait()
			w.chsMux.RUnlock()
		}()

		for _, ch := range w.chs {
			go func(ch chan Event) {
				var timer = time.NewTimer(w.timeout)
				defer timer.Stop()

				select {
				case ch <- event:
				case <-w.stop:
				case <-timer.C:
				}

				wg.Done()
			}(ch)
		}
	}()
}

func (w *world) RunObserver(observer Observer) {
	ch := make(chan Event, worldEventsBufferSize)

	w.chsMux.Lock()
	w.chs = append(w.chs, ch)
	w.chsMux.Unlock()

	observer.Run(ch)

	w.chsMux.Lock()
	for i := range w.chs {
		if w.chs[i] == ch {
			close(ch)
			w.chs = append(w.chs[:i], w.chs[i+1:]...)
			break
		}
	}
	w.chsMux.Unlock()
}

func (w *world) Stop() {
	close(w.stop)

	w.chsMux.Lock()
	defer w.chsMux.Unlock()

	for _, ch := range w.chs {
		close(ch)
	}

	w.chs = w.chs[:0]
}

func (w *world) ObjectExists(object interface{}) bool {
	return w.pg.ObjectExists(object)
}

func (w *world) LocationExists(location engine.Location) bool {
	return w.pg.LocationExists(location)
}

func (w *world) EntityExists(object interface{}, location engine.Location) bool {
	return w.pg.EntityExists(object, location)
}

func (w *world) GetObjectByLocation(location engine.Location) interface{} {
	if object := w.pg.GetObjectByLocation(location); object != nil {
		w.event(Event{
			Type:    EventTypeObjectChecked,
			Payload: object,
		})
		return object
	}
	return nil

}

func (w *world) GetObjectByDot(dot *engine.Dot) interface{} {
	if object := w.pg.GetObjectByDot(dot); object != nil {
		w.event(Event{
			Type:    EventTypeObjectChecked,
			Payload: object,
		})
		return object
	}
	return nil
}

func (w *world) GetEntityByDot(dot *engine.Dot) (interface{}, engine.Location) {
	if object, location := w.pg.GetEntityByDot(dot); object != nil && !location.Empty() {
		w.event(Event{
			Type:    EventTypeObjectChecked,
			Payload: object,
		})
		return object, location
	}
	return nil, nil
}

func (w *world) GetObjectsByDots(dots []*engine.Dot) []interface{} {
	if objects := w.pg.GetObjectsByDots(dots); len(objects) > 0 {
		for _, object := range objects {
			w.event(Event{
				Type:    EventTypeObjectChecked,
				Payload: object,
			})
		}
		return objects
	}
	return nil
}

func (w *world) CreateObject(object interface{}, location engine.Location) error {
	if err := w.pg.CreateObject(object, location); err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return err
	}
	w.event(Event{
		Type:    EventTypeObjectCreate,
		Payload: object,
	})
	return nil
}

func (w *world) CreateObjectAvailableDots(object interface{}, location engine.Location) (engine.Location, *playground.ErrCreateObjectAvailableDots) {
	location, err := w.pg.CreateObjectAvailableDots(object, location)
	if err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return nil, err
	}
	w.event(Event{
		Type:    EventTypeObjectCreate,
		Payload: object,
	})
	return location, err
}

func (w *world) DeleteObject(object interface{}, location engine.Location) *playground.ErrDeleteObject {
	err := w.pg.DeleteObject(object, location)
	if err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return err
	}
	w.event(Event{
		Type:    EventTypeObjectDelete,
		Payload: object,
	})
	return err
}

func (w *world) UpdateObject(object interface{}, old, new engine.Location) *playground.ErrUpdateObject {
	if err := w.pg.UpdateObject(object, old, new); err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return err
	}
	w.event(Event{
		Type:    EventTypeObjectUpdate,
		Payload: object,
	})
	return nil
}

func (w *world) UpdateObjectAvailableDots(object interface{}, old, new engine.Location) (engine.Location, *playground.ErrUpdateObjectAvailableDots) {
	location, err := w.pg.UpdateObjectAvailableDots(object, old, new)
	if err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return nil, err
	}
	w.event(Event{
		Type:    EventTypeObjectUpdate,
		Payload: object,
	})
	return location, err
}

func (w *world) CreateObjectRandomDot(object interface{}) (engine.Location, error) {
	location, err := w.pg.CreateObjectRandomDot(object)
	if err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return nil, err
	}
	w.event(Event{
		Type:    EventTypeObjectCreate,
		Payload: object,
	})
	return location, err
}

func (w *world) CreateObjectRandomRect(object interface{}, rw, rh uint8) (engine.Location, error) {
	location, err := w.pg.CreateObjectRandomRect(object, rw, rh)
	if err != nil {
		w.event(Event{
			Type:    EventTypeError,
			Payload: err,
		})
		return nil, err
	}
	w.event(Event{
		Type:    EventTypeObjectCreate,
		Payload: object,
	})
	return location, err
}

func (w *world) Navigate(dot *engine.Dot, dir engine.Direction, dis uint8) (*engine.Dot, error) {
	return w.pg.Navigate(dot, dir, dis)
}

func (w *world) Size() uint16 {
	return w.pg.Size()
}

func (w *world) Width() uint8 {
	return w.pg.Width()
}

func (w *world) Height() uint8 {
	return w.pg.Height()
}

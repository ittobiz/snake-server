package game

import (
	"sync"
	"time"

	"github.com/ivan1993spb/snake-server/engine"
	"github.com/ivan1993spb/snake-server/playground"
)

const (
	worldEventsChanMainBufferSize  = 512
	worldEventsChanProxyBufferSize = 128
	worldEventsChanOutBufferSize   = 32

	worldEventsSendTimeout = time.Millisecond * 100
)

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
	pg          *playground.Playground
	chMain      chan Event
	chsProxy    []chan Event
	chsProxyMux *sync.RWMutex
	stopGlobal  chan struct{}
	flagStarted bool
}

func newWorld(pg *playground.Playground) *world {
	return &world{
		pg:          pg,
		chMain:      make(chan Event, worldEventsChanMainBufferSize),
		chsProxy:    make([]chan Event, 0),
		chsProxyMux: &sync.RWMutex{},
		stopGlobal:  make(chan struct{}, 0),
	}
}

func (w *world) event(event Event) {
	select {
	case w.chMain <- event:
	case <-w.stopGlobal:
	}
}

func (w *world) start() {
	if !w.flagStarted {
		return
	}
	w.flagStarted = true

	go func() {
		for {
			select {
			case event := <-w.chMain:
				w.broadcast(event)
			case <-w.stopGlobal:
				return
			}
		}
	}()
}

func (w *world) broadcast(event Event) {
	w.chsProxyMux.RLock()
	defer w.chsProxyMux.RUnlock()

	for _, chProxy := range w.chsProxy {
		select {
		case chProxy <- event:
		case <-w.stopGlobal:
		}
	}
}

func (w *world) createChanProxy() chan Event {
	chProxy := make(chan Event, worldEventsChanProxyBufferSize)

	w.chsProxyMux.Lock()
	w.chsProxy = append(w.chsProxy, chProxy)
	w.chsProxyMux.Unlock()

	return chProxy
}

func (w *world) deleteChanProxy(chProxy chan Event) {
	w.chsProxyMux.Lock()
	for i := range w.chsProxy {
		if w.chsProxy[i] == chProxy {
			w.chsProxy = append(w.chsProxy[:i], w.chsProxy[i+1:]...)
			close(chProxy)
			break
		}
	}
	w.chsProxyMux.Unlock()
}

func (w *world) Events(stop <-chan struct{}) <-chan Event {
	chProxy := w.createChanProxy()
	chOut := make(chan Event, worldEventsChanOutBufferSize)

	go func() {
		defer close(chOut)
		defer w.deleteChanProxy(chProxy)

		for {
			select {
			case <-stop:
				return
			case <-w.stopGlobal:
				return
			case event := <-chProxy:
				w.sendEvent(chOut, event, stop, worldEventsSendTimeout)
			}
		}
	}()

	return chOut
}

func (w *world) sendEvent(ch chan Event, event Event, stop <-chan struct{}, timeout time.Duration) {
	var timer = time.NewTimer(timeout)
	defer timer.Stop()
	if cap(ch) == 0 {
		select {
		case ch <- event:
		case <-w.stopGlobal:
		case <-stop:
		case <-timer.C:
		}
	} else {
		for {
			select {
			case ch <- event:
				return
			case <-w.stopGlobal:
				return
			case <-stop:
				return
			case <-timer.C:
				return
			default:
				if len(ch) == cap(ch) {
					<-ch
				}
			}
		}
	}
}

func (w *world) stop() {
	close(w.stopGlobal)
	close(w.chMain)

	w.chsProxyMux.Lock()
	defer w.chsProxyMux.Unlock()

	for _, ch := range w.chsProxy {
		close(ch)
	}

	w.chsProxy = w.chsProxy[:0]
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

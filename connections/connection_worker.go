package connections

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pquerna/ffjson/ffjson"
	"github.com/sirupsen/logrus"

	"github.com/ivan1993spb/snake-server/broadcast"
	"github.com/ivan1993spb/snake-server/game"
	"github.com/ivan1993spb/snake-server/player"
)

const (
	chanOutputMessageBuffer     = 128
	chanReadMessagesBuffer      = 128
	chanDecodeMessageBuffer     = 128
	chanEncodeMessageBuffer     = 128
	chanProxyInputMessageBuffer = 64
	chanInputMessagesBuffer     = 32
	chanBroadcastBuffer         = 32
	chanEventsBuffer            = 32
	chanSnakeCommandsBuffer     = 32

	sendInputMessageTimeout = time.Millisecond * 50
)

type ConnectionWorker struct {
	conn   *websocket.Conn
	logger logrus.FieldLogger

	chsInput    []chan InputMessage
	chsInputMux *sync.RWMutex

	flagStarted bool
}

func NewConnectionWorker(conn *websocket.Conn, logger logrus.FieldLogger) *ConnectionWorker {
	return &ConnectionWorker{
		conn:        conn,
		logger:      logger,
		chsInput:    make([]chan InputMessage, 0),
		chsInputMux: &sync.RWMutex{},
	}
}

type ErrStartConnectionWorker string

func (e ErrStartConnectionWorker) Error() string {
	return "error start connection worker: " + string(e)
}

func (cw *ConnectionWorker) Start(stop <-chan struct{}, game *game.Game, broadcast *broadcast.GroupBroadcast) error {
	if cw.flagStarted {
		return ErrStartConnectionWorker("connection worker already started")
	}

	cw.flagStarted = true

	broadcast.BroadcastMessage("user joined your game group")

	// Input
	chInputBytes, chStop := cw.read()
	chInputMessages := cw.decode(chInputBytes, chStop)
	cw.broadcastInputMessage(chInputMessages, chStop)
	chCommands := cw.listenSnakeCommands(chStop, cw.input(chStop, chanInputMessagesBuffer))
	cw.listenPlayerBroadcasts(chStop, cw.input(chStop, chanInputMessagesBuffer), broadcast)

	p := player.NewPlayer(cw.logger, game.World())

	// Output
	chPlayer := p.Start(chStop, chCommands)
	chGame := game.ListenEvents(chStop, chanEventsBuffer)
	chBroadcast := broadcast.ListenMessages(chStop, chanBroadcastBuffer)
	chOutputBytes := cw.encode(chStop, cw.listenBroadcast(chStop, chBroadcast), cw.listenPlayer(chStop, chPlayer), cw.listenGame(chStop, chGame))
	cw.write(chOutputBytes, chStop)

	select {
	case <-chStop:
		// On connection error
	case <-stop:
		// External stop
	}

	broadcast.BroadcastMessage("user left your game group")

	cw.stopInputs()

	return nil
}

func (cw *ConnectionWorker) stopInputs() {
	cw.chsInputMux.Lock()
	defer cw.chsInputMux.Unlock()

	for _, ch := range cw.chsInput {
		close(ch)
	}

	cw.chsInput = cw.chsInput[:0]
}

func (cw *ConnectionWorker) read() (<-chan []byte, <-chan struct{}) {
	chout := make(chan []byte, chanReadMessagesBuffer)
	chstop := make(chan struct{}, 0)

	go func() {
		defer close(chout)
		defer close(chstop)

		for {
			messageType, data, err := cw.conn.ReadMessage()
			if err != nil {
				cw.logger.Errorln("read input message error:", err)
				return
			}

			if websocket.TextMessage != messageType {
				cw.logger.Warning("unexpected input message type")
				continue
			}

			chout <- data
		}
	}()

	return chout, chstop
}

func (cw *ConnectionWorker) decode(chin <-chan []byte, stop <-chan struct{}) <-chan InputMessage {
	chout := make(chan InputMessage, chanDecodeMessageBuffer)

	go func() {
		defer close(chout)

		var decoder = ffjson.NewDecoder()

		for {
			select {
			case data := <-chin:
				var inputMessage InputMessage
				if err := decoder.Decode(data, &inputMessage); err != nil {
					cw.logger.Errorln("decode input message error:", err)
				} else {
					chout <- inputMessage
				}
			case <-stop:
				return
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) broadcastInputMessage(chin <-chan InputMessage, stop <-chan struct{}) {
	go func() {
		for {
			select {
			case inputMessage := <-chin:
				cw.chsInputMux.RLock()
				for _, ch := range cw.chsInput {
					select {
					case ch <- inputMessage:
					case <-stop:
						return
					}
				}
				cw.chsInputMux.RUnlock()
			case <-stop:
				return
			}
		}
	}()
}

func (cw *ConnectionWorker) input(stop <-chan struct{}, buffer uint) <-chan InputMessage {
	chProxy := make(chan InputMessage, chanProxyInputMessageBuffer)

	cw.chsInputMux.Lock()
	cw.chsInput = append(cw.chsInput, chProxy)
	cw.chsInputMux.Unlock()

	chout := make(chan InputMessage, buffer)

	go func() {
		defer close(chout)
		defer func() {
			cw.chsInputMux.Lock()
			for i := range cw.chsInput {
				if cw.chsInput[i] == chProxy {
					cw.chsInput = append(cw.chsInput[:i], cw.chsInput[i+1:]...)
					close(chProxy)
					break
				}
			}
			cw.chsInputMux.Unlock()
		}()

		for {
			select {
			case <-stop:
				return
			case inputMessage := <-chProxy:
				cw.sendInputMessage(chout, inputMessage, stop, sendInputMessageTimeout)
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) sendInputMessage(ch chan InputMessage, inputMessage InputMessage, stop <-chan struct{}, timeout time.Duration) {
	const tickSize = 5

	var timer = time.NewTimer(timeout)
	defer timer.Stop()

	var ticker = time.NewTicker(timeout / tickSize)
	defer ticker.Stop()

	if cap(ch) == 0 {
		select {
		case ch <- inputMessage:
		case <-stop:
		case <-timer.C:
		}
	} else {
		for {
			select {
			case ch <- inputMessage:
				return
			case <-stop:
				return
			case <-timer.C:
				return
			case <-ticker.C:
				if len(ch) == cap(ch) {
					<-ch
				}
			}
		}
	}
}

func (cw *ConnectionWorker) write(chin <-chan []byte, stop <-chan struct{}) {
	go func() {
		for {
			select {
			case data := <-chin:
				if err := cw.conn.WriteMessage(websocket.TextMessage, data); err != nil {
					cw.logger.Errorln("write output message error:", err)
				}
			case <-stop:
				return
			}
		}
	}()
}

func (cw *ConnectionWorker) encode(stop <-chan struct{}, chins ...<-chan OutputMessage) <-chan []byte {
	chout := make(chan []byte, chanEncodeMessageBuffer)

	wg := sync.WaitGroup{}
	wg.Add(len(chins))

	for _, chin := range chins {
		go func(chin <-chan OutputMessage) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				case message, ok := <-chin:
					if !ok {
						return
					}
					if data, err := ffjson.Marshal(message); err != nil {
						cw.logger.Errorln("encode output message error:", err)
					} else {
						chout <- data
					}
				}
			}
		}(chin)
	}

	go func() {
		wg.Wait()
		close(chout)
	}()

	return chout
}

func (cw *ConnectionWorker) listenGame(stop <-chan struct{}, chin <-chan game.Event) <-chan OutputMessage {
	chout := make(chan OutputMessage, chanOutputMessageBuffer)

	go func() {
		defer close(chout)

		for {
			select {
			case event := <-chin:
				outputMessage := OutputMessage{
					Type:    OutputMessageTypeGame,
					Payload: event,
				}

				select {
				case chout <- outputMessage:
				case <-stop:
					return
				}
			case <-stop:
				return
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) listenPlayer(stop <-chan struct{}, chin <-chan player.Message) <-chan OutputMessage {
	chout := make(chan OutputMessage, chanOutputMessageBuffer)

	go func() {
		defer close(chout)

		for {
			select {
			case event := <-chin:
				outputMessage := OutputMessage{
					Type:    OutputMessageTypePlayer,
					Payload: event,
				}

				select {
				case chout <- outputMessage:
				case <-stop:
					return
				}
			case <-stop:
				return
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) listenBroadcast(stop <-chan struct{}, chin <-chan broadcast.BroadcastMessage) <-chan OutputMessage {
	chout := make(chan OutputMessage, chanOutputMessageBuffer)

	go func() {
		defer close(chout)

		for {
			select {
			case message := <-chin:
				outputMessage := OutputMessage{
					Type:    OutputMessageTypeBroadcast,
					Payload: message,
				}

				select {
				case chout <- outputMessage:
				case <-stop:
					return
				}
			case <-stop:
				return
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) listenSnakeCommands(stop <-chan struct{}, chin <-chan InputMessage) <-chan string {
	chout := make(chan string, chanSnakeCommandsBuffer)

	go func() {
		defer close(chout)

		for {
			select {
			case message := <-chin:
				if message.Type == InputMessageTypeSnakeCommand {
					select {
					case chout <- message.Payload:
					case <-stop:
						return
					}
				}
			case <-stop:
				return
			}
		}
	}()

	return chout
}

func (cw *ConnectionWorker) listenPlayerBroadcasts(stop <-chan struct{}, chin <-chan InputMessage, b *broadcast.GroupBroadcast) {
	go func() {
		for {
			select {
			case message := <-chin:
				if message.Type == InputMessageTypeBroadcast {
					b.BroadcastMessage(broadcast.BroadcastMessage(message.Payload))
				}
			case <-stop:
				return
			}
		}
	}()
}

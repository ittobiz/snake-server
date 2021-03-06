package player

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ivan1993spb/snake-server/objects/snake"
	"github.com/ivan1993spb/snake-server/world"
)

const countdown = 5

const chanMessageBuffer = 16

const chanErrorBuffer = 32

type Player struct {
	world  *world.World
	logger logrus.FieldLogger
}

func NewPlayer(logger logrus.FieldLogger, world *world.World) *Player {
	return &Player{
		logger: logger,
		world:  world,
	}
}

func (p *Player) Start(stop <-chan struct{}, chin <-chan string) <-chan Message {
	chout := make(chan Message, chanMessageBuffer)
	localStopper := make(chan struct{})

	go func() {
		<-stop
		close(localStopper)
	}()

	go func() {
		defer close(chout)

		chout <- NewMessageNotice("welcome to snake server!")
		chout <- NewMessageSize(p.world.Width(), p.world.Height())
		chout <- NewMessageObjects(p.world.GetObjects())

		for {
			chout <- NewMessageCountdown(countdown)

			timer := time.NewTimer(time.Second * countdown)
			select {
			case <-timer.C:
				timer.Stop()
			case <-localStopper:
				timer.Stop()
				return
			}

			chout <- NewMessageNotice("start")

			s, err := snake.NewSnake(p.world)
			if err != nil {
				chout <- NewMessageError("cannot create snake")
				p.logger.Errorln("cannot create snake to player:", err)
				continue
			}
			snakeStop := s.Run(localStopper)

			chout <- NewMessageSnake(s.GetUUID())

			p.processSnakeCommands(snakeStop, chin, s)

			select {
			case <-snakeStop:
			case <-localStopper:
				return
			}
		}
	}()

	return chout
}

func (p *Player) processSnakeCommands(stop <-chan struct{}, chin <-chan string, s *snake.Snake) <-chan error {
	errch := make(chan error, chanErrorBuffer)

	go func() {
		defer close(errch)
		for {
			select {
			case <-stop:
				return
			case command := <-chin:
				p.logger.WithField("command", command).Debug("received snake command")
				if err := s.Command(snake.Command(command)); err != nil {
					// TODO: Handle error and send it to channel with timeout!
					//errch <- err
				}
			}
		}
	}()

	return errch
}

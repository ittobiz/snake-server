
# Snake server [![Build Status](https://travis-ci.org/ivan1993spb/snake-server.svg?branch=master)](https://travis-ci.org/ivan1993spb/snake-server) [![Go Report Card](https://goreportcard.com/badge/github.com/ivan1993spb/snake-server)](https://goreportcard.com/report/github.com/ivan1993spb/snake-server)

Server for online arcade game - snake.

// TODO: Create screen shot

## Installation

### Go get

Use `go get -u github.com/ivan1993spb/snake-server` to install server.

### Docker

Use `docker pull ivan1993spb/snake-server` to pull server image from docker hub.

## CLI arguments

// TODO: Create arguments description

## API Description

API methods provide JSON format.

### Request `POST /games`

Creates game.

```
curl -v -X POST -d limit=3 -d width=100 -d height=100 http://localhost:8080/games
```

### Request `DELETE /games/{id}`

Deletes game if there is not players.

```
curl -v -X DELETE http://localhost:8080/games/0
```

### Request `GET /games`

Return info about all games on server.

```
curl -v -X GET http://localhost:8080/games
```

### Request `GET /games/{id}`

Returns game information

```
curl -v -X GET http://localhost:8080/games/0
```

### Request `GET /games/{id}/ws`

Connects to game WebSocket.

* return playground size : width and height
* return room_id and player_id
* initialize gamer objects and session
* return all objects on playground
* push events and objects from game

Primitives:

* Area: `[width, height]`
* Direction: `"n"`, `"w"`, `"s"`, `"e"`
* Dot: `[x, y]`
* Dot list: `[[x, y], [x, y], [x, y], [x, y], [x, y], [x, y]]`
* Location: `[[x, y], [x, y], [x, y], [x, y], [x, y], [x, y]]`
* Rect: `[x, y, width, height]`

Game objects:

* Apple: `{"type": "apple", "id": 1, "dot": [x, y]}`
* Corpse: `{"type": "corpse", "id": 2, "dots": [[x, y], [x, y], [x, y]]}`
* Mouse: `{"type": "mouse", "id": 3, dot: [x, y], "dir": "n"}`
* Snake: `{"type": "snake", "id": 4, "dots": [[x, y], [x, y], [x, y]]}`
* Wall: `{"type": "wall", "id": 5, "dots": [[x, y], [x, y], [x, y]]}`
* Watermelon: `{"type": "watermelon", "id": 6, "dots": [[x, y], [x, y], [x, y]]}`

Message types:

* Object: `{"type": "object", "object": {}}` - delete, update or create
* Error: `{"type": "error", "message": "text"}`
* Notice: `{"type": "notice", "message": "text"}`
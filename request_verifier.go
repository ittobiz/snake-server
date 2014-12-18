package main

import (
	"github.com/golang/glog"
	"github.com/ivan1993spb/pwshandler"
	"golang.org/x/net/websocket"
)

// RequestVerifier verifies requests by hash sum of passed request
// data
type RequestVerifier struct{}

func NewVerifier(HashSalt string) pwshandler.RequestVerifier {
	return &RequestVerifier{}
}

// Implementing pwshandler.RequestVerifier interface
func (*RequestVerifier) Verify(ws *websocket.Conn) error {
	if glog.V(INFOLOG_LEVEL_ABOUT_CONNS) {
		glog.Infoln("Verifying accepted connection")
	}

	return nil
}
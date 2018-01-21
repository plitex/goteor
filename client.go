// Copyright 2015 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:3000", "http service address")

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/websocket"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// Send connect
	msg := map[string]interface{}{
		"msg":     "connect",
		"version": "1",
		"support": []string{"pre1", "pre2", "1"},
	}
	c.WriteJSON(msg)

	done := make(chan struct{})

	methodID := 0

	go func() {
		defer c.Close()
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)

			var m map[string]interface{}
			if err := json.Unmarshal(message, &m); err != nil {
				fmt.Printf("error: %v", err)
				break
			}

			switch m["msg"] {
			case "ping":
				pong := map[string]string{
					"msg": "pong",
				}
				c.WriteJSON(pong)
			case "connected":
				// Subscribe
				methodID++
				sub := map[string]interface{}{
					"msg":  "sub",
					"id":   strconv.Itoa(methodID),
					"name": "mysubscription",
				}
				err := c.WriteJSON(sub)
				if err != nil {
					log.Println("write:", err)
					return
				}

				// Method
				methodID++
				method := map[string]interface{}{
					"msg":    "method",
					"id":     strconv.Itoa(methodID),
					"method": "hello",
					"params": [1]interface{}{"Miguel"},
				}
				err = c.WriteJSON(method)
				if err != nil {
					log.Println("write:", err)
					return
				}
			default:
				// fmt.Println("error: Unknown message received from client")
			}
		}
	}()

	ticker := time.NewTicker(time.Second * 60)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ping := map[string]string{
				"msg": "ping",
				"id":  "1",
			}
			err := c.WriteJSON(ping)
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")
			// To cleanly close a connection, a client should send a close
			// frame and wait for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			c.Close()
			return
		}
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/plitex/goteor/ddpserver"
)

var addr = flag.String("addr", "127.0.0.1:3000", "http service address")

func handleHello(ctx ddpserver.MethodContext) (interface{}, *ddpserver.Error) {
	if len(ctx.Params) != 1 {
		err := ddpserver.NewError("wrong-params", "Missing required parameter", "")
		return nil, err
	}

	name, ok := ctx.Params[0].(string)
	if !ok {
		err := ddpserver.NewError("wrong-params", "Incorrect name", "")
		return nil, err
	}

	return fmt.Sprintf("Hello %s", name), nil
}

func handleMySubscription(ctx ddpserver.PublicationContext) (interface{}, *ddpserver.Error) {
	ctx.Ready()

	return []interface{}{}, nil
}

func main() {
	fmt.Println("Starting DDPServer")
	flag.Parse()

	ddpServer := ddpserver.NewServer()

	ddpServer.Method("hello", handleHello)
	ddpServer.Publish("mysubscription", handleMySubscription)

	ddpServer.Run()
	http.HandleFunc("/websocket", func(w http.ResponseWriter, r *http.Request) {
		ddpserver.ServeWs(ddpServer, w, r)
	})

	fmt.Println("Listening at ws://" + *addr + "/websocket")

	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe:  ", err)
	}
}

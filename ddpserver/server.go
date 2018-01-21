package ddpserver

import (
	"log"
	"net/http"
)

type Server struct {
	id      string
	running bool

	// Registered connections.
	connections map[*Connection]bool

	// Register requests from the connections.
	register chan *Connection

	// Unregister requests from connections.
	unregister chan *Connection

	// Method handlers
	methods map[string]MethodHandler

	// Publication handlers
	publications map[string]PublicationHandler
}

func NewServer() *Server {
	return &Server{
		id:           "0",
		register:     make(chan *Connection),
		unregister:   make(chan *Connection),
		connections:  make(map[*Connection]bool),
		methods:      make(map[string]MethodHandler),
		publications: make(map[string]PublicationHandler),
	}
}

func (s *Server) Run() {
	if s.running == false {
		go func() {
			s.running = true
			for {
				select {
				case conn := <-s.register:
					s.connections[conn] = true
				case conn := <-s.unregister:
					if _, ok := s.connections[conn]; ok {
						delete(s.connections, conn)
						close(conn.send)
					}
				}
			}
		}()
	}
}

// Method registers new method
func (s *Server) Method(name string, fn MethodHandler) {
	s.methods[name] = fn
}

// Publish registers new publication
func (s *Server) Publish(name string, fn PublicationHandler) {
	s.publications[name] = fn
}

// ServeWs handles websocket requests from the peer.
func ServeWs(server *Server, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Client connected")

	c := &Connection{
		server:        server,
		conn:          conn,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]*PublicationContext),
	}
	c.server.register <- c

	// Start connection go routines
	c.run()
}

package ddpserver

type MethodHandler func(MethodContext) (result interface{}, err *Error)

type MethodContext struct {
	ID      string
	Params  []interface{}
	conn    Connection
	done    bool
	updated bool
}

func NewMethodContext(m Message, conn Connection) MethodContext {
	ctx := MethodContext{}
	ctx.conn = conn
	ctx.ID = m.ID
	ctx.Params = m.Params
	return ctx
}

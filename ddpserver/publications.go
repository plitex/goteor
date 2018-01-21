package ddpserver

type PublicationHandler func(ctx PublicationContext) (result interface{}, err *Error)

type PublicationContext struct {
	ID     string
	Params []interface{}
	conn   Connection
	ready  bool
}

func NewPublicationContext(m Message, conn Connection) *PublicationContext {
	ctx := &PublicationContext{}
	ctx.conn = conn
	ctx.ID = m.ID
	ctx.Params = m.Params
	return ctx
}

func (ctx *PublicationContext) Ready() {
	ctx.ready = true

	ctx.conn.sendSubscriptionReady(ctx)
}

func (ctx *PublicationContext) Added(collection string, id string, fields map[string]interface{}) {
}

func (ctx *PublicationContext) Changed(collection string, id string, fields map[string]interface{}) {
}

func (ctx *PublicationContext) Removed(collection string, id string) {
}

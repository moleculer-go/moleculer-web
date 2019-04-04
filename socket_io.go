package gateway

import (
	"sync"

	socketio "github.com/googollee/go-socket.io"
	"github.com/moleculer-go/moleculer"
)

type clientEntry struct {
	conn socketio.Conn
}

type topicEntry struct {
	topic   string
	context moleculer.BrokerContext
	started bool
	list    []deliveryEntry
}

type deliveryEntry struct {
	ce    clientEntry
	name  string
	value string
}

// validate check if params should be added in the event stream
func (te *topicEntry) validate(params moleculer.Payload, name, value string) bool {
	pvalue := params.Get(name)
	if pvalue.Exists() && pvalue.String() == value {
		te.context.Logger().Debug("socker.io event validate() - record is valid !")
		return true
	}
	te.context.Logger().Debug("socker.io event validate() - record is invalid :(")
	return false
}

func eventService(topic string, handler moleculer.EventHandler) moleculer.Service {
	return moleculer.Service{
		Name: "socket_io-events_delivery-" + topic,
		Events: []moleculer.Event{
			{
				Name:    topic,
				Handler: handler,
			},
		},
	}
}

// startDelivery start the delivery of moleculer events to socket events.
func (te *topicEntry) startDelivery(ce clientEntry, name, value string) {
	te.list = append(te.list, deliveryEntry{ce, name, value})
	if te.started {
		return
	}
	te.started = true
	te.context.Logger().Debug("startDelivery() started for topic: ", te.topic)

	te.context.AddService(eventService(te.topic, func(context moleculer.Context, params moleculer.Payload) {
		context.Logger().Debug("event handler for topic: ", te.topic, " received params: ", params)
		for _, de := range te.list {
			if te.validate(params, de.name, de.value) {
				de.ce.conn.Emit(value+"."+te.topic, params.Value())
			}
		}
	}))
}

// createDelivery creates a delivery of moleculer events to a socket.io listener.
func createDelivery(context moleculer.BrokerContext, clients *sync.Map) func(socketio.Conn, string, string, string) {
	handlers := &sync.Map{}
	return func(conn socketio.Conn, name, value, topic string) {
		context.Logger().Debug("onEvent delivery -> name: ", name, " value: ", value, " topic: ", topic)
		client, ok := clients.Load(conn.ID())
		if ok {
			temp, exists := handlers.Load(topic)
			var te topicEntry
			if exists {
				te = temp.(topicEntry)
			} else {
				te = topicEntry{context: context, topic: topic}
				handlers.Store(topic, te)
			}
			te.startDelivery(client.(clientEntry), name, value)
		}
	}
}

// createIOServer Creates and start a new socket.io server.
func createIOServer(context moleculer.BrokerContext) *socketio.Server {
	server, err := socketio.NewServer(nil)
	clients := &sync.Map{}
	if err != nil {
		context.Logger().Error("Error creating new socket io server - error: ", err)
	}
	server.OnConnect("/", func(conn socketio.Conn) error {
		context.Logger().Debug("socketio connection conn.ID: ", conn.ID())
		clients.Store(conn.ID(), clientEntry{conn})
		return nil
	})

	server.OnEvent("/", "delivery", createDelivery(context, clients))
	server.OnDisconnect("/", func(conn socketio.Conn, msg string) {
		context.Logger().Debug("socketio disconnected conn.ID: ", conn.ID())
		clients.Delete(conn.ID())
	})
	return server
}

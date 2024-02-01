package msgpublisher

import (
	"log"

	natsconnect "github.com/csabakissmalta/tpee/nats_connect"
)

const (
	PUB_BETBUILDER_1 = "betbuilder.csaba"
)

type Option func(mp *MsgPublisher)

type Processer interface {
	// process function
	Process(data_in any) ([]byte, error)
}

type MsgPublisher struct {
	// NATS client
	NClient *natsconnect.NATSClient

	// processer
	Processer Processer
}

// new instance
func New(opts ...Option) *MsgPublisher {
	p := &MsgPublisher{}
	for _, o := range opts {
		o(p)
	}
	return p
}

// with client
func WithNATSClient(nc *natsconnect.NATSClient) Option {
	return func(mp *MsgPublisher) {
		mp.NClient = nc
	}
}

// With data processer
// The processer needs to implement the Process method,
// which will be published to the subject or stream,
// set on the NATSClient
func WithProcesser(p Processer) Option {
	return func(mp *MsgPublisher) {
		mp.Processer = p
	}
}

func (mp *MsgPublisher) Publish(raw_msg any) error {
	// The processer
	msg, err := mp.Processer.Process(raw_msg)
	if err != nil {
		log.Println("ERR: Failed processing the raw message.\n", err.Error())
	}

	// this is the point, where the NATSClient needs to publish
	if msg == nil {
		log.Println("...PROBABLY DEBUGGING?")
	}
	log.Println("PUBLISHING: ", PUB_BETBUILDER_1, string(msg))
	mp.NClient.Publish(PUB_BETBUILDER_1, msg)
	return nil
}

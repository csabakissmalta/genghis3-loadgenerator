package runtimeconfig

import (
	"context"
	"log"
	"strconv"

	natsconnect "github.com/csabakissmalta/tpee/nats_connect"
	"github.com/csabakissmalta/tpee/timeline"
	"github.com/nats-io/nats.go/jetstream"
)

// 1. create a kv under the testrun name (ticket-date-testrun)
//   - the bucket has to be ephemeral - when the etst is finished or interrupted, the bucket has to be deleted
//
// 2. add the keys
//   - these keys are only runtime values
//   - new keys modifying the behaviour of the lod generator or the traffic characteristics
//   - structure has to be populated to be used as a template

// runtime properties - should be watched
type RuntimeConfig struct {
	// Set the name for the conifig  - will be set for the KV store too
	Name string

	// traffic rates - per timeline
	TrafficRates map[string]int32

	// Current delta
	CurrentTransition *timeline.Transition

	// nats client
	NATSClient *natsconnect.NATSClient

	// watch channel for the key-value store
	UpdatesChannel <-chan jetstream.KeyValueEntry

	// KV store
	KVStore jetstream.KeyValue
}

type Option func(*RuntimeConfig)

// new
func NeWithName(name string, opts ...Option) *RuntimeConfig {
	rc := &RuntimeConfig{
		Name: name,
	}
	for _, o := range opts {
		o(rc)
	}
	return rc
}

// Returns the option func with map of timeline names mapped to traffic rates
func WithTrafficRatesConfigs(tls []*timeline.Timeline, nc *natsconnect.NATSClient, ctx context.Context) Option {
	rate_map := make(map[string]int32, len(tls))
	return func(r *RuntimeConfig) {
		js, _ := jetstream.New(nc.Conn)

		// ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
		// defer cancel()

		kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: "runtime",
		})

		// set the struct KV property
		r.KVStore = kv

		// set the natsconn as natsclient
		r.NATSClient = nc

		for _, tl := range tls {
			// kv.Put(ctx, fmt.Sprintf("%s.%s", r.Name, tl.Name), []byte(strconv.Itoa(tl.Rules.Frequency)))
			kv.Put(ctx, tl.Name, []byte(strconv.Itoa(tl.Rules.Frequency)))
			kv.Put(ctx, "transitiontime", []byte("60"))
			rate_map[tl.Name] = int32(tl.Rules.Frequency)
		}

		// set the rates initial values from the exec config
		r.TrafficRates = rate_map

		// set up the watch for the store
		// vals_to_watch := fmt.Sprintf("%s.*", r.Name)
		updates, err := kv.WatchAll(ctx)
		if err != nil {
			log.Println("ERR: Watch KV error\n", err.Error())
		}
		// defer updates.Stop()

		r.UpdatesChannel = updates.Updates()
	}
}

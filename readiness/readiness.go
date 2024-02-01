package readiness

import (
	"log"
	"net/http"
	"os"
	"time"
)

type Option func(*Readiness)

type Readiness struct {
	// the client - calls the endpoint
	Client *http.Client

	// channel to push results into
	IsServiceReady chan bool

	// check interval
	ReadinessCheckIntervalSeconds int32
}

func WithClient(c *http.Client) Option {
	return func(r *Readiness) {
		r.Client = c
	}
}

func WithServiceReadyChannel(ch chan bool) Option {
	return func(r *Readiness) {
		r.IsServiceReady = ch
	}
}

func WithCheckInterval(iv int32) Option {
	return func(r *Readiness) {
		r.ReadinessCheckIntervalSeconds = iv
	}
}

func NewReadinessCheck(option ...Option) *Readiness {
	readiness := &Readiness{}
	for _, o := range option {
		o(readiness)
	}
	return readiness
}

func (rd *Readiness) StartReadinessCheck(rq *http.Request) {
	go func() {
		err_counter := 0
		for {
			rsp, err := rd.Client.Do(rq)
			if err != nil {
				log.Println(err.Error())
				err_counter += 1
				if err_counter > 10 {
					os.Exit(10)
				}
			}

			if rsp.StatusCode < 400 {
				rd.IsServiceReady <- true
				break
			} else {
				rd.IsServiceReady <- false
				time.Sleep(time.Duration(rd.ReadinessCheckIntervalSeconds * int32(time.Second)))
			}
		}
	}()
}

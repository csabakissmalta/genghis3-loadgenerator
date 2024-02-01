package timeseriesreporting

import (
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

var (
	// max_queue  = os.Getenv("MAX_QUEUE")
	max_queue = 100
)

var jobQueue chan *WriteTask

type WriteTask struct {
	DataPoint *write.Point
}

type PointDispatcher struct {
	Queue chan *WriteTask
	DB    *InfluxDB
	quit  chan bool
}

func NewPointDispatcher(db *InfluxDB) *PointDispatcher {
	jobQueue = make(chan *WriteTask, max_queue)
	return &PointDispatcher{
		Queue: jobQueue,
		DB:    db,
		quit:  make(chan bool),
	}
}

func (d *PointDispatcher) Start() {
	go func() {
		for {
			select {
			case t := <-d.Queue:
				d.DB.API.WritePoint(t.DataPoint)
			case <-d.quit:
				return
			}
		}
	}()
}

func (d *PointDispatcher) Stop() {
	d.quit <- true
}

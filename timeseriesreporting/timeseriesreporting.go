package timeseriesreporting

import (
	"time"

	execconf "github.com/csabakissmalta/tpee/exec"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

type InfluxDB struct {
	Client      influxdb2.Client
	API         api.WriteAPI
	Measurement string
}

func NewInfluxdb(settings *execconf.ExecInfluxdbSettings) *InfluxDB {
	c := influxdb2.NewClientWithOptions(settings.DatabaseHost, settings.DatabaseToken, influxdb2.DefaultOptions().SetBatchSize(1000))

	a := c.WriteAPI(settings.DatabaseOrg, settings.Database)
	return &InfluxDB{
		Client:      c,
		API:         a,
		Measurement: settings.Measurement,
	}
}

func NewPt(m string, t map[string]string, f map[string]interface{}, execTs time.Time) *write.Point {
	return influxdb2.NewPoint(m, t, f, execTs)
}

func (infl *InfluxDB) WritePt(pt *write.Point) {
	infl.API.WritePoint(pt)
	infl.API.Flush()
}

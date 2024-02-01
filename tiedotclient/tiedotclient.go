package tiedotclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"time"

	execconf "github.com/csabakissmalta/tpee/exec"
)

// this client supposed to communicate with the tiedot doc database
// should:
// 1. create a test object and pass to the histogram modules
// 2. save the test object (doc)
// 3. when the histogram is written, update the test object and save to the DB

// RequestTested
type RequestTested struct {
	// Name is the label in the postman collection
	Name string `json:"name"`

	// Frequency - the RPS setting in the config
	Frequency int `json:"frequency"`

	// DelaySeconds - the request can have delayed execution
	DelaySeconds int `json:"dealay-seconds"`

	// TiedotDbId - the ID in the DB - needs to be updated once the test is finished
	TiedotDbId string `json:"tiedot-db-id"`
}

// PerfTest
type PerfTest struct {
	// TimeStart - the test started time
	TimeStart time.Time `json:"time-start"`

	// TestDuration - the whole duration of the test
	TestDuration int `json:"test-duration"`

	// VersionLabel - the histogram setting's label
	VersionLabel string `json:"version-label"`

	// PostmanCollection - the collection files's name or path
	PostmanCollection string `json:"postman-collection"`

	// RequestsTested - the list of Requests and their basic settings
	RequestsTested []*RequestTested `json:"requests-tested"`
}

// TiedotClient
type TiedotClient struct {
	// HttpClient
	HttpClient http.Client

	// DBHost
	DBHost string

	// Collection
	TestCollection string

	// PerfTest
	PTest *PerfTest

	// PerfTestDbId
	PerfTestDbId string
}

// TiedotDocQuery
type TiedotDocQuery struct {
	Q string `json:"q"`
}

type Option func(*TiedotClient)

func NewTest(starttime time.Time, postamnColl string, cnf *execconf.Exec) *PerfTest {
	rs := cnf.Requests
	rqs := []*RequestTested{}

	for _, r := range rs {
		rqs = append(rqs, &RequestTested{
			Name:         r.Name,
			Frequency:    r.Frequency,
			DelaySeconds: r.DelaySeconds,
		})
	}

	pt := &PerfTest{
		RequestsTested:    rqs,
		TimeStart:         starttime,
		TestDuration:      cnf.DurationSeconds,
		VersionLabel:      cnf.HdrHistogramSettings.VersionLabel,
		PostmanCollection: postamnColl,
	}

	return pt
}

func NewClient(options ...Option) *TiedotClient {
	tc := &TiedotClient{}
	for _, o := range options {
		o(tc)
	}
	return tc
}

func WithHttpClient(c http.Client) Option {
	return func(t *TiedotClient) {
		t.HttpClient = c
	}
}

func WithDBHost(h string) Option {
	return func(t *TiedotClient) {
		t.DBHost = h
	}
}

func WithPerfTest(pt *PerfTest) Option {
	return func(t *TiedotClient) {
		t.PTest = pt
	}
}

func (t *TiedotClient) WriteTestEntryToDB() (string, error) {
	_url := fmt.Sprintf("%s/insert?col=%s", t.DBHost, "perf-tests")
	id, err := writeDoDB(_url, t.HttpClient, t.PTest)
	return id, err
}

func (t *TiedotClient) UpdatePerfTestRequestsData() error {
	update_url_ := fmt.Sprintf("%s/update?col=%s&id=%s", t.DBHost, "perf-tests", t.PerfTestDbId)
	_, err := writeDoDB(update_url_, t.HttpClient, t.PTest)
	return err
}

func writeDoDB(u string, c http.Client, pl *PerfTest) (string, error) {
	bd, e := json.Marshal(pl)
	if e != nil {
		log.Fatal("ERROR histogram data marshal error")
	}
	log.Println(string(bd))

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("doc", string(bd))
	err := writer.Close()
	if err != nil {
		fmt.Println(err)
	}

	// the data needs to be saved in the document db
	docReq, e := http.NewRequest(http.MethodPost, u, payload)
	if e != nil {
		log.Fatal("ERROR histogram data body reader create error")
	}
	docReq.Header.Set("Content-Type", "application/octet-stream")
	docReq.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := c.Do(docReq)
	if err != nil {
		log.Println(err.Error())
	}

	b, e := io.ReadAll(res.Body)
	if e != nil {
		log.Println("ERROR could not read body - missing ID for the histogram")
	}
	res.Body.Close()

	return string(b), err
}

package histograms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/openhistogram/circonusllhist"
)

// circllhist is a library, which geneates yet another dataset, with even more buckets.
// https://github.com/openhistogram/libcircllhist/
// The hitograms meant to be analysed after they have been written into files.
// They can be merged and aggregated with least effort and can give more granular view on the behaviour.

// doc db url
const (
	HISTOGRAM_DB_URL = "http://10.168.48.76:8000"
	COLLECTION       = "genghis2"
)

type OptionCircllhist func(*CirclhistRecorder)

type CircllHistData struct {
	Name string    `json:"name"`
	Data string    `json:"data"`
	Time time.Time `json:"timestamp"`
}

// CirclhistRecorder struct type
type CirclhistRecorder struct {
	Buff                 *bytes.Buffer
	Histograms           []*circonusllhist.Histogram
	HDRHistogramSettings []*CircllhistHistogramSettings
	OutputConf           *OutputSettings
}

type CircllhistHistogramSettings struct {
	Label string
	Hist  *circonusllhist.Histogram
}

func NewCircllhist(options ...OptionCircllhist) *CirclhistRecorder {
	buff := &bytes.Buffer{}
	hdr_recorder := &CirclhistRecorder{
		Buff: buff,
	}
	for _, o := range options {
		o(hdr_recorder)
	}
	return hdr_recorder
}

func WithCircllhistOutputSettings(out_conf *OutputSettings) OptionCircllhist {
	e := os.MkdirAll(out_conf.BasePath, 0777)
	if e != nil {
		log.Fatalf("ERROR: Histogram file could not been written. %s", e.Error())
	}

	return func(h *CirclhistRecorder) {
		h.OutputConf = out_conf
	}
}

func (hdr *CirclhistRecorder) AddCircllhistHistogram(hist *CircllhistHistogramSettings) *circonusllhist.Histogram {
	h := circonusllhist.New()
	hdr.Histograms = append(hdr.Histograms, h)
	hdr.HDRHistogramSettings = append(hdr.HDRHistogramSettings, hist)
	return h
}

func (hdr *CirclhistRecorder) GetCisrcllhistSettingsByLabel(label string) *CircllhistHistogramSettings {
	for _, s := range hdr.HDRHistogramSettings {
		if s.Label == label {

			return s
		}
	}
	return nil
}

func (hdr *CirclhistRecorder) WriteCircllhistHistogramOutputs() (map[string]string, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	dbClient := http.Client{
		Transport: t,
		Timeout:   time.Duration(10 * time.Second),
	}

	histograms_ids := map[string]string{}

	for _, hs := range hdr.HDRHistogramSettings {
		buf := bytes.Buffer{}
		e := hs.Hist.SerializeB64(&buf)
		if e != nil {
			panic(e)
		}

		// create data
		d := CircllHistData{
			Name: hs.Label,
			Data: buf.String(),
			Time: time.Now(),
		}
		bd, e := json.Marshal(d)
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
		_url := fmt.Sprintf("%s/insert?col=%s", HISTOGRAM_DB_URL, COLLECTION)
		docReq, e := http.NewRequest(http.MethodPost, _url, payload)
		if e != nil {
			log.Fatal("ERROR histogram data body reader create error")
		}
		docReq.Header.Set("Content-Type", "application/octet-stream")
		docReq.Header.Set("Content-Type", writer.FormDataContentType())
		res, err := dbClient.Do(docReq)
		if err != nil {
			log.Println(err.Error())
			return nil, err
		}
		b, e := io.ReadAll(res.Body)
		if e != nil {
			log.Println("ERROR could not read body - missing ID for the histogram")
		}
		histograms_ids[hs.Label] = string(b)
		res.Body.Close()

		time.Sleep(time.Duration(1000 * time.Millisecond))
	}
	return histograms_ids, nil
}

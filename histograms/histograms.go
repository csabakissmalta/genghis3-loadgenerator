package histograms

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

type Option func(*HDRRecorder)

type OutputSettings struct {
	BasePath string
	Version  string
}

type HDRRecorder struct {
	Buff                 *bytes.Buffer
	Writer               *hdrhistogram.HistogramLogWriter
	HDRHistograms        []*hdrhistogram.Histogram
	HDRHistogramSettings []*HistogramSettings
	OutputConf           *OutputSettings
}

type HistogramSettings struct {
	Label           string
	LowestDiscVal   int64
	HighestTrackVal int64
	NumSignifDigits int
	Hist            *hdrhistogram.Histogram
}

func NewHDRRecorder(option ...Option) *HDRRecorder {
	buff := &bytes.Buffer{}
	hdr_recorder := &HDRRecorder{
		Buff:   buff,
		Writer: hdrhistogram.NewHistogramLogWriter(buff),
	}

	hdr_recorder.Writer.OutputLogFormatVersion()
	hdr_recorder.Writer.OutputStartTime(0)
	hdr_recorder.Writer.OutputLegend()
	for _, o := range option {
		o(hdr_recorder)
	}
	return hdr_recorder
}

func WithOutputSettings(out_conf *OutputSettings) Option {
	e := os.MkdirAll(out_conf.BasePath, 0777)
	if e != nil {
		log.Fatalf("ERROR: Histogram file could not been written. %s", e.Error())
	}

	return func(h *HDRRecorder) {
		h.OutputConf = out_conf
	}
}

func (hdr *HDRRecorder) AddHistogram(hist *HistogramSettings) *hdrhistogram.Histogram {
	h := hdrhistogram.New(hist.LowestDiscVal, hist.HighestTrackVal, hist.NumSignifDigits)
	h.SetTag(hist.Label)
	hdr.HDRHistograms = append(hdr.HDRHistograms, h)
	hdr.HDRHistogramSettings = append(hdr.HDRHistogramSettings, hist)
	return h
}

func (hdr *HDRRecorder) GetSettingsByLabel(label string) *HistogramSettings {
	for _, s := range hdr.HDRHistogramSettings {
		if s.Label == label {
			return s
		}
	}
	return nil
}

func (hdr *HDRRecorder) WriteHistogramOutputs() error {
	for _, hs := range hdr.HDRHistogramSettings {
		hdr_out_file := fmt.Sprintf("%s/hdr_%s_%s.hgrm", hdr.OutputConf.BasePath, hs.Label, hdr.OutputConf.Version)
		file, err := os.OpenFile(hdr_out_file, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			return err
		}
		defer file.Close()
		w := bufio.NewWriter(file)
		hs.Hist.PercentilesPrint(w, 3, 0.99999)
		w.Flush()
		time.Sleep(time.Duration(1000 * time.Millisecond))
	}
	return nil
}

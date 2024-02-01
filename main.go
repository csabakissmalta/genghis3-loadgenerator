package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	engine "github.com/csabakissmalta/tpee/coil"
	execconf "github.com/csabakissmalta/tpee/exec"
	natsconnect "github.com/csabakissmalta/tpee/nats_connect"
	postman "github.com/csabakissmalta/tpee/postman"
	task "github.com/csabakissmalta/tpee/task"
	timeline "github.com/csabakissmalta/tpee/timeline"
	"github.com/nats-io/nats.go"

	histograms "github.com/csabakissmalta/genghis3-loadgen/histograms"
	msgpublisher "github.com/csabakissmalta/genghis3-loadgen/msg_publisher"
	"github.com/csabakissmalta/genghis3-loadgen/processers"
	"github.com/csabakissmalta/genghis3-loadgen/runtimeconfig"
	"github.com/csabakissmalta/genghis3-loadgen/tiedotclient"
	timeseriesreporting "github.com/csabakissmalta/genghis3-loadgen/timeseriesreporting"
)

// Engine instance
var runner *engine.Coil

// Collection
var postman_coll *postman.Postman

// Timelines
var timelines []*timeline.Timeline

// Exec Config
var exec_conf *execconf.Exec

// Reporting channel
var rep_chan chan *task.Task

// Influxdbapi
var influxdbapi *timeseriesreporting.InfluxDB
var influx_point_dispatcher *timeseriesreporting.PointDispatcher

// HDR Recorder
var hdr_recorder *histograms.HDRRecorder

// circllhist HDR recorder
var circllhistrec *histograms.CirclhistRecorder

// tiedot client
var tiedotClient *tiedotclient.TiedotClient

// ---
// Config files vars
// ---
// the file with the config
var exec_conf_file string

// the postman file to be loaded
var postman_coll_file string

// dty run - no data is written
var dry_run bool

// test db id
var tideout_test_db_id string

// transition time, when it is changed via NATS
var transition_time int64

// Define modes of operaion:
// 1. In GEN_MODE_LOAD_GENERATOR
// the app will have all features enabled by default
// 2. In GEN_FEED_GENERATOR
// no reporting modules should be anbled
// and publish should be set, to the defined NATS topic
const (
	GEN_MODE_LOAD_GENERATOR = "loadgenerator"
	GEN_FEED_GENERATOR      = "feedgenerator"
)

// runtime configs
var traffic_rate_configs *runtimeconfig.RuntimeConfig

var op_mode string = GEN_FEED_GENERATOR

func init() {
	runtime.GOMAXPROCS(8)
	debug.SetMaxThreads(20000)
	// Check flags
	flag.StringVar(&exec_conf_file, "execconf", "", "The execution configuration file.")
	flag.StringVar(&postman_coll_file, "postman", "", "The postman collection file. Has to be without folders.")
	flag.StringVar(&op_mode, "opmode", "loadgenerator", "Define modes of operaion: loadgenerator or feedgenerator")
	flag.Parse()

	if len(exec_conf_file) == 0 || len(postman_coll_file) == 0 {
		log.Fatalf("Argument ERROR: Both flags -execconf and -postman has to be set")
	}

	// Load execution config
	exec_conf = &execconf.Exec{}
	exec_conf.LoadExecConfig(exec_conf_file)

	// Load postman collection
	postman_coll = &postman.Postman{}
	pe := postman_coll.LoadCollection(postman_coll_file)
	if pe != nil {
		log.Println("LOAD POSTMAN ERR: ", pe.Error())
	}
	log.Println(postman_coll.Items)
	log.Println("------")

	// HDR
	if exec_conf.HdrHistogramSettings != nil && op_mode == GEN_MODE_LOAD_GENERATOR {
		hdr_recorder = histograms.NewHDRRecorder(
			histograms.WithOutputSettings(&histograms.OutputSettings{
				BasePath: exec_conf.HdrHistogramSettings.BaseOutPath,
				Version:  exec_conf.HdrHistogramSettings.VersionLabel,
			}),
		)

		circllhistrec = histograms.NewCircllhist(
			histograms.WithCircllhistOutputSettings(&histograms.OutputSettings{
				BasePath: exec_conf.HdrHistogramSettings.BaseOutPath,
				Version:  exec_conf.HdrHistogramSettings.VersionLabel,
			}),
		)

		trp := http.DefaultTransport.(*http.Transport).Clone()
		trp.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		tdc := http.Client{
			Transport: trp,
			Timeout:   time.Duration(60000 * time.Millisecond),
		}

		// perf test object
		pt_obj := tiedotclient.NewTest(time.Now(), postman_coll_file, exec_conf)

		// Histogram doc DB client
		tiedotClient = tiedotclient.NewClient(
			tiedotclient.WithDBHost("http://10.168.48.76:8000"),
			tiedotclient.WithHttpClient(tdc),
			tiedotclient.WithPerfTest(pt_obj),
		)
		tideout_test_db_id, tderr := tiedotClient.WriteTestEntryToDB()
		if tderr != nil {
			log.Fatal("ERROR tiedot client DB write error. Exiting.")
		}
		tiedotClient.PerfTestDbId = tideout_test_db_id
		log.Println("-------------")
		log.Println(tiedotClient)
		log.Println("-------------")
	}

	// INFLUXDB
	if exec_conf.InfluxdbSettings != nil && op_mode == GEN_MODE_LOAD_GENERATOR {
		influxdbapi = timeseriesreporting.NewInfluxdb(exec_conf.InfluxdbSettings)
		influx_point_dispatcher = timeseriesreporting.NewPointDispatcher(influxdbapi)
		influx_point_dispatcher.Start()
	}

}

func main() {
	// debug.SetMaxThreads(100000)
	runtime.GOMAXPROCS(runtime.NumCPU())

	// context with timeout stops execution, when the Done message is dispatched
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(exec_conf.DurationSeconds+*exec_conf.Rampup.DurationSeconds)*time.Second)

	var subs_channels map[string]chan *nats.Msg = make(map[string]chan *nats.Msg, len(exec_conf.NatsConnection.Subscription))

	var nats_client *natsconnect.NATSClient
	// var subs map[string]chan *nats.Msg
	if exec_conf.NatsConnection.NatsUrl != nil {
		log.Println("-------> NATS CLIENT LOAD GENERATOR", exec_conf.NatsConnection.NatsUrl)
		nats_client = natsconnect.New(
			natsconnect.WithConnectionUrl(*exec_conf.NatsConnection.NatsUrl),
			natsconnect.WithCredsFilePath(*exec_conf.NatsConnection.UserCredsFilePath),
			natsconnect.WithSubjects(exec_conf.NatsConnection.Subscription),
		)

		e := nats_client.Connect()
		if e != nil {
			log.Fatal("Cannot connect to NATS")
		}

		if op_mode == GEN_MODE_LOAD_GENERATOR && len(exec_conf.NatsConnection.Subscription) > 0 {
			for _, v := range exec_conf.NatsConnection.Subscription {
				sch := make(chan *nats.Msg, 100000)
				subs_channels[v] = sch
				nats_client.Conn.ChanSubscribe(v, subs_channels[v])
				log.Println("-------> NATS SUBSCRIPTION: ", subs_channels[v])
			}
		}

	}

	// Create timelines based on the config
	timelines = make([]*timeline.Timeline, len(exec_conf.Requests))

	for i := 0; i < len(exec_conf.Requests); i++ {
		req_rules := exec_conf.Requests[i]

		t := http.DefaultTransport.(*http.Transport).Clone()
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

		var client *http.Client
		if !req_rules.FollowRedirects {
			client = &http.Client{
				Transport: t,
				Timeout:   time.Duration(600000 * time.Millisecond),
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
		} else {
			client = &http.Client{
				Transport: t,
				Timeout:   time.Duration(600000 * time.Millisecond),
			}
		}

		tmln := timeline.New(
			timeline.WithRules(req_rules),
			timeline.WithHTTPClient(client),
			timeline.WithName(req_rules.Name),
		)
		log.Println(req_rules.Name)
		postmanReq, e := postman_coll.GetRequestByName(req_rules.Name)
		if e != nil {
			log.Printf("REQUEST DEFINITION ERROR: %s", e.Error())
			continue
		}
		log.Println("======== NATS CLIENT: ", subs_channels)
		tmln.Populate(exec_conf.DurationSeconds, postmanReq, exec_conf.Environment, exec_conf.Rampup, subs_channels)
		log.Println("... TIMELINE POPULATED...", req_rules.Name)
		timelines[i] = tmln
		log.Println(timelines)

		if hdr_recorder != nil && !dry_run {
			// Add a new histogram to the recorder
			hsetting := &histograms.HistogramSettings{
				Label:           tmln.Name,
				LowestDiscVal:   1,
				HighestTrackVal: int64(exec_conf.DurationSeconds) * int64(time.Millisecond),
				NumSignifDigits: 3,
			}
			h := hdr_recorder.AddHistogram(hsetting)
			hsetting.Hist = h
		}

		if circllhistrec != nil && !dry_run {
			c_hsetting := &histograms.CircllhistHistogramSettings{
				Label: tmln.Name,
			}
			h := circllhistrec.AddCircllhistHistogram(c_hsetting)
			c_hsetting.Hist = h
		}
	}

	// Reporting channel
	rep_chan = make(chan *task.Task, 100000)

	// Create engine
	runner = engine.New(
		engine.WithContext(ctx),
		engine.WithTimelines(timelines),
		engine.WithEnvVariables(exec_conf.Environment),
		engine.WithResultsReportingChannel(rep_chan),
		engine.WithExecutionMode(engine.GO_TIMER_MODE),
		engine.WithNATSClient(nats_client),
		// engine.WithExecutionMode(engine.COMPARE_TIMESTAMPS_MODE),
	)

	if op_mode == GEN_MODE_LOAD_GENERATOR {
		go func() {
			// ---- RUNTIME CONFIGS ----
			traffic_rate_configs = runtimeconfig.NeWithName(
				"plab",
				runtimeconfig.WithTrafficRatesConfigs(timelines, nats_client, ctx),
			)
			time.Sleep(1 * time.Second)
			for {
				change := <-traffic_rate_configs.UpdatesChannel

				if change != nil {
					log.Println("REV: ", change.Revision())
					if change.Revision() > 1 {

						log.Println("\n\n------------------------")
						log.Println(change.Bucket())
						log.Println(change.Key(), ": ", string(change.Value()))
						log.Println("REVISION: ", change.Revision())
						log.Println("-----------------------\n\n-")

						// get default values for the target rate and transition time

						// ---- create transition and repopulate the timeline
						transition_time_getval, _ := traffic_rate_configs.KVStore.Get(ctx, "transitiontime")
						transition_time, _ = strconv.ParseInt(string(transition_time_getval.Value()), 10, 64)

						switch change.Key() {
						case "transitiontime":
							transition_time, _ = strconv.ParseInt(string(change.Value()), 10, 64)
						default:
							targetrate_val, _ := strconv.ParseInt(string(change.Value()), 10, 64)
							trans := &timeline.Transition{
								TargetRate:                  int(targetrate_val),
								TransitionRampupTimeSeconds: int(transition_time),
								RampupType:                  string(timeline.SINUSOIDAL),
							}

							runner.UpdateTrafficRateFromNATSKVUpdate(change.Key(), trans, exec_conf.DurationSeconds)
						}

					}
				}
				time.Sleep(1 * time.Second)
			}
		}()
	}

	go func() {
		for {
			<-runner.Ctx.Done()
			if hdr_recorder != nil && !dry_run {
				e := hdr_recorder.WriteHistogramOutputs()
				if e != nil {
					log.Fatalf("HDR WRITE ERROR: %s", e.Error())
				}
			}

			if circllhistrec != nil && !dry_run {
				histids, e := circllhistrec.WriteCircllhistHistogramOutputs()
				if e != nil {
					log.Fatalf("HDR WRITE ERROR: %s", e.Error())
				}

				for name, id := range histids {
					for _, rt := range tiedotClient.PTest.RequestsTested {
						if name == rt.Name {
							rt.TiedotDbId = id
						}
					}
				}
				tiedotClient.UpdatePerfTestRequestsData()
			}
			os.Exit(0)
		}
	}()

	if rep_chan != nil {

		go func() {
			var feed_publisher *msgpublisher.MsgPublisher
			if op_mode == GEN_FEED_GENERATOR {
				log.Println("PREP DATA TO PUBLISH")

				// PLAB-1349 -- the processer is task-specific
				bbprov := processers.BetbuilderTestMsgProcesser{}

				feed_publisher = msgpublisher.New(
					msgpublisher.WithNATSClient(nats_client),
					msgpublisher.WithProcesser(bbprov),
				)
			}

			for {
				if rep_chan != nil {
					t := <-rep_chan

					if op_mode == GEN_FEED_GENERATOR {
						feed_publisher.Publish(t)
					}

					if influxdbapi != nil && op_mode == GEN_MODE_LOAD_GENERATOR {
						// write point to influxdb
						go func() {
							tags := map[string]string{"name": t.TaskLabel}
							fields := map[string]interface{}{"status_code": t.Response.StatusCode, "latency": t.ResponseTime}
							ipt := timeseriesreporting.NewPt(influxdbapi.Measurement, tags, fields, t.ExecutionTime)
							influx_point_dispatcher.Queue <- &timeseriesreporting.WriteTask{
								DataPoint: ipt,
							}
						}()
					}

					if hdr_recorder != nil && op_mode == GEN_MODE_LOAD_GENERATOR {
						// record a value to histogram
						go func() {
							hst := hdr_recorder.GetSettingsByLabel(t.TaskLabel)
							if hst != nil && t != nil {
								e := hst.Hist.RecordValue(t.ResponseTime)
								if e != nil {
									log.Printf("HDR ERROR: %s", e.Error())
								}
							} else {
								log.Printf("hist %s is nil", t.TaskLabel)
							}
						}()
					}

					if circllhistrec != nil && op_mode == GEN_MODE_LOAD_GENERATOR {
						go func() {
							hst := circllhistrec.GetCisrcllhistSettingsByLabel(t.TaskLabel)
							if hst != nil && t != nil {
								e := hst.Hist.RecordValue(float64(t.ResponseTime))
								if e != nil {
									log.Printf("HDR ERROR: %s", e.Error())
								}
							} else {
								log.Printf("hist %s is nil", t.TaskLabel)
							}
						}()
					}

					// b, e := io.ReadAll(t.Response.Body)
					// if e != nil {
					// 	log.Println("ERR cannot read response body")
					// }

					log.Println(
						"\n- - - - - - - - -> TL:", t.TaskLabel,
						"\nRESP_TIME:", t.ResponseTime,
						"ms\nSTATUS_CODE:", t.Response.StatusCode,
						"\n:?: URL:", t.Request.URL,
					)

					// if t.Response.StatusCode < 400 {
					// 	log.Println("\nBODY:", string(b))
					// 	t.Response.Body.Close()
					// }

				}
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		go func() {
			<-c
			// cleanup and write histogram files before quit

			os.Exit(1)
		}()
	}()

	// start the coil
	runner.Start()

	// Finish execution and exit
	runtime.Goexit()
}

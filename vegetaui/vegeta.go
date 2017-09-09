package main

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	vegeta "github.com/tsenart/vegeta/lib"
)

// Constants section
const (

	// Load test status constants
	AttackStatusCodeNotStarted = 0
	AttackStatusMsgNotStarted  = "NotStarted"

	AttackStatusCodeInProgress = 1
	AttackStatusMsgInProgress  = "InProgress"

	AttackStatusCodeCompleted = 2
	AttackStatusMsgCompleted  = "Completed"

	AttackStatusCodeCancelled = 3
	AttackStatusMsgCancelled  = "Cancelled"
)

// vegetaAttack holds the initial parameters and the state of a Vegeta load test
// attack. A global *vegetaAttack variable should be used, to ensure only one
// load test is to to be performed at one given time.
type vegetaAttack struct {
	sync.Mutex // generic, multi-purpose mutex for the instance

	// status indicates the current status of the attack. Its value could be one
	// of the AttackStatusCodeNotStarted, AttackStatusCodeInProgress,
	// AttackStatusCodeCompleted or AttackStatusCodeCancelled constants.
	// To prevent races, you must use the setStatus and getStatus method instead of
	// accessing the status directly.
	status      int
	statusMutex sync.Mutex // dedicated mutex for the status

	elapsed uint32

	// Number of requests completed so far. An atomic value, incremented whenever
	// new values are read from the Vegeta attack channel
	CompletedRequests uint32

	// Total requests expected to complete in the current load test
	TotalRequests uint32

	// The Vegeta Metrics pointer. If Status is 0, and Metrics is not nil, it means a previous test
	// has completed, and the Metrics hold the results inside.
	// If Status is 0, and Metrics is nil, it means that no previous load tests have been performed
	Metrics *vegeta.Metrics

	// Allow the user to cancel the test
	Cancel          chan bool
	CurrentAttacker *vegeta.Attacker

	// When the test completes, send the load test structure via this channel
	FinalResults chan LoadTestStatus

	// Inbound messages from the connections.
	Incoming chan []byte

	// Register requests from the connections.
	Register chan *WsConnection

	// Unregister requests from connections.
	Unregister chan *WsConnection

	// Registered connections
	Connections map[*WsConnection]bool

	// Holds the most recent set of results
	CurrentResults vegeta.Results

	// Hold the most recent load status with the metrics
	FinalMetrics LoadTestStatus
}

func (v *vegetaAttack) getStatus() int {
	var status int
	v.statusMutex.Lock()
	status = v.status
	v.statusMutex.Unlock()
	return status
}

func (v *vegetaAttack) setStatus(newStatus int) {
	v.statusMutex.Lock()
	v.status = newStatus
	v.statusMutex.Unlock()
}

func (v *vegetaAttack) getStatusMsg(statusCode int) string {
	if statusCode == AttackStatusCodeNotStarted {
		return AttackStatusMsgNotStarted
	} else if statusCode == AttackStatusCodeInProgress {
		return AttackStatusMsgInProgress
	} else if statusCode == AttackStatusCodeCompleted {
		return AttackStatusMsgCompleted
	} else if statusCode == AttackStatusCodeCancelled {
		return AttackStatusMsgCancelled
	} else {
		return AttackStatusMsgNotStarted
	}
}

func (v *vegetaAttack) getCurrentAttacker() *vegeta.Attacker {
	var attackerCopy *vegeta.Attacker
	instance.Lock()
	attackerCopy = instance.CurrentAttacker
	instance.Unlock()
	return attackerCopy
}

func (v *vegetaAttack) setCurrentAttacker(vegetaAttacker *vegeta.Attacker) {
	instance.Lock()
	instance.CurrentAttacker = vegetaAttacker
	instance.Unlock()
}

func (v *vegetaAttack) getLatestVegetaMetrics() LoadTestStatus {
	var metricsCopy LoadTestStatus
	instance.Lock()
	metricsCopy = instance.FinalMetrics
	instance.Unlock()
	return metricsCopy
}

func (v *vegetaAttack) setLatestVegetaMetrics(latestMetrics LoadTestStatus) {
	instance.Lock()
	instance.FinalMetrics = latestMetrics
	instance.Unlock()
}

func (v *vegetaAttack) getCurrentResults() vegeta.Results {
	var resultsCopy vegeta.Results
	instance.Lock()
	resultsCopy = append(resultsCopy, instance.CurrentResults...)
	instance.Unlock()
	return resultsCopy
}

func (v *vegetaAttack) setCurrentResults(latestResults vegeta.Results) {

	instance.Lock()
	instance.CurrentResults = nil
	instance.CurrentResults = append(instance.CurrentResults, latestResults...)
	instance.Unlock()

}

// Single instance of the vegetaAttack structure
var instance = &vegetaAttack{
	status:       0,
	Cancel:       make(chan bool),
	FinalResults: make(chan LoadTestStatus),
	Incoming:     make(chan []byte),
	Register:     make(chan *WsConnection),
	Unregister:   make(chan *WsConnection),
	Connections:  make(map[*WsConnection]bool),
}

// startAttack initiates the Vegeta attack if no other attack is ongoing at that time.
func startAttack(targetURL string, attackMethod string, body []byte, requestsPerSecond int,
	duration int, timeout int, workers int, differentConnections bool, ignoreInvalidCert bool) {

	currentStatus := instance.getStatus()

	// Prevent execution of than one test attack at any given time
	if currentStatus == AttackStatusCodeInProgress {
		return
	}

	// When done, indicate that the load test completed so it can be resumed.
	defer instance.setStatus(AttackStatusCodeCompleted)

	// Set status to "In Progress", to avoid multiple tests at once
	// and reset the elapsed to 0
	instance.setStatus(AttackStatusCodeInProgress)
	atomic.StoreUint32(&instance.elapsed, 0)

	// also reset the completed requests
	atomic.StoreUint32(&instance.CompletedRequests, 0)

	// set the total requests expected to complete
	atomic.StoreUint32(&instance.TotalRequests, uint32(requestsPerSecond*duration))

	debugInfo := `
		****************************************************
			About to start Vegeta attack on:
			Target URL: ` + targetURL + `
			Method: ` + attackMethod + `
			Body: ` + string(body) + `
			Requests per second: ` + strconv.Itoa(requestsPerSecond) + `
			Duration: ` + strconv.Itoa(duration) + `
			Timeout: ` + strconv.Itoa(timeout) + `
			CPU Workers: ` + strconv.Itoa(workers) + `
			Keep-alive: ` + strconv.FormatBool(!differentConnections) + `
			Ignore invalid cert: ` + strconv.FormatBool(ignoreInvalidCert) + `
		****************************************************`
	log.Println(debugInfo)

	rate := uint64(requestsPerSecond) // per second
	durationTime := time.Duration(duration) * time.Second

	targeter := vegeta.NewStaticTargeter(vegeta.Target{
		Method: attackMethod,
		URL:    targetURL,
		Body:   body,
	})

	// set up the tls config to ignore invalid certificates if applicable
	c := &tls.Config{InsecureSkipVerify: ignoreInvalidCert}

	attacker := vegeta.NewAttacker(
		vegeta.Timeout(time.Duration(timeout)*time.Second),
		vegeta.Workers(uint64(workers)),
		vegeta.KeepAlive(!differentConnections),
		vegeta.TLSConfig(c))

	// initialize a new Metrics structures, for this session
	if instance.Metrics != nil {
		instance.Metrics = nil
	}
	instance.Metrics = &vegeta.Metrics{}

	instance.setCurrentAttacker(attacker)

	attackChannel := attacker.Attack(targeter, rate, durationTime)

	// results slice to store all the results to subsequently generate
	// the chart
	var results vegeta.Results = make(vegeta.Results, 0, requestsPerSecond*duration)

	for res := range attackChannel {
		instance.Metrics.Add(res)
		results = append(results, *res)
		atomic.StoreUint32(&instance.CompletedRequests, uint32(instance.Metrics.Requests))
	}

	log.Println("Attack ended. Closing the metrics and setting the results.")

	// If status is still "In progress", set it as "Completed" here
	status := instance.getStatus()
	if status == AttackStatusCodeInProgress {
		instance.setStatus(AttackStatusCodeCompleted)
	}

	instance.Metrics.Close()
	instance.setCurrentResults(results)

	finalLoadTestStatus := LoadTestStatus{
		Status:            instance.getStatusMsg(instance.getStatus()),
		Elapsed:           uint32((instance.Metrics.Duration / time.Second) * 10),
		TotalRequests:     uint32(uint32(requestsPerSecond * duration)),
		RequestsCompleted: uint32(instance.Metrics.Requests),

		SuccessRate: strconv.FormatFloat((instance.Metrics.Success * 100), 'f', 2, 64),

		P50: uint32(instance.Metrics.Latencies.P50 / time.Millisecond),
		P95: uint32(instance.Metrics.Latencies.P95 / time.Millisecond),
		P99: uint32(instance.Metrics.Latencies.P99 / time.Millisecond),
	}

	// persist it in the singleton
	instance.setLatestVegetaMetrics(finalLoadTestStatus)

	// send the final results via the channel
	instance.FinalResults <- finalLoadTestStatus
}

func WriteVegetaPlotResults(writer io.Writer, results *vegeta.Results) error {

	vegetaReporter := vegeta.NewPlotReporter("Vegeta Load Test Plot Results", results)
	return vegetaReporter.Report(writer)
}

func WriteVegetaHistogramResults(writer io.Writer, results *vegeta.Results) error {

	histogram := vegeta.Histogram{
		Buckets: []time.Duration{
			0,
			10 * time.Millisecond,
			25 * time.Millisecond,
			50 * time.Millisecond,
			100 * time.Millisecond,
			1000 * time.Millisecond,
			2000 * time.Millisecond,
			4000 * time.Millisecond,
		},
	}

	for _, r := range *results {

		result := r

		histogram.Add(&result)
	}

	vegetaReporter := vegeta.NewHistogramReporter(&histogram)

	return vegetaReporter.Report(writer)
}

func initVegeta() {
	log.Println("Starting the Vegeta UI Background Process...")

	// start a ticker and update the tenth of a second in a
	// separate go routine
	attackTicker := time.NewTicker(time.Millisecond * 100)

	go func() {
		defer attackTicker.Stop()

		for {
			select {
			case _, ok := <-attackTicker.C:

				currentStatus := instance.getStatus()

				// If the attack is in progress, the tick will be used to retrieve
				// the attack's progress (requests completed versus total requests)
				// and to push it to the websocket clients
				if currentStatus == AttackStatusCodeInProgress {

					newValue := atomic.AddUint32(&instance.elapsed, 1)
					status := AttackStatusCodeInProgress

					if !ok {
						// The ticker channel has been closed, which means that
						// the load test is over
						status = AttackStatusCodeCompleted
					}

					completedRequests := atomic.LoadUint32(&instance.CompletedRequests)
					totalRequests := atomic.LoadUint32(&instance.TotalRequests)

					// construct a new status struct
					newLoadTestStatus := LoadTestStatus{
						Elapsed:           newValue,
						RequestsCompleted: completedRequests,
						TotalRequests:     totalRequests,
						Status:            instance.getStatusMsg(status),
					}

					// send the test status to all subscribed client websockets
					if err := publishStatusToOpenSockets(newLoadTestStatus); err != nil {
						log.Println("publishStatusToOpenSockets error: ", err)
						return
					}
				}

			// when the load test finishes, the final results arrive via a dedicated channel
			case results := <-instance.FinalResults:
				// send the final test results to all subscribed client websockets
				if err := publishStatusToOpenSockets(results); err != nil {
					log.Println("Final Results publishStatusToOpenSockets error: ", err)
					return
				}

			// when the cancel signal arrives, it should contain a pointer to
			// the attacker itself, and we'll call the Stop method
			case cancelOrder, ok := <-instance.Cancel:
				// instruct the attacker to stop
				if ok && cancelOrder {
					log.Println("entering cancel")
					if instance.getStatus() == AttackStatusCodeInProgress {
						instance.setStatus(AttackStatusCodeCancelled)
						instance.getCurrentAttacker().Stop()
					}
				}

			// When a connection arrives via the Register channel, add it to the
			// open connections map.
			case c := <-instance.Register:
				instance.Connections[c] = true

			// When a connection arrives via the Unregister channel, remove it from
			// the open connections map.
			case c := <-instance.Unregister:
				if _, ok := instance.Connections[c]; ok {
					delete(instance.Connections, c)
					close(c.Send)
				}

			// When a message is arriving in the incoming channel, try to send it to all
			// the websocket clients registered with the registry.
			case m := <-instance.Incoming:

				// ignore the control messages, do not resend them to clients
				if !(string(m) == MessageCancelTest || string(m) == MessageGetTestStatus) {

					for c := range instance.Connections {
						select {
						case c.Send <- m:
						default:
							close(c.Send)
							delete(instance.Connections, c)
						}
					}
				} else {
					if string(m) == MessageCancelTest {
						// Send the cancel signal from a different go routine to avoid
						// a deadlock with the case waiting for the cancelChan value.
						go func(cancelChan chan bool) { cancelChan <- true }(instance.Cancel)
					}
				}
			} // end select
		}
	}() // end go func
} // end initVegeta()

// Pushes the new status to the write pump for the open websocket connections
// to receive it
func publishStatusToOpenSockets(newStatus LoadTestStatus) error {

	// convert the status struct to json
	var content []byte
	var err error
	content, err = json.MarshalIndent(&newStatus, "", "  ")
	if err != nil {
		return err
	}

	for c := range instance.Connections {
		select {
		case c.Send <- content:
		default:
			// If unable to send, remove this connection.
			close(c.Send)
			delete(instance.Connections, c)
		}
	}
	return nil
}

// LoadTestStatus holds data to be serialized and passed via websockets to inform the
// web clients of state of the attack.
type LoadTestStatus struct {

	// Status can be "InProgress", "NotStarted" or "Completed"
	Status string `json:"status"`

	// Time elapsed in ticks of 100ms each. Thus, a value of 14 means 1400ms have passed since the test started
	Elapsed uint32 `json:"elapsed"`

	// Number of requests that are completed so far
	RequestsCompleted uint32 `json:"requestsCompleted"`

	// Total number of requests in the test
	TotalRequests uint32 `json:"totalRequests"`

	// Percentage of successful requests
	SuccessRate string `json:"successRate"`

	// Final latencies stats when test is complete

	P50 uint32 `json:"p50"`
	P95 uint32 `json:"p95"`
	P99 uint32 `json:"p99"`
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

/*
routes.go: Contains the http handlers for various routes
(home page, start load test, get status, etc)
*/

const (
	contentType        = "Content-Type"
	ResponseStatus_OK  = 0
	ResponseStatus_ERR = -1
)

// HomeHandler handles the default root path and renders the main page template
func HomeHandler(w http.ResponseWriter, r *http.Request) {

	// Get the index (home) page. Let's not worry about caching it in memory
	// since it's a single page app
	homeTemplate := *static + "/index.html"
	c, err := ioutil.ReadFile(homeTemplate)
	if err != nil {
		log.Fatal("Error reading the index page: ", err)
	}

	tmpl, err := template.New("homeTemplate").Parse(string(c))
	if err != nil {
		log.Fatal("Error running template.New for the index page:", err)
	}

	previousResults := instance.getCurrentResults()
	hasResultsAlready := (previousResults != nil)

	var previousMetrics LoadTestStatus
	if previousResults != nil {
		previousMetrics = instance.getLatestVegetaMetrics()
	}

	pageStateValues := struct {
		HasResultsAlready bool
		Host              string
		PreviousResults   LoadTestStatus
	}{
		HasResultsAlready: hasResultsAlready,
		Host:              host,
		PreviousResults:   previousMetrics,
	}

	var templBuf bytes.Buffer
	err = tmpl.Execute(&templBuf, pageStateValues)
	if err != nil {
		log.Fatal("Error running template.Execute for the index page:", err)
	}

	fmt.Fprint(w, templBuf.String())
}

// StartAttackHandler handles the request to start a Vegeta load test
// if no such test is currently running.
func StartAttackHandler(w http.ResponseWriter, r *http.Request) {

	var statusCode = 0
	var statusMsg = "OK"

	var testInProgress = false

	// Check whether a test is already in progress, to prevent execution
	// of more than one test attack at any given time.
	if instance.getStatus() == AttackStatusCodeInProgress {
		testInProgress = true
		statusMsg = "Test Already Running"
	}

	validationErrors := make([]string, 0)
	// only bother doing validation if there is no other test currently running
	if testInProgress == false {
		log.Println("StartAttackHandler: Preparing to validate input params")

		targetURL, err := getString(r, "targetURL", true)
		if err != nil {
			validationErrors = append(validationErrors, "Empty value for the targetURL parameter")
		}

		attackMethod, err := getString(r, "attackMethod", false)
		if attackMethod == "" {
			attackMethod = "GET"
		}

		bodyParams, err := getString(r, "bodyParams", false)
		var bParams []byte
		if bodyParams != "" {
			bParams = []byte(bodyParams)
		}

		requestsPerSecond, err := getInt(r, "requestsPerSecond", true)
		if err != nil {
			validationErrors = append(validationErrors, "Invalid or empty value for the requestsPerSecond parameter")
		}

		duration, err := getInt(r, "duration", true)
		if err != nil {
			validationErrors = append(validationErrors, "Invalid or empty value for the duration parameter")
		}

		requestTimeout, err := getInt(r, "requestTimeout", true)
		if err != nil {
			validationErrors = append(validationErrors, "Invalid or empty value for the requestTimeout parameter")
		}

		workers, err := getInt(r, "workers", true)
		if err != nil {
			validationErrors = append(validationErrors, "Invalid or empty value for the workers parameter")
		}

		if len(validationErrors) == 0 {

			// get the checkbox params

			// ignore invalid certs
			ignoreInvalidCertificate, err := getString(r, "ignoreInvalidCert", false)
			if err != nil {
				log.Println("Error retrieving request parameter ignoreInvalidCert: ", err)
			}

			// keep-alive false
			differentConnections, err := getString(r, "differentConnections", false)
			if err != nil {
				log.Println("Error retrieving request parameter differentConnections: ", err)
			}

			// we need to initiate the Vegeta attack into a different subroutine
			go startAttack(targetURL, attackMethod, bParams, requestsPerSecond, duration,
				requestTimeout, workers,
				(differentConnections == "y" || differentConnections == "Y"),
				(ignoreInvalidCertificate == "y" || ignoreInvalidCertificate == "Y"))
		} else {

			statusCode = -10
			statusMsg = "Validation Errors"
		}

	}

	// Serve the response as JSON
	serveJSON(w, statusCode, statusMsg, validationErrors, nil)
}

// GetVegetaResults renders the latest results
func GetVegetaResults(w http.ResponseWriter, r *http.Request) {

	// get the latest results
	latestVegetaResults := instance.getCurrentResults()

	if latestVegetaResults == nil {
		w.Write([]byte(NoResultsYet))
		return
	}

	err := WriteVegetaPlotResults(w, &latestVegetaResults)
	if err != nil {
		log.Println("WriteVegetaResults vegetaReporter.Report Error:", err)
	}
}

// GetVegetaHistogram renders the Vegeta histogram
func GetVegetaHistogram(w http.ResponseWriter, r *http.Request) {

	// get the latest results
	latestVegetaResults := instance.getCurrentResults()

	if latestVegetaResults == nil {
		w.Write([]byte("[no results found for the histogram]"))
		return
	}

	err := WriteVegetaHistogramResults(w, &latestVegetaResults)
	if err != nil {
		log.Println("GetVegetaHistogram vegetaReporter.Report Error", err)
	}
}

const NoResultsYet = `<html>
<head>
	<title>Vegeta Load Test - No Results Available So Far</title>
</head>
<body>
	<h1>No Results Found</h1>
	<p>It appears that no results have been found. Please ensure at least one test has been run.</p>
</html>
`

func serveJSON(w http.ResponseWriter, responseStatusCode int, responseMessage string, errors []string, data interface{}) error {
	w.Header().Set(contentType, "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	wrapper := struct {
		Status  int
		Message string
		Errors  []string    `json:"Errors,omitempty"`
		Data    interface{} `json:"Data,omitempty"`
	}{
		Status:  responseStatusCode,
		Message: responseMessage,
		Errors:  errors,
		Data:    data,
	}

	unencodedJson := &bytes.Buffer{}
	if err := json.NewEncoder(unencodedJson).Encode(&wrapper); err != nil {
		return err
	}

	w.Write(unencodedJson.Bytes())
	return nil
}

func getString(r *http.Request, paramName string, mandatory bool) (string, error) {

	paramVal := r.FormValue(paramName)
	if mandatory && paramVal == "" {
		return "", fmt.Errorf("getString(): mandatory param %s is missing from the request", paramName)
	}
	return paramVal, nil
}

func getInt(r *http.Request, paramName string, mandatory bool) (int, error) {

	paramVal := r.FormValue(paramName)
	if mandatory && paramVal == "" {
		return 0, fmt.Errorf("getInt(): mandatory param %s is missing from the request", paramName)
	}
	return strconv.Atoi(paramVal)
}

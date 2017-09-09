package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// Command line flags
var addr = flag.String("addr", "localhost", "the address or hostname the http server should listen on. Defaults to localhost")
var port = flag.Int("port", 8007, "the port the http server should listen on. Defaults to 8007")
var static = flag.String("static", ".", "the directory to serve static files from. Defaults to the current dir")

var host = ""

func main() {
	flag.Parse()
	host = *addr + ":" + strconv.Itoa(*port)

	// Initialize the Vegeta engine instance
	initVegeta()

	r := mux.NewRouter()

	r.HandleFunc("/", HomeHandler)
	r.HandleFunc("/start", StartAttackHandler)
	r.HandleFunc("/status", GetAttackStatus)
	r.HandleFunc("/results", GetVegetaResults)
	r.HandleFunc("/histogram", GetVegetaHistogram)

	// This will serve files under http://localhost:8000/static/<filename>
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(*static))))

	srv := &http.Server{
		Handler:      r,
		Addr:         host,
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

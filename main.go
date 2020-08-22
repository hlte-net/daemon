package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var writeChan = make(chan inputType)
var formatChan = make(chan writeFormatMutate)

func main() {
	configPath := flag.String("config", defaultConfigPath, "config file path")
	flag.Parse()

	var config Config
	if err := ParseJSON(*configPath, &config); err != nil {
		log.Panicf("failed to parse '%s': %v", *configPath, err)
	}

	ldPath, err := LocalDataPath(dataPathEnvVarName)

	if err != nil {
		log.Panicf("local data path %v", err)
	}

	localDataPath, err := InitLocalData(ldPath)

	if err != nil {
		log.Panicf("init data path %v", err)
	}

	log.Printf("using local data path '%s'", localDataPath)

	http.HandleFunc("/version", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, config.Version)
	})

	http.HandleFunc("/formats", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" {
			jsonBytes, err := json.Marshal(validFormats())

			if err != nil {
				log.Panic("validFormats to json")
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, string(jsonBytes))
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var bodyBytes []byte
		var err error

		if bodyBytes, err = ioutil.ReadAll(req.Body); err != nil {
			log.Printf("main body read failed\n%v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if len(bodyBytes) > 0 {
			var bodyObj ingestType

			if err := json.Unmarshal(bodyBytes, &bodyObj); err != nil {
				log.Printf("main json parse failed\n%v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			writeChan <- inputType{bodyObj, time.Now().UnixNano()}
			w.WriteHeader(http.StatusOK)
		}
	})

	go writersHandler(writeChan)
	go formatsMutator(formatChan, localDataPath)

	// enable all validFormats()
	for _, formatName := range validFormats() {
		newWriteChan := make(chan inputType)
		formatChan <- writeFormatMutate{writeFormat{formatName, newWriteChan}, true}
		log.Printf("format '%s' available", formatName)
	}

	listenSpec := fmt.Sprintf("%s:%d", config.Host, config.Port)
	log.Printf("listening on %s\n", listenSpec)
	http.ListenAndServe(listenSpec, nil)
}

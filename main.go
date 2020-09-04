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
	log.Printf("version %s", version)

	var err error
	var config Config
	var ldPath string

	configPath := flag.String("config", defaultConfigPath, "config file path")
	flag.Parse()

	if err := ParseJSON(*configPath, &config); err != nil {
		log.Panicf("failed to parse '%s': %v", *configPath, err)
	}

	if config.LocalDataPath != "" {
		ldPath = config.LocalDataPath
	} else {
		ldPath, err = LocalDataPath(dataPathEnvVarName)

		if err != nil {
			log.Panicf("local data path %v", err)
		}
	}

	localDataPath, err := InitLocalData(ldPath)

	if err != nil {
		log.Panicf("init data path %v", err)
	}

	log.Printf("using local data path '%s'", localDataPath)

	hdrs := func(w http.ResponseWriter) {
		w.Header().Set("Access-Control-Allow-Headers", ppHeader)
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	authCheck := func(w http.ResponseWriter, req *http.Request) (bool, error) {
		hdrs(w)

		if req.Method == "OPTIONS" {
			return true, fmt.Errorf("not-an-error")
		}

		authed := false
		reqPP, ppHeaderFound := req.Header[ppHeader]

		if len(config.PassphraseSha512) > 0 {
			authed = ppHeaderFound && len(reqPP) == 1 && reqPP[0] == config.PassphraseSha512
		} else {
			authed = !ppHeaderFound
		}

		if !authed {
			log.Printf("WARN: %s attempted unauthorized call to '%s' %s (headers: %v)",
				req.RemoteAddr, req.RequestURI, req.Method, req.Header)
			w.WriteHeader(http.StatusForbidden)
			return false, nil
		}

		return true, nil
	}

	http.HandleFunc("/version", func(w http.ResponseWriter, req *http.Request) {
		if authed, err := authCheck(w, req); !authed || err != nil {
			return
		}

		w.Header().Set("Content-type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, version)
	})

	http.HandleFunc("/formats", func(w http.ResponseWriter, req *http.Request) {
		if authed, err := authCheck(w, req); !authed || err != nil {
			return
		}

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
		if authed, err := authCheck(w, req); !authed || err != nil {
			return
		}

		if req.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var bodyBytes []byte

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

	if config.PassphraseSha512 != "" {
		log.Printf("passphrase is set")
	}

	listenSpec := fmt.Sprintf("%s:%d", config.Host, config.Port)
	log.Printf("listening on %s", listenSpec)
	http.ListenAndServe(listenSpec, nil)
}

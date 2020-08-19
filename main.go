package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultListenHost = "localhost"
const defaultListenPort = 56555
const version = "20200818"
const dataPathEnvVarName = "HLTE_DAEMON_DATA_PATH"

type payloadType struct {
	Data string
	URI  string
}

type inputType struct {
	Checksum string
	Payload  payloadType
}

type writeFormat struct {
	name  string
	write chan inputType
}

type writeFormatMutate struct {
	format  writeFormat
	enabled bool
}

type formatHandlerFunc func(chan inputType, string)

var writeChan = make(chan inputType)
var formatChan = make(chan writeFormatMutate)

var validFormats = []string{"json", "csv"} //TODO configurable!
var formatHandlers = map[string]formatHandlerFunc{
	"json": func(jsonChannel chan inputType, localDataPath string) {
		for w := range jsonChannel {
			if jsonBytes, err := json.Marshal(w); err == nil {
				fileName := fmt.Sprintf("%s/%d.json", localDataPath, time.Now().UnixNano())

				if file, err := os.Create(fileName); err == nil {
					if _, err = file.Write(jsonBytes); err != nil {
						log.Printf("ERROR: failed to write '%s': %v", fileName, err)
					}
				}
			}
		}
	},
	"csv": func(csvChannel chan inputType, localDataPath string) {
		csvFile, err := os.OpenFile(fmt.Sprintf("%s/data.csv", localDataPath),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)

		if err != nil {
			log.Panicf("csv open: %v", err)
		}

		csvWriter := csv.NewWriter(csvFile)

		for w := range csvChannel {
			csvWriter.Write([]string{w.Checksum, w.Payload.URI, w.Payload.Data})
			csvWriter.Flush()
		}
	},
}

func formatReqHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-type", "text/json")

	if req.Method == "GET" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, fmt.Sprintf("[%s]", strings.Join(validFormats, ",")))
		return
	}

	if !(req.Method == "PUT" || req.Method == "DELETE") {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	formatName := strings.Replace(req.URL.Path, "/format/", "", 1)
	newWriteChan := make(chan inputType)
	enabled := req.Method == "PUT"

	formatChan <- writeFormatMutate{writeFormat{formatName, newWriteChan}, enabled}
	log.Printf("format '%s' enabled? %v", formatName, enabled)
}

func main() {
	listenPort := flag.Uint("port", defaultListenPort, "http listen port")
	listenHost := flag.String("listen", defaultListenHost, "http listen host")

	flag.Parse()

	if listenHost == nil || listenPort == nil || *listenPort < 1024 || *listenPort > 65535 {
		log.Panic("listen spec")
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
		fmt.Fprintf(w, version)
	})

	for _, validFormat := range validFormats {
		http.HandleFunc(fmt.Sprintf("/format/%s", validFormat), formatReqHandler)
	}

	//TODO this should only return what's currently enabled (for options page)
	//TODO need to actually persist these settings!
	http.HandleFunc("/format", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "GET" {
			jsonBytes, err := json.Marshal(validFormats)

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
			var bodyObj inputType

			if err := json.Unmarshal(bodyBytes, &bodyObj); err != nil {
				log.Printf("main json parse failed\n%v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			writeChan <- bodyObj
			w.WriteHeader(http.StatusOK)
		}
	})

	var writers map[string]chan inputType = map[string]chan inputType{}
	var writersLock sync.Mutex

	// writers handler
	go func() {
		for toWrite := range writeChan {
			writersLock.Lock()
			for _, writerChan := range writers {
				writerChan <- toWrite
			}
			writersLock.Unlock()
		}
	}()

	// format mutator
	go func() {
		for formatMutate := range formatChan {
			writersLock.Lock()
			if formatMutate.enabled {
				if _, ok := writers[formatMutate.format.name]; ok {
					log.Printf("attempt to enable format '%s' but already enabled", formatMutate.format.name)
				} else {
					// starts the format's handler on the channel
					go formatHandlers[formatMutate.format.name](formatMutate.format.write, localDataPath)
					writers[formatMutate.format.name] = formatMutate.format.write
				}
			} else {
				close(writers[formatMutate.format.name])
				delete(writers, formatMutate.format.name)
			}
			writersLock.Unlock()
		}
	}()

	listenSpec := fmt.Sprintf("%s:%d", *listenHost, *listenPort)
	log.Printf("listening on %s\n", listenSpec)
	http.ListenAndServe(listenSpec, nil)
}

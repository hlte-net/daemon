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

var writeChan = make(chan inputType)
var formatChan = make(chan writeFormatMutate)

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
			csvWriter.Write([]string{
				w.Input.Checksum,
				w.Input.Payload.URI,
				w.Input.Payload.Data,
				fmt.Sprintf("%d", w.Timestamp),
			})
			csvWriter.Flush()
		}
	},
}

func validFormats() []string {
	retVal := make([]string, 0, len(formatHandlers))

	for formatKey := range formatHandlers {
		retVal = append(retVal, formatKey)
	}

	return retVal
}

func formatReqHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-type", "text/json")

	if req.Method == "GET" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, fmt.Sprintf("[%s]", strings.Join(validFormats(), ",")))
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

	var writers map[string]chan inputType = map[string]chan inputType{}
	var writersLock sync.Mutex

	// writers handler
	go func() {
		for toWrite := range writeChan {
			writersLock.Lock()
			for _, formatName := range toWrite.Input.Formats {
				if writerChan, ok := writers[formatName]; ok {
					writerChan <- toWrite
					log.Printf("post %v sent to %v writer", toWrite.Input.Checksum, formatName)
				} else {
					log.Printf("WARN: attempt to use invalid format '%s'", formatName)
				}
			}
			writersLock.Unlock()
		}
	}()

	// format mutator -- has ability to dynamically enable/disable formats, but this
	// functionality isn't current used after the choice was made to avoid persisting any
	// type of format configuration here and defer that entirely to the extension's settings
	// (and therefore just now have the extension pass the formats to be used with each post)
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

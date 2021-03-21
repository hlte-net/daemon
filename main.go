package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
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
		allowHdrs := []string{ppHeader, "content-type"}
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowHdrs[:], ","))
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

	http.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
		if authed, err := authCheck(w, req); !authed || err != nil {
			return
		}

		if req.Method == "GET" {
			limit := searchDefaultLimit
			newestFirst := true
			if qArr, ok := req.URL.Query()["q"]; ok {
				if limitArr, ok := req.URL.Query()["l"]; ok {
					limit, err = strconv.Atoi(limitArr[0])

					if err != nil || limit < 1 || limit > searchMaxLimit {
						log.Printf("limit parse error %s", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				}

				if nfArr, ok := req.URL.Query()["d"]; ok {
					newestFirst, err = strconv.ParseBool(nfArr[0])

					if err != nil {
						log.Printf("d parse error %s", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				}

				qStr := html.UnescapeString(qArr[0])
				qSpec := QuerySpec{qStr, limit, newestFirst, localDataPath}
				log.Printf("search %v\n", qSpec)

				rows, err := queryFormat("sqlite", qSpec)

				if err != nil {
					log.Printf("queryFormat('%s') failed: %s\n", qStr, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				log.Printf("found %d rows\n", len(rows))

				noEscEnc := json.NewEncoder(w)
				noEscEnc.SetEscapeHTML(false)
				err = noEscEnc.Encode(rows)

				if err != nil {
					log.Printf("queryFormat('%s') Marshal failed: %s\n", qStr, err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-type", "application/json")
				return
			}
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
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

		var bodyObj ingestType

		if fmtsArr, ok := req.URL.Query()["formats"]; ok {
			bodyObj.Formats = strings.Split(fmtsArr[0], ",")
		} else {
			log.Printf("POST w/o formats")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if csArr, ok := req.URL.Query()["checksum"]; ok {
			bodyObj.Checksum = html.UnescapeString(csArr[0])
		}

		var bodyBytes []byte

		if bodyBytes, err = ioutil.ReadAll(req.Body); err != nil {
			log.Printf("main body read failed\n%v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if len(bodyBytes) > 0 {
			payloadHash := sha256.Sum256(bodyBytes)
			payloadDigest := hex.EncodeToString(payloadHash[:])

			// client always tries to provide checksum but sometimes is unable (some sites reset
			// crypto.subtle), so if it is empty do not verify, just set
			if len(bodyObj.Checksum) == 0 {
				bodyObj.Checksum = payloadDigest
				log.Printf("warn: payload rx'ed w/o checksum:\n%v", string(bodyBytes))
			} else if payloadDigest != bodyObj.Checksum {
				log.Printf("json validation verify failed\n%v vs %v", payloadDigest, bodyObj.Checksum)
				log.Printf("%v", string(bodyBytes))
				w.WriteHeader(http.StatusForbidden)
				return
			}

			if err := json.Unmarshal(bodyBytes, &bodyObj.Payload); err != nil {
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

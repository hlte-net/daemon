package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var writers map[string]chan inputType = map[string]chan inputType{}
var writersLock sync.Mutex

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
				w.Input.Payload.Annotation,
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

func writersHandler(writeChan chan inputType) {
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
}

// format mutator -- has ability to dynamically enable/disable formats, but this
// functionality isn't current used after the choice was made to avoid persisting any
// type of format configuration here and defer that entirely to the extension's settings
// (and therefore just now have the extension pass the formats to be used with each post)
func formatsMutator(formatChan chan writeFormatMutate, localDataPath string) {
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
}

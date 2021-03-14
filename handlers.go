package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var writers map[string]chan inputType = map[string]chan inputType{}
var writersLock sync.Mutex

func sqliteDbHandle(localDataPath string) *sql.DB {
	dbName := fmt.Sprintf("%s/data.sqlite3", localDataPath)

	_, err := os.Stat(dbName)
	newDb := os.IsNotExist(err)

	db, err := sql.Open("sqlite3", dbName)

	if err != nil {
		log.Panicf("sqlite handler: %v", err)
	}

	if newDb {
		log.Printf("initializing %s", dbName)

		createStmnt := `create table hlte (
			checksum text not null,
			timestamp integer not null,
			primaryURI text not null,
			secondaryURI text,
			hilite text,
			annotation text
			);
			delete from hlte;`

		if _, err = db.Exec(createStmnt); err != nil {
			log.Fatalf("sqlite init failed: %v", err)
		}
	}

	return db
}

// handlers are executed before any requests are accepted from clients
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
			//TODO: arrange these like in same order as sqlite below
			csvWriter.Write([]string{
				w.Input.Checksum,
				w.Input.Payload.URI,
				w.Input.Payload.Data,
				fmt.Sprintf("%d", w.Timestamp),
				w.Input.Payload.Annotation,
				w.Input.Payload.SecondaryURI,
			})
			csvWriter.Flush()
		}
	},
	"sqlite": func(channel chan inputType, localDataPath string) {
		db := sqliteDbHandle(localDataPath)
		defer db.Close()

		for w := range channel {
			_, err := db.Exec("insert into hlte values(?, ?, ?, ?, ?, ?)",
				w.Input.Checksum,
				w.Timestamp,
				w.Input.Payload.URI,
				w.Input.Payload.SecondaryURI,
				w.Input.Payload.Data,
				w.Input.Payload.Annotation,
			)

			if err != nil {
				log.Printf("ERROR: failed to write %s to sqlite!", w.Input.Checksum)
			}
		}
	},
}

var queryHandlers = map[string]formatQueryFunc{
	"sqlite": func(query string, localDataPath string) (retval []interface{}) {
		db := sqliteDbHandle(localDataPath)
		defer db.Close()

		pStmt, err := db.Prepare(`select timestamp, primaryURI, secondaryURI, hilite, annotation 
			from hlte where hilite like '%' || ? || '%' or annotation like '%' || ? || '%' or 
			primaryURI like '%' || ? || '%' or secondaryURI like '%' || ? || '%'`)

		if err != nil {
			log.Printf("prep failed: %s", err)
			return
		}

		defer pStmt.Close()
		rows, err := pStmt.Query(query, query, query, query)

		if err != nil {
			log.Printf("query failed: %s", err)
			return
		}

		defer rows.Close()

		for rows.Next() {
			var timestamp int64
			var primaryURI string
			var secondaryURI string
			var hilite string
			var annotation string

			err = rows.Scan(&timestamp, &primaryURI, &secondaryURI, &hilite, &annotation)

			if err != nil {
				log.Printf("scan failed: %s", err)
				return
			}

			retval = append(retval, map[string]string{
				"timestamp":    fmt.Sprintf("%d", timestamp),
				"primaryURI":   primaryURI,
				"secondaryURI": secondaryURI,
				"hilite":       hilite,
				"annotation":   annotation,
			})
		}

		return
	},
}

func validFormats() []string {
	retVal := make([]string, 0, len(formatHandlers))

	for formatKey := range formatHandlers {
		retVal = append(retVal, formatKey)
	}

	return retVal
}

func queryFormat(format string, localDataPath string, query string) ([]interface{}, error) {
	if handler, ok := queryHandlers[format]; ok {
		return handler(query, localDataPath), nil
	} else {
		return nil, fmt.Errorf("bad format %s", format)
	}
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

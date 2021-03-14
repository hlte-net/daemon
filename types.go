package main

type payloadType struct {
	Data         string `json:"data"`
	URI          string `json:"uri"`
	Annotation   string `json:"annotation",omitempty`
	SecondaryURI string `json:"secondaryURI",omitempty`
}

type ingestType struct {
	Checksum string
	Payload  payloadType
	Formats  []string
}

type inputType struct {
	Input     ingestType
	Timestamp int64
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

type formatQueryFunc func(query string, localDataPath string) []interface{}

type Config struct {
	Host             string
	Port             int
	Version          string
	PassphraseSha512 string
	LocalDataPath    string
}

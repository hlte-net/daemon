package main

type payloadType struct {
	Data       string
	URI        string
	Annotation string
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

type Config struct {
	Host             string
	Port             int
	Version          string
	PassphraseSha512 string
	LocalDataPath    string
}

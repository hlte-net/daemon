package main

type payloadType struct {
	Data string
	URI  string
}

type inputType struct {
	Checksum string
	Payload  payloadType
	Formats  []string
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
	Host    string
	Port    int
	Version string
}

package main

import (
	"net/http"
)

const version = "20210320"
const defaultConfigPath = "./config.json"
const dataPathEnvVarName = "HLTE_DAEMON_DATA_PATH"
const searchDefaultLimit = 10
const searchMaxLimit = 8192

var ppHeader = http.CanonicalHeaderKey("x-hlte-pp")

package main

import (
	"net/http"
)

const version = "20200830"
const defaultConfigPath = "./config.json"
const dataPathEnvVarName = "HLTE_DAEMON_DATA_PATH"

var ppHeader = http.CanonicalHeaderKey("x-hlte-pp")

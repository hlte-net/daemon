package main

import (
	"net/http"
)

const version = "20220111"
const defaultConfigPath = "./config.json"
const dataPathEnvVarName = "HLTE_DAEMON_DATA_PATH"
const searchDefaultLimit = 10
const searchMaxLimit = 8192

var ppHeader = http.CanonicalHeaderKey("x-hlte-pp")

const jsonIndent = "  "

// https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
const argonBytesOfSalt = 32
const argonMemoryMb = 64
const argonTime = 1
const argonParallelism = 4
const argonHashLength = 64

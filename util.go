package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppName defines the default global application name
const AppName string = "net.hlte.daemon"

// LocalDataPath returns the appropriate path for general user-data storage on the given platform.
// If `checkEnvVar` is non-empty, it must contain the name of an environmental variable to source for the path prior to generating one.
func LocalDataPath(checkEnvVar string) (string, error) {
	if len(checkEnvVar) > 0 {
		envPath := os.Getenv(checkEnvVar)

		if len(envPath) > 0 {
			return envPath, nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		return "Library/Application Support", nil
	case "windows":
		return "AppData", nil
	case "linux":
		return fmt.Sprintf(".%s", AppName), nil
	}

	return "", fmt.Errorf("Unsupported runtime platform '%v'", runtime.GOOS)
}

// InitLocalData will prepare the directory at `path` for use as a local user data store
func InitLocalData(path string) (string, error) {
	userHomeDir, err := os.UserHomeDir()

	if err != nil {
		fmt.Fprintf(os.Stderr, "user homedir lookup failed: %v\n", err)
		return "", err
	}

	absPath, err := filepath.Abs(fmt.Sprintf("%s/%s/%s", userHomeDir, path, AppName))

	if err != nil {
		fmt.Fprintf(os.Stderr, "initLocalData failed: %v\n", err)
		return absPath, err
	}

	absPath = filepath.FromSlash(absPath)
	err = os.MkdirAll(absPath, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initLocalData MkdirAll failed: %v\n", err)
		return absPath, err
	}

	return absPath, nil
}

// ParseJSON is a convenience function for parsing a JSON file at `path` into the object `intoObj`
func ParseJSON(path string, intoObj interface{}) error {
	file, err := os.Open(path)

	if err != nil {
		fmt.Fprintf(os.Stderr, "parseJSON unable to open '%s': %v\n", path, err)
		return err
	}

	defer file.Close()

	dec := json.NewDecoder(file)
	err = dec.Decode(intoObj)

	if err != nil {
		fmt.Fprintf(os.Stderr, "parseJSON failed to decode: %v\n", err)
		return err
	}

	return nil
}

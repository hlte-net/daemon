package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/argon2"
	"log"
	"os"
)

func hash(password string, salt *string) (Auth, error) {
	var saltBytes []byte
	var err error

	if salt != nil {
		saltBytes, err = hex.DecodeString(*salt)

		if err != nil {
			return Auth{}, err
		}

		if len(saltBytes) != argonBytesOfSalt {
			return Auth{}, fmt.Errorf("wrong salt length (%d vs %d)", len(saltBytes), argonBytesOfSalt)
		}
	} else {
		saltBytes = make([]byte, argonBytesOfSalt)
		_, err := rand.Read(saltBytes)

		if err != nil {
			return Auth{}, err
		}
	}

	hashed := argon2.IDKey([]byte(password), saltBytes, argonTime, argonMemoryMb*1024, argonParallelism, argonHashLength)
	return Auth{hex.EncodeToString(hashed), hex.EncodeToString(saltBytes)}, nil
}

func hashPasswordThenExit(hashPassword, configPath *string, config Config) {
	authObj, err := hash(*hashPassword, nil)

	if err != nil {
		log.Panicf("couldn't hash password! %v", err)
	}

	log.Printf("writing config file at %s...", *configPath)
	config.Auth = authObj
	jsonBytes, err := json.MarshalIndent(config, "", jsonIndent)

	if err != nil {
		log.Panicf("unable to marshal %v", err)
	}

	if file, err := os.Create(*configPath); err == nil {
		if _, err = file.Write(jsonBytes); err != nil {
			log.Panicf("failed to write '%s': %v", *configPath, err)
		}
	}

	log.Printf("success")
	os.Exit(0)
}

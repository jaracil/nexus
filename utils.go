package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/scrypt"
)

func inStrSlice(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func safeId(sz int) string {
	if sz <= 0 {
		sz = 16
	}
	b := make([]byte, sz)
	n, err := rand.Read(b)
	if err != nil || n != len(b) {
		panic(errors.New("Can't read from crypto/rand"))
	}
	return hex.EncodeToString(b)
}

func prefixes(s string) (res []string) {
	for strings.HasSuffix(s, ".") {
		s = s[0 : len(s)-1]
	}
	chunks := strings.Split(s, ".")
	for n := len(chunks); n > 0; n-- {
		res = append(res, strings.Join(chunks[0:n], "."))
	}
	res = append(res, ".")
	return
}

func getPathMethod(s string) (path, method string) {
	chunks := strings.Split(s, ".")
	path = strings.Join(chunks[0:len(chunks)-1], ".") + "."
	method = chunks[len(chunks)-1]
	return
}

func HashPass(pass, salt string) (string, error) {
	bsalt, err := hex.DecodeString(salt)
	if err != nil {
		return "", errors.New("Invalid salt")
	}
	bdk, err := scrypt.Key([]byte(pass), bsalt, 16384, 8, 1, 16)
	if err != nil {
		return "", errors.New("scrypt error")
	}
	return hex.EncodeToString(bdk), nil
}

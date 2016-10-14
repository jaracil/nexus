package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
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

func headerContains(header []string, str string) bool {
	if header == nil {
		return false
	}
	for _, s := range header {
		if strings.Contains(s, str) {
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

func truncateJson(j interface{}) interface{} {
	switch t := j.(type) {
	//Number
	case float64:
		return j

	// Null
	case nil:
		return j

	// Bool
	case bool:
		return j

	// String
	case string:
		maxlen := 1024 * 10

		if len(t) > maxlen {
			return t[:maxlen] + "..."
		}
		return t

	// Object
	case map[string]interface{}:
		a := make(map[string]interface{})
		for k, v := range t {
			a[k] = truncateJson(v)
		}
		return a

	// Array?
	default:
		slice := make([]interface{}, 0)
		if b, e := json.Marshal(j); e == nil && json.Unmarshal(b, &slice) == nil {
			for k, v := range slice {
				slice[k] = truncateJson(v)
			}
			return slice
		}
	}
	return fmt.Sprintf("Unknown JSON type: %s", reflect.TypeOf(j))
}

func roundHelper(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func round(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(roundHelper(num*output)) / output
}

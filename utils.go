package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/fatih/color"

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

// Logging

func infoln(i ...interface{}) {
	log.Println(i...)
}

func infof(s string, i ...interface{}) {
	log.Printf(s, i...)
}

func errln(i ...interface{}) {
	log.Println(color.RedString("%s", fmt.Sprint(i...)))
}

func errf(s string, i ...interface{}) {
	log.Printf("%s", color.RedString("%s", fmt.Sprintf(s, i...)))
}

func errfatalln(i ...interface{}) {
	log.Fatalln(color.RedString("%s", fmt.Sprint(i...)))
}

func errfatalf(s string, i ...interface{}) {
	log.Fatalf("%s", color.RedString("%s", fmt.Sprintf(s, i...)))
}

func warnln(i ...interface{}) {
	log.Println(color.YellowString("%s", fmt.Sprint(i...)))
}

func warnf(s string, i ...interface{}) {
	log.Printf("%s", color.YellowString("%s", fmt.Sprintf(s, i...)))
}

func sysln(i ...interface{}) {
	log.Println(color.WhiteString("%s", fmt.Sprint(i...)))
}

func sysf(s string, i ...interface{}) {
	log.Printf("%s", color.WhiteString("%s", fmt.Sprintf(s, i...)))
}

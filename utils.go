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

	"github.com/jaracil/ei"

	"golang.org/x/crypto/scrypt"
	r "gopkg.in/rethinkdb/rethinkdb-go.v5"
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
		if strings.Contains(strings.ToLower(s), strings.ToLower(str)) {
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
	chunks := strings.Split(strings.TrimRight(s, "."), ".")
	for n := len(chunks); n > 0; n-- {
		res = append(res, strings.Join(chunks[0:n], "."))
	}
	res = append(res, ".")
	return
}

func getPathMethod(s string) (path, method string) {
	chunks := strings.Split(strings.TrimRight(s, "."), ".")
	path = strings.Join(chunks[0:len(chunks)-1], ".") + "."
	method = chunks[len(chunks)-1]
	return
}

func getPrefixParam(reqParams interface{}) string {
	return strings.TrimRight(ei.N(reqParams).M("prefix").Lower().StringZ(), ".")
}

func getListParams(reqParams interface{}) (string, int, string, int, int) {
	prefix := getPrefixParam(reqParams)
	depth, err := ei.N(reqParams).M("depth").Int()
	if err != nil {
		depth = -1
	}
	filter := ei.N(reqParams).M("filter").StringZ()
	limit, err := ei.N(reqParams).M("limit").Int()
	if err != nil {
		limit = 100
	}
	skip, err := ei.N(reqParams).M("skip").Int()
	if err != nil {
		skip = 0
	}
	return prefix, depth, filter, limit, skip
}

func getCountTerm(table string, index string, filterBy string, prefix string, filter string, subprefixes bool) r.Term {
	var term r.Term
	if subprefixes {
		if prefix == "" {
			term = r.Table(table)
			if filter != "" {
				term = term.Filter(r.Row.Field(filterBy).Match(filter))
			}
			term = term.Group(r.Row.Field(filterBy).Match("^([^.]*)(?:[.][^.]*)*$").Field("groups").Nth(0).Field("str"))
			return term.Count().Ungroup().Map(func(t r.Term) r.Term {
				return r.Branch(t.HasFields("group"), r.Object("prefix", t.Field("group"), "count", t.Field("reduction")), r.Object("prefix", "", "count", t.Field("reduction")))
			})
		} else {
			if index != "" {
				term = r.Table(table).GetAllByIndex(index, prefix).Union(r.Table(table).Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: index}))
			} else {
				term = r.Table(table).GetAll(prefix).Union(r.Table(table).Between(prefix+".", prefix+".\uffff"))
			}
			if filter != "" {
				term = term.Filter(r.Row.Field(filterBy).Match(filter))
			}
			term = term.Group(r.Row.Field(filterBy).Match(fmt.Sprintf("^%s[.]([^.]*)(?:[.][^.]*)*$", prefix)).Field("groups").Nth(0).Field("str"))
			return term.Count().Ungroup().Map(func(t r.Term) r.Term {
				return r.Branch(t.HasFields("group"), r.Object("prefix", r.Add(prefix+".", t.Field("group")), "count", t.Field("reduction")), r.Object("prefix", prefix, "count", t.Field("reduction")))
			})
		}
	} else {
		if prefix == "" {
			term = r.Table(table)
		} else {
			if index != "" {
				term = r.Table(table).GetAllByIndex(index, prefix).Union(r.Table(table).Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: index}))
			} else {
				term = r.Table(table).GetAll(prefix).Union(r.Table(table).Between(prefix+".", prefix+".\uffff"))
			}
		}
		if filter != "" {
			term = term.Filter(r.Row.Field(filterBy).Match(filter))
		}
		return term.Count()
	}
}

func getListTerm(table string, index string, filterBy string, prefix string, depth int, filter string, limit int, skip int) r.Term {
	var term r.Term
	if prefix == "" {
		if depth < 0 {
			term = r.Table(table)
		} else if depth == 0 {
			if index != "" {
				term = r.Table(table).GetAllByIndex(index, prefix)
			} else {
				term = r.Table(table).GetAll(prefix)
			}
		} else if depth == 1 {
			term = r.Table(table).Filter(r.Row.Field(filterBy).Match("^[^.]*$"))
		} else {
			term = r.Table(table).Filter(r.Row.Field(filterBy).Match(fmt.Sprintf("^[^.]*(?:[.][^.]*){0,%d}$", depth-1)))
		}
	} else {
		if index != "" {
			term = r.Table(table).GetAllByIndex(index, prefix)
		} else {
			term = r.Table(table).GetAll(prefix)
		}
		if depth != 0 {
			if index != "" {
				term = term.Union(r.Table(table).Between(prefix+".", prefix+".\uffff", r.BetweenOpts{Index: index}))
			} else {
				term = term.Union(r.Table(table).Between(prefix+".", prefix+".\uffff"))
			}
		}
		if depth > 0 {
			term = term.Filter(r.Row.Field(filterBy).Match(fmt.Sprintf("^%s(?:[.][^.]*){0,%d}$", prefix, depth)))
		}
	}
	if filter != "" {
		term = term.Filter(r.Row.Field(filterBy).Match(filter))
	}
	if skip >= 0 {
		term = term.Skip(skip)
	}
	if limit > 0 {
		term = term.Limit(limit)
	}
	return term
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

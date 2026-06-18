package semver

import (
	"strconv"
	"strings"
)

type version struct {
	parts [3]int
}

func Valid(tag string) bool {
	_, ok := parse(tag)
	return ok
}

func Compare(a, b string) int {
	va, oka := parse(a)
	vb, okb := parse(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := range va.parts {
		if va.parts[i] > vb.parts[i] {
			return 1
		}
		if va.parts[i] < vb.parts[i] {
			return -1
		}
	}
	return 0
}

func Latest(tags []string) string {
	latest := ""
	for _, tag := range tags {
		if !Valid(tag) {
			continue
		}
		if latest == "" || Compare(tag, latest) > 0 {
			latest = tag
		}
	}
	return latest
}

func NaturalLess(a, b string) bool {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a < b
}

func parse(tag string) (version, bool) {
	tag = strings.TrimPrefix(tag, "v")
	core := strings.SplitN(tag, "-", 2)[0]
	fields := strings.Split(core, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return version{}, false
	}
	var out version
	for i, field := range fields {
		if field == "" {
			return version{}, false
		}
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return version{}, false
		}
		out.parts[i] = n
	}
	return out, true
}

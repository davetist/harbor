package updates

import (
	"regexp"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type version struct {
	core [3]string
	pre  []string
}

func parseVersion(text string) (version, bool) {
	parts := versionPattern.FindStringSubmatch(text)
	if parts == nil {
		return version{}, false
	}
	v := version{core: [3]string{parts[1], parts[2], parts[3]}}
	if parts[4] != "" {
		v.pre = strings.Split(parts[4], ".")
		for _, identifier := range v.pre {
			if numeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
				return version{}, false
			}
		}
	}
	return v, true
}

func numeric(text string) bool {
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return text != ""
}

func compareNumber(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

func (v version) compare(other version) int {
	for i := range v.core {
		if result := compareNumber(v.core[i], other.core[i]); result != 0 {
			return result
		}
	}
	if len(v.pre) == 0 && len(other.pre) > 0 {
		return 1
	}
	if len(v.pre) > 0 && len(other.pre) == 0 {
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(other.pre); i++ {
		a, b := v.pre[i], other.pre[i]
		an, bn := numeric(a), numeric(b)
		if an && !bn {
			return -1
		}
		if !an && bn {
			return 1
		}
		result := strings.Compare(a, b)
		if an {
			result = compareNumber(a, b)
		}
		if result != 0 {
			return result
		}
	}
	if len(v.pre) < len(other.pre) {
		return -1
	}
	if len(v.pre) > len(other.pre) {
		return 1
	}
	return 0
}

func (v version) development() bool {
	return len(v.pre) > 0 && (v.pre[0] == "dev" || strings.HasPrefix(v.pre[0], "dev-"))
}

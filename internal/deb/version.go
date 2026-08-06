package deb

import (
	"strings"
)

// CompareVersions implements the Debian version comparison algorithm, the same
// one used by `dpkg --compare-versions` / `dpkg-version-*`.
//
// It returns -1 if a < b, 0 if a == b, 1 if a > b.
//
// A Debian version string has the form [epoch:]upstream[-revision]:
//   - epoch:       an unsigned integer (optional), a larger epoch always wins.
//   - upstream:    must begin with a digit; may contain [0-9a-zA-Z.+~:-].
//   - revision:    (optional) split at the LAST '-', compared like upstream.
//
// The leading "v"/"V" of GitHub tags (e.g. "v1.17.0") is tolerated by
// stripping it, since dpkg rejects versions that don't start with a digit.
func CompareVersions(a, b string) int {
	ea, ua, ra := parseVersion(a)
	eb, ub, rb := parseVersion(b)

	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
	if c := verrevcmp(ua, ub); c != 0 {
		return c
	}
	return verrevcmp(ra, rb)
}

// parseVersion splits a version into (epoch, upstream, revision).
func parseVersion(v string) (int, string, string) {
	epoch := 0
	// Strip a single leading "v"/"V" tag prefix before anything else, since
	// Debian upstream versions must start with a digit.
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') {
		v = v[1:]
	}
	// epoch: everything before the first ':' (only if it's a non-empty digit run)
	if i := strings.IndexByte(v, ':'); i > 0 {
		ep := v[:i]
		valid := true
		for _, c := range ep {
			if c < '0' || c > '9' {
				valid = false
				break
			}
		}
		if valid {
			for _, c := range ep {
				epoch = epoch*10 + int(c-'0')
			}
			v = v[i+1:]
		}
	}
	// revision: split at the LAST '-'
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

// verrevcmp compares two non-epoch version components (upstream or revision)
// using dpkg's ordering: '~' < nothing < digits/letters < '+' < '.' < others.
// Returns -1 / 0 / 1.
func verrevcmp(a, b string) int {
	ca, cb := []byte(a), []byte(b)
	ia, ib := 0, 0
	la, lb := len(ca), len(cb)

	for ia < la || ib < lb {
		// Compare the non-digit prefix runs.
		for (ia < la && !isDigit(ca[ia])) || (ib < lb && !isDigit(cb[ib])) {
			ac := order(ca, ia, la)
			bc := order(cb, ib, lb)
			if ac != bc {
				if ac < bc {
					return -1
				}
				return 1
			}
			if ia < la {
				ia++
			}
			if ib < lb {
				ib++
			}
		}
		// Skip leading zeros so "007" == "7".
		for ia < la && ca[ia] == '0' {
			ia++
		}
		for ib < lb && cb[ib] == '0' {
			ib++
		}
		// Compare the digit runs numerically (longer = bigger; on equal length,
		// first differing digit decides).
		firstDiff := 0
		for ia < la && isDigit(ca[ia]) && ib < lb && isDigit(cb[ib]) {
			if firstDiff == 0 {
				firstDiff = int(ca[ia]) - int(cb[ib])
			}
			ia++
			ib++
		}
		if ia < la && isDigit(ca[ia]) {
			return 1
		}
		if ib < lb && isDigit(cb[ib]) {
			return -1
		}
		if firstDiff != 0 {
			if firstDiff < 0 {
				return -1
			}
			return 1
		}
	}
	return 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// order returns the dpkg weight of the character at index i in s.
// Out-of-range (i >= len) maps to 0, matching dpkg's NUL terminator handling.
func order(s []byte, i, n int) int {
	if i >= n {
		return 0
	}
	c := s[i]
	switch {
	case c >= '0' && c <= '9':
		return 0
	case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		return int(c)
	case c == '~':
		return -1
	default:
		return int(c) + 256
	}
}

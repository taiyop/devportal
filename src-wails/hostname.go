package main

import (
	"net"
	"strings"
	"unicode"
)

const hostnameSuffix = "devportal.localhost"

func fullHostname(label string) string {
	if label == "" {
		return ""
	}
	return label + "." + hostnameSuffix
}

func publicAppURL(entry AppEntry, port *uint16, status AppStatus, gatewayPort uint16, bindHost string) *string {
	if entry.Hostname != "" {
		u := "http://" + fullHostname(entry.Hostname)
		if gatewayPort != 0 && gatewayPort != 80 {
			u += ":" + itoaPort(gatewayPort)
		}
		return &u
	}
	switch status {
	case AppStatusIdle, AppStatusStarting, AppStatusRunning:
		if port != nil {
			u := loopbackURL(bindHost, *port)
			return &u
		}
	}
	return nil
}

func hostLabelFromRequest(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return ""
	}
	if parsed, _, err := net.SplitHostPort(h); err == nil {
		h = parsed
	}
	h = strings.TrimSuffix(h, ".")
	suffix := "." + hostnameSuffix
	if !strings.HasSuffix(h, suffix) {
		return ""
	}
	label := strings.TrimSuffix(h, suffix)
	if label == "" || strings.Contains(label, ".") {
		return ""
	}
	return label
}

func normalizeHostname(input string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimSuffix(s, "/")
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(s, "."+hostnameSuffix)
	s = strings.Trim(s, ".")
	if s == "" {
		return "", nil
	}
	if strings.Contains(s, ".") {
		return "", errString("ドメインは " + hostnameSuffix + " の直前の1ラベルだけ指定してください")
	}
	if !validHostnameLabel(s) {
		return "", errString("ドメインが不正です: " + s + "（英小文字・数字・ハイフン、先頭末尾はハイフン不可）")
	}
	return s, nil
}

func validHostnameLabel(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func slugHostname(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '_' || r == '.' || r == '/':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	if out == "" {
		return "app"
	}
	return out
}

func uniqueHostname(apps []AppEntry, id, preferred string) string {
	if preferred == "" {
		return ""
	}
	candidate := preferred
	n := 2
	for {
		taken := false
		for _, app := range apps {
			if app.ID != id && app.Hostname == candidate {
				taken = true
				break
			}
		}
		if !taken {
			return candidate
		}
		suffix := "-" + itoaPort(uint16(n))
		base := preferred
		if len(base)+len(suffix) > 63 {
			base = strings.Trim(base[:63-len(suffix)], "-")
		}
		candidate = base + suffix
		n++
	}
}

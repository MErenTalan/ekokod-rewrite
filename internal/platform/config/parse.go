package config

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var rateWindows = map[string]time.Duration{
	"s": time.Second, "sec": time.Second, "second": time.Second,
	"m": time.Minute, "min": time.Minute, "minute": time.Minute,
	"h": time.Hour, "hour": time.Hour,
	"d": 24 * time.Hour, "day": 24 * time.Hour,
}

// parseRateLimit parses "<limit>/<count><unit>", e.g. "120/min" or "5/15min".
func parseRateLimit(in string) (RateLimit, error) {
	limitPart, windowPart, ok := strings.Cut(strings.TrimSpace(in), "/")
	if !ok {
		return RateLimit{}, fmt.Errorf("expected <limit>/<window>, got %q", in)
	}
	limit, err := strconv.Atoi(strings.TrimSpace(limitPart))
	if err != nil || limit <= 0 {
		return RateLimit{}, fmt.Errorf("limit must be a positive integer, got %q", limitPart)
	}

	windowPart = strings.TrimSpace(windowPart)
	digits := 0
	for digits < len(windowPart) && windowPart[digits] >= '0' && windowPart[digits] <= '9' {
		digits++
	}
	count := 1
	if digits > 0 {
		count, _ = strconv.Atoi(windowPart[:digits])
	}
	unit, known := rateWindows[strings.ToLower(windowPart[digits:])]
	if !known || count <= 0 {
		return RateLimit{}, fmt.Errorf("unknown window %q in %q", windowPart, in)
	}
	return RateLimit{Limit: limit, Window: time.Duration(count) * unit}, nil
}

// parseKey decodes a base64 key and checks its byte length.
func parseKey(raw string, wantBytes int) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("must be base64: %w", err)
	}
	if len(key) != wantBytes {
		return nil, fmt.Errorf("must decode to exactly %d bytes, got %d", wantBytes, len(key))
	}
	return key, nil
}

// parsePinnedCerts parses "host=base64der,host2=base64der".
func parsePinnedCerts(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		host, cert, ok := strings.Cut(pair, "=")
		if !ok || host == "" || cert == "" {
			return nil, fmt.Errorf("expected host=base64der pairs, got %q", pair)
		}
		if _, err := base64.StdEncoding.DecodeString(cert); err != nil {
			return nil, fmt.Errorf("certificate for %q is not base64: %w", host, err)
		}
		out[host] = cert
	}
	return out, nil
}

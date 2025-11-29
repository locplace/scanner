// Package scanner provides the DNS LOC record scanner implementation.
package scanner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/locplace/scanner/pkg/api"
)

// LOC record format from zdns:
// "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m"
// Format: d1 m1 s1 N/S d2 m2 s2 E/W alt size hp vp

var locRegex = regexp.MustCompile(
	`^(\d+)\s+(\d+)\s+([\d.]+)\s+([NS])\s+` + // latitude
		`(\d+)\s+(\d+)\s+([\d.]+)\s+([EW])\s+` + // longitude
		`(-?[\d.]+)m\s*` + // altitude
		`([\d.]+)m?\s*` + // size (optional m suffix)
		`([\d.]+)m?\s*` + // horiz precision (optional m suffix)
		`([\d.]+)m?$`, // vert precision (optional m suffix)
)

// ParseLOCRecord parses a LOC record string from zdns into structured data.
// Input format: "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m"
func ParseLOCRecord(fqdn, raw string) (*api.LOCRecord, error) {
	raw = strings.TrimSpace(raw)

	matches := locRegex.FindStringSubmatch(raw)
	if matches == nil {
		return nil, fmt.Errorf("invalid LOC record format: %s", raw)
	}

	// Parse latitude - regex ensures valid numeric format, so ParseFloat won't fail
	//nolint:errcheck // Regex validates format
	latDeg, _ := strconv.ParseFloat(matches[1], 64)
	latMin, _ := strconv.ParseFloat(matches[2], 64)
	latSec, _ := strconv.ParseFloat(matches[3], 64)
	latHemi := matches[4]

	latitude := latDeg + latMin/60 + latSec/3600
	if latHemi == "S" {
		latitude = -latitude
	}

	// Parse longitude - regex ensures valid numeric format
	//nolint:errcheck // Regex validates format
	lonDeg, _ := strconv.ParseFloat(matches[5], 64)
	lonMin, _ := strconv.ParseFloat(matches[6], 64)
	lonSec, _ := strconv.ParseFloat(matches[7], 64)
	lonHemi := matches[8]

	longitude := lonDeg + lonMin/60 + lonSec/3600
	if lonHemi == "W" {
		longitude = -longitude
	}

	// Parse other fields - regex ensures valid numeric format
	//nolint:errcheck // Regex validates format
	altitude, _ := strconv.ParseFloat(matches[9], 64)
	size, _ := strconv.ParseFloat(matches[10], 64)
	horizPrec, _ := strconv.ParseFloat(matches[11], 64)
	vertPrec, _ := strconv.ParseFloat(matches[12], 64)

	return &api.LOCRecord{
		FQDN:       fqdn,
		RawRecord:  raw,
		Latitude:   latitude,
		Longitude:  longitude,
		AltitudeM:  altitude,
		SizeM:      size,
		HorizPrecM: horizPrec,
		VertPrecM:  vertPrec,
	}, nil
}

// ParseLOCRecordLenient attempts to parse a LOC record with various formats.
// Falls back to extracting what it can if strict parsing fails.
func ParseLOCRecordLenient(fqdn, raw string) (*api.LOCRecord, error) {
	// Try strict parsing first
	if rec, err := ParseLOCRecord(fqdn, raw); err == nil {
		return rec, nil
	}

	// Try to extract coordinates with more lenient regex
	// Some records might have slightly different formatting
	raw = strings.TrimSpace(raw)

	// Pattern for just the coordinates part
	coordRegex := regexp.MustCompile(
		`(\d+)\s+(\d+)\s+([\d.]+)\s+([NS])\s+(\d+)\s+(\d+)\s+([\d.]+)\s+([EW])`,
	)

	matches := coordRegex.FindStringSubmatch(raw)
	if matches == nil {
		return nil, fmt.Errorf("could not parse LOC record: %s", raw)
	}

	// Parse latitude - regex ensures valid numeric format
	//nolint:errcheck // Regex validates format
	latDeg, _ := strconv.ParseFloat(matches[1], 64)
	latMin, _ := strconv.ParseFloat(matches[2], 64)
	latSec, _ := strconv.ParseFloat(matches[3], 64)
	latHemi := matches[4]

	latitude := latDeg + latMin/60 + latSec/3600
	if latHemi == "S" {
		latitude = -latitude
	}

	// Parse longitude - regex ensures valid numeric format
	//nolint:errcheck // Regex validates format
	lonDeg, _ := strconv.ParseFloat(matches[5], 64)
	lonMin, _ := strconv.ParseFloat(matches[6], 64)
	lonSec, _ := strconv.ParseFloat(matches[7], 64)
	lonHemi := matches[8]

	longitude := lonDeg + lonMin/60 + lonSec/3600
	if lonHemi == "W" {
		longitude = -longitude
	}

	// Try to extract altitude and precision from the rest
	rest := raw[len(matches[0]):]
	altitude, size, horizPrec, vertPrec := 0.0, 1.0, 10000.0, 10.0

	// Look for meter values - regex ensures valid numeric format
	meterRegex := regexp.MustCompile(`(-?[\d.]+)m`)
	meterMatches := meterRegex.FindAllStringSubmatch(rest, -1)
	//nolint:errcheck // Regex validates format
	if len(meterMatches) >= 1 {
		altitude, _ = strconv.ParseFloat(meterMatches[0][1], 64)
	}
	if len(meterMatches) >= 2 {
		size, _ = strconv.ParseFloat(meterMatches[1][1], 64)
	}
	if len(meterMatches) >= 3 {
		horizPrec, _ = strconv.ParseFloat(meterMatches[2][1], 64)
	}
	if len(meterMatches) >= 4 {
		vertPrec, _ = strconv.ParseFloat(meterMatches[3][1], 64)
	}

	return &api.LOCRecord{
		FQDN:       fqdn,
		RawRecord:  raw,
		Latitude:   latitude,
		Longitude:  longitude,
		AltitudeM:  altitude,
		SizeM:      size,
		HorizPrecM: horizPrec,
		VertPrecM:  vertPrec,
	}, nil
}

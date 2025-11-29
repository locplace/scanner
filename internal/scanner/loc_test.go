package scanner

import (
	"math"
	"testing"
)

func TestParseLOCRecord(t *testing.T) {
	tests := []struct {
		name        string
		fqdn        string
		raw         string
		wantLat     float64
		wantLon     float64
		wantAlt     float64
		wantSize    float64
		wantHoriz   float64
		wantVert    float64
		wantErr     bool
		tolerance   float64 // for floating point comparison
	}{
		{
			// Real record from caida.org - Northern hemisphere, Western longitude
			name:      "caida.org real record",
			fqdn:      "caida.org",
			raw:       "32 53 1.000 N 117 14 25.000 W 107.00m 30m 10m 10m",
			wantLat:   32.883611111, // 32 + 53/60 + 1/3600
			wantLon:   -117.240277778, // negative because West
			wantAlt:   107.0,
			wantSize:  30.0,
			wantHoriz: 10.0,
			wantVert:  10.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Real record from ckdhr.com - negative altitude (below sea level)
			name:      "ckdhr.com real record with negative altitude",
			fqdn:      "ckdhr.com",
			raw:       "42 21 43.528 N 71 5 6.284 W -25.00m 1m 3000m 10m",
			wantLat:   42.362091111, // 42 + 21/60 + 43.528/3600
			wantLon:   -71.085078889, // negative because West
			wantAlt:   -25.0,
			wantSize:  1.0,
			wantHoriz: 3000.0,
			wantVert:  10.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Southern hemisphere - latitude should be negative
			name:      "southern hemisphere",
			fqdn:      "example.au",
			raw:       "33 51 54.000 S 151 12 36.000 E 10.00m 1m 1000m 10m",
			wantLat:   -33.865, // negative because South
			wantLon:   151.21,  // positive because East
			wantAlt:   10.0,
			wantSize:  1.0,
			wantHoriz: 1000.0,
			wantVert:  10.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Equator and prime meridian (edge case)
			name:      "equator and prime meridian",
			fqdn:      "null-island.example",
			raw:       "0 0 0.000 N 0 0 0.000 E 0.00m 1m 100m 10m",
			wantLat:   0.0,
			wantLon:   0.0,
			wantAlt:   0.0,
			wantSize:  1.0,
			wantHoriz: 100.0,
			wantVert:  10.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// High precision coordinates
			name:      "high precision seconds",
			fqdn:      "precise.example",
			raw:       "51 30 26.464 N 0 7 39.926 W 0.00m 10m 100m 10m",
			wantLat:   51.507351111, // London approximate
			wantLon:   -0.127757222,
			wantAlt:   0.0,
			wantSize:  10.0,
			wantHoriz: 100.0,
			wantVert:  10.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Large altitude (airplane cruising altitude)
			name:      "high altitude",
			fqdn:      "high.example",
			raw:       "40 0 0.000 N 100 0 0.000 W 10000.00m 100m 1000m 100m",
			wantLat:   40.0,
			wantLon:   -100.0,
			wantAlt:   10000.0,
			wantSize:  100.0,
			wantHoriz: 1000.0,
			wantVert:  100.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Invalid format - missing components
			name:    "invalid format - incomplete",
			fqdn:    "bad.example",
			raw:     "52 22 N 4 53 E",
			wantErr: true,
		},
		{
			// Invalid format - garbage
			name:    "invalid format - garbage",
			fqdn:    "bad.example",
			raw:     "not a loc record",
			wantErr: true,
		},
		{
			// Empty string
			name:    "empty string",
			fqdn:    "empty.example",
			raw:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecord(tt.fqdn, tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLOCRecord() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLOCRecord() unexpected error: %v", err)
				return
			}

			if got.FQDN != tt.fqdn {
				t.Errorf("FQDN = %v, want %v", got.FQDN, tt.fqdn)
			}
			if got.RawRecord != tt.raw {
				t.Errorf("RawRecord = %v, want %v", got.RawRecord, tt.raw)
			}
			if !floatEquals(got.Latitude, tt.wantLat, tt.tolerance) {
				t.Errorf("Latitude = %v, want %v (tolerance %v)", got.Latitude, tt.wantLat, tt.tolerance)
			}
			if !floatEquals(got.Longitude, tt.wantLon, tt.tolerance) {
				t.Errorf("Longitude = %v, want %v (tolerance %v)", got.Longitude, tt.wantLon, tt.tolerance)
			}
			if !floatEquals(got.AltitudeM, tt.wantAlt, tt.tolerance) {
				t.Errorf("AltitudeM = %v, want %v", got.AltitudeM, tt.wantAlt)
			}
			if !floatEquals(got.SizeM, tt.wantSize, tt.tolerance) {
				t.Errorf("SizeM = %v, want %v", got.SizeM, tt.wantSize)
			}
			if !floatEquals(got.HorizPrecM, tt.wantHoriz, tt.tolerance) {
				t.Errorf("HorizPrecM = %v, want %v", got.HorizPrecM, tt.wantHoriz)
			}
			if !floatEquals(got.VertPrecM, tt.wantVert, tt.tolerance) {
				t.Errorf("VertPrecM = %v, want %v", got.VertPrecM, tt.wantVert)
			}
		})
	}
}

func TestParseLOCRecordLenient(t *testing.T) {
	tests := []struct {
		name      string
		fqdn      string
		raw       string
		wantLat   float64
		wantLon   float64
		wantErr   bool
		tolerance float64
	}{
		{
			// Standard format should work
			name:      "standard format via lenient",
			fqdn:      "test.example",
			raw:       "32 53 1.000 N 117 14 25.000 W 107.00m 30m 10m 10m",
			wantLat:   32.883611111,
			wantLon:   -117.240277778,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Format with extra whitespace
			name:      "extra whitespace",
			fqdn:      "test.example",
			raw:       "  32 53 1.000 N 117 14 25.000 W 107.00m 30m 10m 10m  ",
			wantLat:   32.883611111,
			wantLon:   -117.240277778,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Completely invalid should still error
			name:    "completely invalid",
			fqdn:    "bad.example",
			raw:     "this is not a loc record at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecordLenient(tt.fqdn, tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLOCRecordLenient() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLOCRecordLenient() unexpected error: %v", err)
				return
			}

			if !floatEquals(got.Latitude, tt.wantLat, tt.tolerance) {
				t.Errorf("Latitude = %v, want %v", got.Latitude, tt.wantLat)
			}
			if !floatEquals(got.Longitude, tt.wantLon, tt.tolerance) {
				t.Errorf("Longitude = %v, want %v", got.Longitude, tt.wantLon)
			}
		})
	}
}

func TestDMSToDecimal(t *testing.T) {
	// Test the DMS to decimal conversion logic embedded in ParseLOCRecord
	// by checking specific coordinate conversions

	tests := []struct {
		name      string
		raw       string
		wantLat   float64
		wantLon   float64
		tolerance float64
	}{
		{
			// 45 degrees exactly
			name:      "45 degrees north",
			raw:       "45 0 0.000 N 90 0 0.000 W 0.00m 1m 1m 1m",
			wantLat:   45.0,
			wantLon:   -90.0,
			tolerance: 0.0001,
		},
		{
			// 30 minutes = 0.5 degrees
			name:      "half degree test",
			raw:       "45 30 0.000 N 90 30 0.000 E 0.00m 1m 1m 1m",
			wantLat:   45.5,
			wantLon:   90.5,
			tolerance: 0.0001,
		},
		{
			// 1 second = 1/3600 = 0.000277... degrees
			name:      "one second test",
			raw:       "0 0 1.000 N 0 0 1.000 E 0.00m 1m 1m 1m",
			wantLat:   1.0 / 3600.0,
			wantLon:   1.0 / 3600.0,
			tolerance: 0.000001,
		},
		{
			// Combined: 1 degree + 1 minute + 1 second
			name:      "combined dms",
			raw:       "1 1 1.000 N 1 1 1.000 E 0.00m 1m 1m 1m",
			wantLat:   1.0 + 1.0/60.0 + 1.0/3600.0,
			wantLon:   1.0 + 1.0/60.0 + 1.0/3600.0,
			tolerance: 0.000001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecord("test.example", tt.raw)
			if err != nil {
				t.Fatalf("ParseLOCRecord() error: %v", err)
			}

			if !floatEquals(got.Latitude, tt.wantLat, tt.tolerance) {
				t.Errorf("Latitude = %v, want %v", got.Latitude, tt.wantLat)
			}
			if !floatEquals(got.Longitude, tt.wantLon, tt.tolerance) {
				t.Errorf("Longitude = %v, want %v", got.Longitude, tt.wantLon)
			}
		})
	}
}

func floatEquals(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestParseLOCRecord_BoundaryValues(t *testing.T) {
	// Test geographic boundary values
	tests := []struct {
		name      string
		raw       string
		wantLat   float64
		wantLon   float64
		wantErr   bool
		tolerance float64
	}{
		{
			// Maximum latitude - North Pole
			name:      "north pole",
			raw:       "90 0 0.000 N 0 0 0.000 E 0.00m 1m 1m 1m",
			wantLat:   90.0,
			wantLon:   0.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Maximum latitude - South Pole
			name:      "south pole",
			raw:       "90 0 0.000 S 0 0 0.000 W 0.00m 1m 1m 1m",
			wantLat:   -90.0,
			wantLon:   0.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Maximum longitude - International Date Line (East)
			name:      "date line east",
			raw:       "0 0 0.000 N 180 0 0.000 E 0.00m 1m 1m 1m",
			wantLat:   0.0,
			wantLon:   180.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Maximum longitude - International Date Line (West)
			name:      "date line west",
			raw:       "0 0 0.000 N 180 0 0.000 W 0.00m 1m 1m 1m",
			wantLat:   0.0,
			wantLon:   -180.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Extreme altitude - Challenger Deep (deepest point)
			name:      "challenger deep",
			raw:       "11 22 0.000 N 142 35 0.000 E -10994.00m 1m 1m 1m",
			wantLat:   11.366666667,
			wantLon:   142.583333333,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Extreme altitude - above cruising altitude
			name:      "high altitude",
			raw:       "27 59 17.000 N 86 55 31.000 E 8848.00m 1m 1m 1m",
			wantLat:   27.988055556,  // Mt. Everest approximate
			wantLon:   86.925277778,
			wantErr:   false,
			tolerance: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecord("test.example", tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLOCRecord() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLOCRecord() unexpected error: %v", err)
				return
			}

			if !floatEquals(got.Latitude, tt.wantLat, tt.tolerance) {
				t.Errorf("Latitude = %v, want %v", got.Latitude, tt.wantLat)
			}
			if !floatEquals(got.Longitude, tt.wantLon, tt.tolerance) {
				t.Errorf("Longitude = %v, want %v", got.Longitude, tt.wantLon)
			}
		})
	}
}

func TestParseLOCRecordLenient_Fallback(t *testing.T) {
	// Test cases where strict parsing fails but lenient succeeds
	tests := []struct {
		name      string
		raw       string
		wantLat   float64
		wantLon   float64
		wantAlt   float64
		wantErr   bool
		tolerance float64
	}{
		{
			// Missing precision fields - lenient should extract coordinates
			name:      "coordinates only no precision",
			raw:       "52 22 23.000 N 4 53 32.000 E -2.00m",
			wantLat:   52.373055556,
			wantLon:   4.892222222,
			wantAlt:   -2.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Extra text after the record
			name:      "extra trailing text",
			raw:       "52 22 23.000 N 4 53 32.000 E 0.00m 1m 1m 1m ; comment",
			wantLat:   52.373055556,
			wantLon:   4.892222222,
			wantAlt:   0.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
		{
			// Tabs instead of spaces - \s in regex matches any whitespace including tabs
			name:      "tabs as separators",
			raw:       "52\t22\t23.000\tN\t4\t53\t32.000\tE\t0.00m\t1m\t1m\t1m",
			wantLat:   52.373055556,
			wantLon:   4.892222222,
			wantAlt:   0.0,
			wantErr:   false,
			tolerance: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecordLenient("test.example", tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseLOCRecordLenient() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseLOCRecordLenient() unexpected error: %v", err)
				return
			}

			if !floatEquals(got.Latitude, tt.wantLat, tt.tolerance) {
				t.Errorf("Latitude = %v, want %v", got.Latitude, tt.wantLat)
			}
			if !floatEquals(got.Longitude, tt.wantLon, tt.tolerance) {
				t.Errorf("Longitude = %v, want %v", got.Longitude, tt.wantLon)
			}
			if !floatEquals(got.AltitudeM, tt.wantAlt, tt.tolerance) {
				t.Errorf("AltitudeM = %v, want %v", got.AltitudeM, tt.wantAlt)
			}
		})
	}
}

func TestParseLOCRecord_MeterSuffixVariations(t *testing.T) {
	// The regex allows optional 'm' suffix on size/horiz/vert fields
	// Test that both formats work
	tests := []struct {
		name      string
		raw       string
		wantSize  float64
		wantHoriz float64
		wantVert  float64
		tolerance float64
	}{
		{
			name:      "all fields with m suffix",
			raw:       "0 0 0.000 N 0 0 0.000 E 0.00m 10m 100m 10m",
			wantSize:  10.0,
			wantHoriz: 100.0,
			wantVert:  10.0,
			tolerance: 0.0001,
		},
		{
			name:      "size without m suffix",
			raw:       "0 0 0.000 N 0 0 0.000 E 0.00m 10 100m 10m",
			wantSize:  10.0,
			wantHoriz: 100.0,
			wantVert:  10.0,
			tolerance: 0.0001,
		},
		{
			name:      "all precision fields without m suffix",
			raw:       "0 0 0.000 N 0 0 0.000 E 0.00m 10 100 10",
			wantSize:  10.0,
			wantHoriz: 100.0,
			wantVert:  10.0,
			tolerance: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLOCRecord("test.example", tt.raw)
			if err != nil {
				t.Fatalf("ParseLOCRecord() error: %v", err)
			}

			if !floatEquals(got.SizeM, tt.wantSize, tt.tolerance) {
				t.Errorf("SizeM = %v, want %v", got.SizeM, tt.wantSize)
			}
			if !floatEquals(got.HorizPrecM, tt.wantHoriz, tt.tolerance) {
				t.Errorf("HorizPrecM = %v, want %v", got.HorizPrecM, tt.wantHoriz)
			}
			if !floatEquals(got.VertPrecM, tt.wantVert, tt.tolerance) {
				t.Errorf("VertPrecM = %v, want %v", got.VertPrecM, tt.wantVert)
			}
		})
	}
}

func TestParseLOCRecord_PreservesRawRecord(t *testing.T) {
	// Verify that RawRecord field contains the original (trimmed) input
	raw := "  52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m  "
	expected := "52 22 23.000 N 4 53 32.000 E -2.00m 1m 10000m 10m"

	got, err := ParseLOCRecord("test.example", raw)
	if err != nil {
		t.Fatalf("ParseLOCRecord() error: %v", err)
	}

	if got.RawRecord != expected {
		t.Errorf("RawRecord = %q, want %q", got.RawRecord, expected)
	}

	if got.FQDN != "test.example" {
		t.Errorf("FQDN = %q, want %q", got.FQDN, "test.example")
	}
}

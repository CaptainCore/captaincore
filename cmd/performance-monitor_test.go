package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseMonitorLogBackwardsCompatAndPHPProbe(t *testing.T) {
	log := strings.Join([]string{
		"Time | DB Conns | Load | HTTPS Code | HTTPS Time | Workers | PHP Probe",
		"-----|---------|------|-----------|----------|--------|----------",
		"10:30:00 | 3 | 12.10 | 200 0.005123s",
		"10:30:30 | 5 | 14.20 | 200 0.006000s | 4/16",
		"10:31:00 | 6 | 15.00 | 200 0.004500s | 6/16 | 200 0.012s",
		"10:31:30 | 17 | 20.50 | 200 0.005000s | 16/16 | 000 10.000s",
	}, "\n")

	raw := parseMonitorLogRaw(log, 0)
	if len(raw.Samples) != 4 {
		t.Fatalf("expected 4 raw samples, got %d", len(raw.Samples))
	}
	if raw.MaxWorkers != 16 {
		t.Fatalf("expected max_workers=16, got %d", raw.MaxWorkers)
	}
	if raw.Samples[0].PHPResp != nil || raw.Samples[1].PHPResp != nil {
		t.Fatalf("legacy lines should leave php_resp nil")
	}
	if raw.Samples[2].PHPCode == nil || *raw.Samples[2].PHPCode != 200 || raw.Samples[2].PHPResp == nil || *raw.Samples[2].PHPResp != 0.012 {
		t.Fatalf("unexpected sample[2] php probe: %+v", raw.Samples[2])
	}
	if raw.Samples[3].PHPCode == nil || *raw.Samples[3].PHPCode != 0 || raw.Samples[3].PHPResp == nil || *raw.Samples[3].PHPResp != 10.0 {
		t.Fatalf("unexpected sample[3] php probe: %+v", raw.Samples[3])
	}

	agg := parseMonitorLog(log)
	if agg.TotalSamples != 4 {
		t.Fatalf("expected 4 agg samples, got %d", agg.TotalSamples)
	}
	if agg.TotalPHPTimeouts != 1 {
		t.Fatalf("expected 1 php timeout, got %d", agg.TotalPHPTimeouts)
	}
	if agg.PeakPHPResponse != 10.0 {
		t.Fatalf("expected peak php response 10.0, got %v", agg.PeakPHPResponse)
	}
	if agg.AvgPHPResponse != 5.006 {
		t.Fatalf("expected avg php response 5.006, got %v", agg.AvgPHPResponse)
	}
}

func TestParseMonitorLogRawPreservesPHPSpikesWhenDownsampling(t *testing.T) {
	var lines []string
	for i := 0; i < 800; i++ {
		hh := (i / 120) % 24
		mm := (i / 2) % 60
		ss := (i % 2) * 30
		php := "200 0.010s"
		if i == 400 {
			php = "000 10.000s"
		}
		lines = append(lines, fmt.Sprintf("%02d:%02d:%02d | 4 | 10.00 | 200 0.005000s | 4/16 | %s", hh, mm, ss, php))
	}
	raw := parseMonitorLogRaw(strings.Join(lines, "\n"), 0)
	foundTimeout := false
	for _, s := range raw.Samples {
		if s.PHPCode != nil && *s.PHPCode == 0 && s.PHPResp != nil && *s.PHPResp == 10.0 {
			foundTimeout = true
			break
		}
	}
	if !foundTimeout {
		t.Fatalf("downsampling dropped the PHP timeout spike (total kept=%d)", len(raw.Samples))
	}
}

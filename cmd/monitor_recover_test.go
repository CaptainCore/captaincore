package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRecords(t *testing.T, records []MonitorRecord) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "monitor.json")
	data, _ := json.Marshal(records)
	if err := os.WriteFile(file, data, 0644); err != nil {
		t.Fatal(err)
	}
	return file
}

func readRecords(t *testing.T, file string) map[string]MonitorRecord {
	t.Helper()
	var records []MonitorRecord
	data, _ := os.ReadFile(file)
	json.Unmarshal(data, &records)
	out := map[string]MonitorRecord{}
	for _, r := range records {
		out[r.Name] = r
	}
	return out
}

func stubLaunch(t *testing.T) *[]string {
	t.Helper()
	var launched []string
	prev := launchRecover
	launchRecover = func(r MonitorRecord, logsPath string) error {
		launched = append(launched, r.Name)
		return nil
	}
	t.Cleanup(func() { launchRecover = prev; monitorNoRecovery = false })
	return &launched
}

func TestMonitorQueueRecoveriesEligibility(t *testing.T) {
	launched := stubLaunch(t)
	now := time.Now().Unix()
	file := writeRecords(t, []MonitorRecord{
		{Name: "one-check", HTTPCode: "000", CheckCount: 1, CreatedAt: now - 300},
		{Name: "fresh", HTTPCode: "000", CheckCount: 2, CreatedAt: now - 600},
		{Name: "too-soon", HTTPCode: "504", CheckCount: 9, CreatedAt: now - 3000, RecoveryAttempts: 1, LastRecoveryAt: now - 1800},
		{Name: "due-again", HTTPCode: "504", CheckCount: 30, CreatedAt: now - 9000, RecoveryAttempts: 1, LastRecoveryAt: now - 7200},
		{Name: "exhausted", HTTPCode: "000", CheckCount: 60, CreatedAt: now - 20000, RecoveryAttempts: 3, LastRecoveryAt: now - 7200},
		{Name: "old-outage", HTTPCode: "000", CheckCount: 500, CreatedAt: now - 2*86400},
		{Name: "redirect", HTTPCode: "301", CheckCount: 5, CreatedAt: now - 600},
	})

	monitorQueueRecoveries(file, 7, "", "")

	want := map[string]bool{"fresh": true, "due-again": true}
	if len(*launched) != len(want) {
		t.Fatalf("launched %v, want %v", *launched, want)
	}
	for _, name := range *launched {
		if !want[name] {
			t.Errorf("unexpected launch for %s", name)
		}
	}
	got := readRecords(t, file)
	if got["fresh"].RecoveryAttempts != 1 || got["fresh"].LastRecoveryAt == 0 {
		t.Errorf("fresh not recorded: %+v", got["fresh"])
	}
	if got["due-again"].RecoveryAttempts != 2 {
		t.Errorf("due-again attempts = %d, want 2", got["due-again"].RecoveryAttempts)
	}
	if got["too-soon"].RecoveryAttempts != 1 || got["exhausted"].RecoveryAttempts != 3 {
		t.Errorf("throttled records changed: %+v %+v", got["too-soon"], got["exhausted"])
	}
}

func TestMonitorQueueRecoveriesGuards(t *testing.T) {
	now := time.Now().Unix()
	var many []MonitorRecord
	for i := 0; i < 8; i++ {
		many = append(many, MonitorRecord{Name: string(rune('a' + i)), HTTPCode: "000", CheckCount: 2, CreatedAt: now - 600})
	}

	t.Run("caps launches per run", func(t *testing.T) {
		launched := stubLaunch(t)
		monitorQueueRecoveries(writeRecords(t, many), 8, "", "")
		if len(*launched) != recoveryMaxSitesPerRun {
			t.Errorf("launched %d, want %d", len(*launched), recoveryMaxSitesPerRun)
		}
	})
	t.Run("skips a mass outage", func(t *testing.T) {
		launched := stubLaunch(t)
		monitorQueueRecoveries(writeRecords(t, many), recoveryMassOutage+1, "", "")
		if len(*launched) != 0 {
			t.Errorf("launched %v during a mass outage", *launched)
		}
	})
	t.Run("config off", func(t *testing.T) {
		launched := stubLaunch(t)
		monitorQueueRecoveries(writeRecords(t, many), 8, "", "off")
		if len(*launched) != 0 {
			t.Errorf("launched %v with monitor_auto_recovery off", *launched)
		}
	})
	t.Run("flag off", func(t *testing.T) {
		launched := stubLaunch(t)
		monitorNoRecovery = true
		monitorQueueRecoveries(writeRecords(t, many), 8, "", "")
		if len(*launched) != 0 {
			t.Errorf("launched %v with --no-recovery", *launched)
		}
	})
}

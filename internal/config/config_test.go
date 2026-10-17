package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.PollInterval != time.Minute || c.ContinuityGap != 5*time.Minute || c.StaleAfter != 3*time.Minute {
		t.Errorf("timing defaults: %+v", c)
	}
	if c.FetchMode != FetchModeDirect || c.UpstreamURL != DefaultUpstreamURL || c.DatabasePath != "/data/pwr-tracker.db" {
		t.Errorf("defaults: %+v", c)
	}
	if c.AnalyticsTimezone.String() != "Europe/Vilnius" || c.ObservationRetention != 90*24*time.Hour || c.StoreRawPayloads {
		t.Errorf("defaults: %+v", c)
	}
}

func TestDerivedDefaultsFollowInterval(t *testing.T) {
	c, err := Load(env(map[string]string{"POLL_INTERVAL": "2m"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ContinuityGap != 10*time.Minute || c.StaleAfter != 6*time.Minute {
		t.Errorf("derived: gap=%s stale=%s", c.ContinuityGap, c.StaleAfter)
	}
}

func TestValidationCollectsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{
		"POLL_INTERVAL":      "soon",
		"FETCH_MODE":         "auto",
		"ANALYTICS_TIMEZONE": "Mars/Base",
		"UPSTREAM_URL":       "ftp://x",
		"STORE_RAW_PAYLOADS": "maybe",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"POLL_INTERVAL", "FETCH_MODE", "ANALYTICS_TIMEZONE", "UPSTREAM_URL", "STORE_RAW_PAYLOADS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestFlareSolverrModeRequiresURL(t *testing.T) {
	_, err := Load(env(map[string]string{"FETCH_MODE": "flaresolverr"}))
	if err == nil || !strings.Contains(err.Error(), "FLARESOLVERR_URL") {
		t.Fatalf("got %v", err)
	}
	c, err := Load(env(map[string]string{"FETCH_MODE": "FlareSolverr", "FLARESOLVERR_URL": "http://flaresolverr:8191"}))
	if err != nil || c.FetchMode != FetchModeFlareSolverr {
		t.Fatalf("got %v %+v", err, c)
	}
}

func TestGapMustCoverInterval(t *testing.T) {
	_, err := Load(env(map[string]string{"POLL_INTERVAL": "60s", "CONTINUITY_GAP": "30s"}))
	if err == nil || !strings.Contains(err.Error(), "CONTINUITY_GAP") {
		t.Fatalf("got %v", err)
	}
}

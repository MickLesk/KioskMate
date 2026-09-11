package systemtime

import "testing"

func TestApplyTimesyncProperties(t *testing.T) {
	status := Status{Server: "configured.example"}
	applyTimesyncProperties(&status, "ServerName=ntp.example\nServerAddress=192.0.2.5\nPollIntervalUSec=32s\nRootDistanceMaxUSec=5s\n")
	if status.Server != "ntp.example" || status.ServerAddress != "192.0.2.5" || status.PollIntervalSeconds != 32 || status.RootDistanceMS != 5000 {
		t.Fatalf("status = %#v", status)
	}
}

func TestSystemdDuration(t *testing.T) {
	if got := systemdDuration("1min").Seconds(); got != 60 {
		t.Fatalf("duration = %v", got)
	}
}

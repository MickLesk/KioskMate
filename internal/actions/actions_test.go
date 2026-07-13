package actions

import (
	"testing"
	"time"
)

func TestJobWriterStreamsCompleteLines(t *testing.T) {
	job := &Job{ExitCode: -1}
	writer := &jobWriter{job: job}
	_, _ = writer.Write([]byte("first\nsec"))
	_, _ = writer.Write([]byte("ond\n"))
	writer.flush()
	if len(job.Output) != 2 || job.Output[0] != "first" || job.Output[1] != "second" {
		t.Fatalf("output = %#v", job.Output)
	}
}

func TestJobsReturnsNewestFirst(t *testing.T) {
	service := &Service{jobs: map[string]*Job{
		"old": {ID: "old", Started: time.Unix(1, 0)},
		"new": {ID: "new", Started: time.Unix(2, 0)},
	}}
	jobs := service.Jobs(1)
	if len(jobs) != 1 || jobs[0].ID != "new" {
		t.Fatalf("jobs = %#v", jobs)
	}
}

func TestResolvePrivilegeUsesTemporaryCredential(t *testing.T) {
	service := &Service{}
	if err := service.RememberPrivilege("sudo", "secret"); err != nil {
		t.Fatal(err)
	}
	mode, password, configured := service.ResolvePrivilege("", "")
	if mode != "sudo" || password != "secret" || !configured {
		t.Fatalf("mode = %q, password = %q, configured = %v", mode, password, configured)
	}
}

func TestResolvePrivilegeDefaultsToPasswordlessSudo(t *testing.T) {
	service := &Service{}
	mode, password, configured := service.ResolvePrivilege("", "")
	if mode != "sudo" || password != "" || !configured {
		t.Fatalf("mode = %q, password = %q, configured = %v", mode, password, configured)
	}
}

package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
	"github.com/rakyll/ate-watch/internal/watcher"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestStatusName(t *testing.T) {
	tests := []struct {
		status   ateapipb.Actor_Status
		expected string
	}{
		{ateapipb.Actor_STATUS_UNSPECIFIED, "UNSPECIFIED"},
		{ateapipb.Actor_STATUS_RUNNING, "RUNNING"},
		{ateapipb.Actor_STATUS_SUSPENDING, "SUSPENDING"},
		{ateapipb.Actor_STATUS_SUSPENDED, "SUSPENDED"},
		{ateapipb.Actor_STATUS_RESUMING, "RESUMING"},
		{ateapipb.Actor_STATUS_PAUSING, "PAUSING"},
		{ateapipb.Actor_STATUS_PAUSED, "PAUSED"},
		{ateapipb.Actor_STATUS_CRASHED, "CRASHED"},
	}

	for _, tc := range tests {
		got := StatusName(tc.status)
		if got != tc.expected {
			t.Errorf("StatusName(%v) = %q, want %q", tc.status, got, tc.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{5 * time.Second, "5s"},
		{45 * time.Second, "45s"},
		{60 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
		{120 * time.Minute, "2h"},
		{48 * time.Hour, "2d"},
	}

	for _, tc := range tests {
		got := FormatDuration(tc.d)
		if got != tc.expected {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.d, got, tc.expected)
		}
	}
}

func TestRenderTable(t *testing.T) {
	now := time.Date(2026, 8, 20, 22, 0, 0, 0, time.UTC)
	createTime := timestamppb.New(now.Add(-10 * time.Minute))

	snap := &watcher.Snapshot{
		Timestamp: now,
		Actors: []*watcher.ActorState{
			{
				Actor: &ateapipb.Actor{
					Metadata: &ateapipb.ResourceMetadata{
						Atespace:   "default",
						Name:       "env-1",
						Version:    3,
						CreateTime: createTime,
					},
					ActorTemplateNamespace: "ate-env",
					ActorTemplateName:      "default-env",
					Status:                 ateapipb.Actor_STATUS_RUNNING,
					AteomPodNamespace:     "ate-system",
					AteomPodName:          "pod-1",
					AteomPodIp:            "10.244.0.12",
				},
				StatusChangedAt: now.Add(-30 * time.Second),
			},
		},
		TotalCount: 1,
		CountByStatus: map[ateapipb.Actor_Status]int{
			ateapipb.Actor_STATUS_RUNNING: 1,
		},
	}

	var buf bytes.Buffer
	err := RenderTable(&buf, snap, RenderOptions{
		Atespace: "default",
		Interval: time.Second,
		UseColor: false,
	})
	if err != nil {
		t.Fatalf("RenderTable failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Every 1s: ate-watch (default)") {
		t.Errorf("missing header in output:\n%s", out)
	}
	if !strings.Contains(out, "default") || !strings.Contains(out, "env-1") || !strings.Contains(out, "RUNNING") {
		t.Errorf("missing actor details in output:\n%s", out)
	}
}

func TestBuildDescribeMarkdown(t *testing.T) {
	now := time.Date(2026, 8, 20, 22, 0, 0, 0, time.UTC)
	createTime := timestamppb.New(now.Add(-10 * time.Minute))

	actor := &ateapipb.Actor{
		Metadata: &ateapipb.ResourceMetadata{
			Atespace:   "team-a",
			Name:       "crawler-1",
			Uid:        "uid-12345",
			Version:    4,
			CreateTime: createTime,
		},
		ActorTemplateNamespace: "custom",
		ActorTemplateName:      "scraper",
		Status:                 ateapipb.Actor_STATUS_RUNNING,
		AteomPodNamespace:     "ate-system",
		AteomPodName:          "pod-42",
		AteomPodIp:            "10.244.1.5",
		WorkerPoolName:        "gpu-pool",
		InProgressSnapshot:     "snap-in-flight",
		LatestSnapshotInfo: &ateapipb.SnapshotInfo{
			Data: &ateapipb.SnapshotInfo_External{
				External: &ateapipb.ExternalSnapshotInfo{
					SnapshotUriPrefix: "gs://my-bucket/snapshots/snap-99",
				},
			},
		},
	}

	md := BuildDescribeMarkdown(actor)
	if !strings.Contains(md, "**team-a/crawler-1**") {
		t.Errorf("missing actor header in describe:\n%s", md)
	}
	if !strings.Contains(md, "custom/scraper") {
		t.Errorf("missing template in describe:\n%s", md)
	}
	if !strings.Contains(md, "ate-system/pod-42") {
		t.Errorf("missing worker pod in describe:\n%s", md)
	}
	if !strings.Contains(md, "gpu-pool") {
		t.Errorf("missing worker pool in describe:\n%s", md)
	}
	if !strings.Contains(md, "snap-in-flight") {
		t.Errorf("missing in progress snapshot in describe:\n%s", md)
	}
	if !strings.Contains(md, "* **Endpoint:** `crawler-1.team-a.atenet`") {
		t.Errorf("missing endpoint in describe:\n%s", md)
	}
	if !strings.Contains(md, "gs://my-bucket/snapshots/snap-99") {
		t.Errorf("missing latest snapshot in describe:\n%s", md)
	}
}

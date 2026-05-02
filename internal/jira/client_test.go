package jira

import (
	"testing"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
)

func TestIsStatusIn(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		statuses []string
		want     bool
	}{
		{"default Done matches", "Done", []string{"Done"}, true},
		{"non-done does not match", "In Progress", []string{"Done"}, false},
		{"awaiting matches when configured", "Awaiting Dev Deployment", []string{"Done", "Awaiting Dev Deployment"}, true},
		{"awaiting does not match when not configured", "Awaiting Dev Deployment", []string{"Done"}, false},
		{"case sensitive", "done", []string{"Done"}, false},
		{"empty list never matches", "Done", []string{}, false},
		{"In Development matches in-progress set", "In Development", []string{"In Progress", "In Development"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStatusIn(tc.status, tc.statuses); got != tc.want {
				t.Errorf("isStatusIn(%q, %v) = %v, want %v", tc.status, tc.statuses, got, tc.want)
			}
		})
	}
}

func TestConvertIssueNormalizesDoneEquivalentStatus(t *testing.T) {
	cfg := &config.Config{
		JIRA: config.JIRAConfig{
			DoneStatuses: []string{"Done", "Awaiting Dev Deployment"},
		},
	}
	c := &Client{}

	cases := []struct {
		name      string
		rawStatus string
		want      string
	}{
		{"Done stays Done", "Done", "Done"},
		{"Awaiting Dev Deployment normalizes to Done", "Awaiting Dev Deployment", "Done"},
		{"In Progress passes through", "In Progress", "In Progress"},
		{"In Development normalizes to In Progress", "In Development", "In Progress"},
		{"To Do passes through", "To Do", "To Do"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issue := Issue{Key: "TEST-1"}
			issue.Fields.Status.Name = tc.rawStatus
			got := c.convertIssue(issue, cfg)
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

func TestExtractStateTransitionsAnchorsOnFirstDoneEquivalent(t *testing.T) {
	c := &Client{}
	earlier := "2026-04-15T10:00:00.000-0700"
	later := "2026-04-20T10:00:00.000-0700"

	t.Run("Awaiting Dev Deployment anchors when configured", func(t *testing.T) {
		changelog := &Changelog{
			Histories: []History{
				{
					Created: "2026-04-10T10:00:00.000-0700",
					Items:   []ChangeItem{{Field: "status", ToString: "In Development"}},
				},
				{
					Created: earlier,
					Items:   []ChangeItem{{Field: "status", ToString: "Awaiting Dev Deployment"}},
				},
				{
					Created: later,
					Items:   []ChangeItem{{Field: "status", ToString: "Done"}},
				},
			},
		}

		inProgress := []string{"In Progress", "In Development"}
		_, doneTime := c.extractStateTransitions(changelog, inProgress, []string{"Done", "Awaiting Dev Deployment"})
		if doneTime == nil {
			t.Fatal("expected doneTime to be set")
		}
		want, _ := time.Parse("2006-01-02T15:04:05.000-0700", earlier)
		if !doneTime.Equal(want) {
			t.Errorf("doneTime = %v, want %v (first done-equivalent transition)", doneTime, want)
		}
	})

	t.Run("Done anchors when only Done is configured", func(t *testing.T) {
		changelog := &Changelog{
			Histories: []History{
				{
					Created: earlier,
					Items:   []ChangeItem{{Field: "status", ToString: "Awaiting Dev Deployment"}},
				},
				{
					Created: later,
					Items:   []ChangeItem{{Field: "status", ToString: "Done"}},
				},
			},
		}

		inProgress := []string{"In Progress", "In Development"}
		_, doneTime := c.extractStateTransitions(changelog, inProgress, []string{"Done"})
		if doneTime == nil {
			t.Fatal("expected doneTime to be set")
		}
		want, _ := time.Parse("2006-01-02T15:04:05.000-0700", later)
		if !doneTime.Equal(want) {
			t.Errorf("doneTime = %v, want %v (skip Awaiting since not configured)", doneTime, want)
		}
	})

	t.Run("nil changelog returns nil times", func(t *testing.T) {
		inProgress, done := c.extractStateTransitions(nil, []string{"In Progress"}, []string{"Done"})
		if inProgress != nil || done != nil {
			t.Errorf("expected (nil, nil), got (%v, %v)", inProgress, done)
		}
	})

	t.Run("In Progress and In Development both anchor inProgressTime", func(t *testing.T) {
		changelog := &Changelog{
			Histories: []History{
				{
					Created: earlier,
					Items:   []ChangeItem{{Field: "status", ToString: "In Development"}},
				},
			},
		}
		inProgress, _ := c.extractStateTransitions(changelog, []string{"In Progress", "In Development"}, []string{"Done"})
		if inProgress == nil {
			t.Fatal("expected inProgressTime to be set for In Development transition")
		}
	})

	t.Run("custom in-progress status is honored", func(t *testing.T) {
		changelog := &Changelog{
			Histories: []History{
				{
					Created: earlier,
					Items:   []ChangeItem{{Field: "status", ToString: "Coding"}},
				},
			},
		}
		inProgress, _ := c.extractStateTransitions(changelog, []string{"Coding"}, []string{"Done"})
		if inProgress == nil {
			t.Fatal("expected custom in-progress status to anchor inProgressTime")
		}
	})
}

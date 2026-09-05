package dispatched

// extract.go: tasks-YAML document decoding and the snapshot record. Journal
// parsing lives in journal.go, filesystem and Git reads in sources.go, and
// evidence joining in evidence.go (FC-SCAFFOLD F1–F4 amendment).

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type yamlTask struct {
	Key             string `yaml:"key"`
	Role            Role   `yaml:"role"`
	Model           string `yaml:"model"`
	Status          string `yaml:"status"`
	StartedAt       string `yaml:"started_at"`
	CompletedAt     string `yaml:"completed_at"`
	IterationCount  int    `yaml:"iteration_count"`
	DispatcherRunID string `yaml:"dispatcher_run_id"`
}

type yamlDocument struct {
	Tasks []yamlTask `yaml:"tasks"`
}

type taskSnapshot struct {
	Key             string
	Role            Role
	AuthoredModel   string
	Status          string
	StartedAt       time.Time
	CompletedAt     time.Time
	IterationCount  int
	DispatcherRunID string
	Revision        Revision
	Repository      string
	Path            string
	// SourceID names the SourceSpec the reading came from; empty on the
	// baseline readers, which predate explicit sources.
	SourceID string
}

// Ref is the reading reference of the snapshot.
func (s taskSnapshot) Ref() ReadingRef {
	return ReadingRef{SourceID: s.SourceID, Repository: s.Repository, Path: s.Path, Revision: s.Revision}
}

// RowFields records which join-key fields were PRESENT (non-empty) in the
// raw YAML row, independently of whether they parsed. It lets the join tell
// a missing started_at (DispositionMissingJoinKeys) from a malformed one
// (DispositionMalformed), which collapse to the same zero time.Time on a
// parsed snapshot.
type RowFields struct {
	Key       bool
	RunID     bool
	StartedAt bool
}

// Complete reports whether every join key was present.
func (f RowFields) Complete() bool { return f.Key && f.RunID && f.StartedAt }

// Reading is the envelope for one discovered tasks-YAML row, whether or not
// it parsed (F3: every examined snapshot receives a Disposition). Ref names
// the reading; Row is the 1-based position in the document's tasks sequence,
// 0 when the document itself did not decode; Present is raw-field presence;
// Snapshot is usable only when Err is nil. Err is the parse failure for a
// row or document that could not be decoded; it is never a read failure,
// which is counted on the source instead.
//
// JoinEvidence classifies from the envelope alone: Err != nil is
// DispositionMalformed; Err == nil and !Present.Complete() is
// DispositionMissingJoinKeys; otherwise the snapshot is joined.
type Reading struct {
	Ref      ReadingRef
	Row      int
	Present  RowFields
	Snapshot taskSnapshot
	Err      error
}

// parseReadings reads one YAML document into one Reading per row, keeping
// malformed rows and an undecodable document as envelopes with Err set so
// the join can count them. It is the amended counterpart of parseSnapshots,
// which the baseline path keeps; ReadSources (FC-SOURCES) calls this one.
// Selection exclusions are not applied here.
func parseReadings(data []byte, ref ReadingRef) []Reading {
	doc, err := decodeTaskDocument(data)
	if err != nil {
		return []Reading{{Ref: ref, Err: fmt.Errorf("document: %w", err)}}
	}
	out := make([]Reading, 0, len(doc.Tasks))
	for i, task := range doc.Tasks {
		reading := Reading{
			Ref: ref, Row: i + 1,
			Present: RowFields{Key: task.Key != "", RunID: task.DispatcherRunID != "", StartedAt: task.StartedAt != ""},
			Snapshot: taskSnapshot{
				Key: task.Key, Role: task.Role, AuthoredModel: task.Model,
				Status: task.Status, IterationCount: task.IterationCount,
				DispatcherRunID: task.DispatcherRunID, Revision: ref.Revision,
				Repository: ref.Repository, Path: ref.Path, SourceID: ref.SourceID,
			},
		}
		if task.StartedAt != "" {
			started, err := time.Parse(time.RFC3339Nano, task.StartedAt)
			if err != nil {
				reading.Err = fmt.Errorf("row %d started_at %q: %w", i+1, task.StartedAt, err)
				out = append(out, reading)
				continue
			}
			reading.Snapshot.StartedAt = started
		}
		if task.CompletedAt != "" {
			completed, err := time.Parse(time.RFC3339Nano, task.CompletedAt)
			if err != nil {
				reading.Err = fmt.Errorf("row %d completed_at %q: %w", i+1, task.CompletedAt, err)
				out = append(out, reading)
				continue
			}
			reading.Snapshot.CompletedAt = completed
		}
		out = append(out, reading)
	}
	return out
}

// yamlSources is what one YAML source yielded, plus what it could not yield.
// A document that is not a task list, and a row with an unreadable timestamp,
// are counted and skipped: features/ holds YAML that is not a tasks file, and
// one such file must not abort the union.
type yamlSources struct {
	Snapshots            []taskSnapshot
	Documents            int
	UnparseableDocuments int
	MalformedRows        int
	MissingJoinKeys      int
}

func (s *yamlSources) absorb(other yamlSources) {
	s.Snapshots = append(s.Snapshots, other.Snapshots...)
	s.Documents += other.Documents
	s.UnparseableDocuments += other.UnparseableDocuments
	s.MalformedRows += other.MalformedRows
	s.MissingJoinKeys += other.MissingJoinKeys
}

// parseSnapshots reads one YAML document. It never returns an error: an
// unreadable document or row is reported through the returned counters so the
// caller can name the shortfall instead of failing the whole build.
func parseSnapshots(data []byte, revision Revision) yamlSources {
	out := yamlSources{Documents: 1}
	doc, err := decodeTaskDocument(data)
	if err != nil {
		out.UnparseableDocuments++
		return out
	}
	for _, task := range doc.Tasks {
		if task.Key == "" || task.DispatcherRunID == "" || task.StartedAt == "" {
			out.MissingJoinKeys++
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, task.StartedAt)
		if err != nil {
			out.MalformedRows++
			continue
		}
		var completed time.Time
		if task.CompletedAt != "" {
			completed, err = time.Parse(time.RFC3339Nano, task.CompletedAt)
			if err != nil {
				out.MalformedRows++
				continue
			}
		}
		out.Snapshots = append(out.Snapshots, taskSnapshot{
			Key: task.Key, Role: task.Role, AuthoredModel: task.Model,
			Status: task.Status, StartedAt: started, CompletedAt: completed,
			IterationCount: task.IterationCount, DispatcherRunID: task.DispatcherRunID,
			Revision: revision,
		})
	}
	return out
}

func isTaskYAML(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func decodeTaskDocument(data []byte) (yamlDocument, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return yamlDocument{}, err
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return yamlDocument{}, fmt.Errorf("expected a mapping with a tasks sequence")
	}
	mapping := root.Content[0]
	found := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "tasks" {
			if mapping.Content[i+1].Kind != yaml.SequenceNode {
				return yamlDocument{}, fmt.Errorf("tasks must be a sequence")
			}
			found = true
		}
	}
	if !found {
		return yamlDocument{}, fmt.Errorf("missing tasks sequence")
	}
	var doc yamlDocument
	if err := root.Decode(&doc); err != nil {
		return yamlDocument{}, err
	}
	return doc, nil
}

package dispatched

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testF1AmbiguousRefPermutation(t *testing.T) {
	// Duplicate-physical-citation diagnostic: two EventRefs share seq/line/type
	// and the same UTC instant but serialize different offsets. ReduceAttempts
	// cannot emit this shape. It is retained diagnostic output, not a sample.
	start := mustTime(t, "2026-01-01T00:00:00Z")
	id := NewAttemptID("run-a", "KC", start)
	journal := JournalIdentity{RunID: "run-a", SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	at := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	east := at.In(time.FixedZone("EAST", 2*3600))
	if !at.Equal(east) {
		t.Fatal("fixture instants are not equivalent")
	}
	if at.Format(time.RFC3339Nano) == east.Format(time.RFC3339Nano) {
		t.Fatal("fixture offsets serialized identically")
	}
	utcRef := EventRef{Journal: journal, Type: EventTaskStarted, At: at.UTC(), HasSeq: true, Seq: 1, Line: 2}
	offsetRef := EventRef{Journal: journal, Type: EventTaskStarted, At: east, HasSeq: true, Seq: 1, Line: 2}

	orders := [][]EventRef{
		{utcRef, offsetRef},
		{offsetRef, utcRef},
	}
	var encoded [][]byte
	for i, refs := range orders {
		t.Run(fmt.Sprintf("order-%d", i), func(t *testing.T) {
			set := attemptSetOf("run-a")
			item := AmbiguousAttempt{ID: id, Starts: 2, Refs: append([]EventRef{}, refs...)}
			set.Ambiguous = []AmbiguousAttempt{item}
			sets := []AttemptSet{set}
			beforeSets := encodeJSON(t, sets)
			beforeRefs := encodeJSON(t, set.Ambiguous[0].Refs)
			got, err := joinContract(t, sets, nil, defaultSelection(), identityUniverse(set))
			if !errors.Is(err, ErrAmbiguousAttempt) {
				t.Fatalf("JoinEvidence err = %v, want ErrAmbiguousAttempt", err)
			}
			if !bytes.Equal(beforeSets, encodeJSON(t, sets)) || !bytes.Equal(beforeRefs, encodeJSON(t, set.Ambiguous[0].Refs)) {
				t.Fatal("JoinEvidence mutated caller-owned ambiguous refs or sets")
			}
			if len(got.Ambiguous) != 1 || got.Ambiguous[0].Starts != 2 || len(got.Ambiguous[0].Refs) != 2 {
				t.Fatalf("ambiguous retained Starts/Refs = %+v", got.Ambiguous)
			}
			if got.Recovered != 0 || len(got.Observations) != 0 {
				t.Fatalf("ambiguous diagnostic recovered a sample: %+v", got)
			}
			if got.Attempts != 1 || len(got.LostAttempts) != 1 || got.LostAttempts[0] != id {
				t.Fatalf("lost/ambiguous denominator attempts=%d lost=%+v", got.Attempts, got.LostAttempts)
			}
			encoded = append(encoded, encodeJSON(t, got))
		})
	}
	t.Run("whole-join-json", func(t *testing.T) {
		if len(encoded) != 2 || !bytes.Equal(encoded[0], encoded[1]) {
			t.Fatal("EvidenceJoin JSON depended on offset-equivalent ambiguous ref order")
		}
	})
}

func requireDiagnosticComponents(t *testing.T, art Artifact, value string, fieldTokens, ruleTokens []string) {
	t.Helper()
	for _, field := range fieldTokens {
		for _, rule := range ruleTokens {
			if field != "" && strings.EqualFold(field, rule) {
				t.Fatalf("field token %q also appears in rule tokens; a field name cannot satisfy the rule check", field)
			}
		}
	}
	got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if got.Eligible {
		t.Fatal("invalid payload marked eligible")
	}
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("invalid payload refuse = %v, want ErrNotEligible and ErrSourceIncomplete", err)
	}
	blob := strings.ToLower(strings.Join(got.Reasons, "\n"))
	if value != "" && !strings.Contains(blob, strings.ToLower(value)) {
		t.Fatalf("reasons %q do not name stored value %q", got.Reasons, value)
	}
	if !reasonContainsAny(blob, fieldTokens) {
		t.Fatalf("reasons %q do not name an offending field among %v", got.Reasons, fieldTokens)
	}
	if !reasonContainsAny(blob, ruleTokens) {
		t.Fatalf("reasons %q do not name a violated rule among %v", got.Reasons, ruleTokens)
	}
}

func reasonContainsAny(blob string, tokens []string) bool {
	for _, token := range tokens {
		if token != "" && strings.Contains(blob, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

func testF4EligibleProvenanceDiagnostics(t *testing.T) {
	t.Run("drive-prefix", func(t *testing.T) {
		const yamlPath = "C:/outside/tasks.yaml"
		art := updateManifestSource(recoveredArtifact(2), "live", func(src *SourceReport) {
			src.Roots = []string{"."}
		})
		art = mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
			ref.Path = yamlPath
			return ref
		})
		requireDiagnosticComponents(t, art, yamlPath, []string{"path", "citation"}, []string{"drive", "portable", "prefix"})
	})

	t.Run("ghost-yaml-source", func(t *testing.T) {
		const ghost = "ghost-yaml"
		art := mapAllRecoveredYAML(recoveredArtifact(2), func(ref ReadingRef) ReadingRef {
			ref.SourceID = ghost
			return ref
		})
		requireDiagnosticComponents(t, art, ghost, []string{"source", "source_id", "sourceid"}, []string{"selected", "unknown", "missing", "not selected"})
	})

	t.Run("wrong-yaml-repository", func(t *testing.T) {
		const repo = "other-repo"
		art := mapAllRecoveredYAML(recoveredArtifact(2), func(ref ReadingRef) ReadingRef {
			ref.Repository = repo
			return ref
		})
		requireDiagnosticComponents(t, art, repo, []string{"repository"}, []string{"match", "selected", "mismatch"})
	})

	t.Run("yaml-path-out-of-root", func(t *testing.T) {
		const yamlPath = "dispatcher/tasks.yaml"
		art := mapAllRecoveredYAML(recoveredArtifact(2), func(ref ReadingRef) ReadingRef {
			ref.Path = yamlPath
			return ref
		})
		requireDiagnosticComponents(t, art, yamlPath, []string{"path"}, []string{"outside", "within", "declared"})
	})

	t.Run("unknown-journal-producer", func(t *testing.T) {
		const producer = "evil-producer-9"
		art := mapAllRecoveredJournals(recoveredArtifact(2), func(j JournalIdentity) JournalIdentity {
			j.Producer = producer
			return j
		})
		requireDiagnosticComponents(t, art, producer, []string{"producer"}, []string{"unsupported"})
	})

	t.Run("journal-path-not-direct-child", func(t *testing.T) {
		const journalPath = "run-a/nested/journal.jsonl"
		art := mapAllRecoveredJournals(recoveredArtifact(2), func(j JournalIdentity) JournalIdentity {
			j.Path = journalPath
			return j
		})
		requireDiagnosticComponents(t, art, journalPath, []string{"path"}, []string{"layout", "direct", "child"})
	})

	t.Run("selected-run-b-journal-on-run-a-attempt", func(t *testing.T) {
		other := JournalIdentity{
			RunID: "run-b", SourceID: "journals", Path: "run-b/journal.jsonl", Producer: ProducerDispatcherV0_1_0,
		}
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			ev := obs.Attempt.Evidence.Model
			ev.Event.Journal = other
			obs.Attempt.Evidence.Model = ev
		})
		att := art.Evidence.Observations[0].Attempt
		if att.Start.Journal.RunID != "run-a" || att.Start.Journal.Path != "run-a/journal.jsonl" ||
			att.Start.Journal.SourceID != "journals" || att.Start.Journal.Producer != ProducerDispatcherV0_1_0 {
			t.Fatalf("fixture changed attempt start journal: %+v", att.Start.Journal)
		}
		if att.ID.RunID != "run-a" || att.ID.Key != "KA" {
			t.Fatalf("fixture attempt identity = %s/%s, want run-a/KA", att.ID.RunID, att.ID.Key)
		}
		if att.Evidence.Model.Event.Journal != other {
			t.Fatalf("model journal = %+v, want structurally valid selected run-b identity", att.Evidence.Model.Event.Journal)
		}

		got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
		if got.Eligible {
			t.Fatal("invalid payload marked eligible")
		}
		if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
			t.Fatalf("invalid payload refuse = %v, want ErrNotEligible and ErrSourceIncomplete", err)
		}
		blob := strings.ToLower(strings.Join(got.Reasons, "\n"))
		if !strings.Contains(blob, "structured observation") {
			t.Fatalf("reasons %q lack structured-observation wrapper", got.Reasons)
		}
		if !strings.Contains(blob, "ka") {
			t.Fatalf("reasons %q do not name attempt key KA", got.Reasons)
		}
		start := recoveredStart().UTC()
		if !reasonContainsAny(blob, []string{strings.ToLower(start.Format(time.RFC3339)), strings.ToLower(start.Format(time.RFC3339Nano))}) {
			t.Fatalf("reasons %q do not name attempt start %s", got.Reasons, start.Format(time.RFC3339))
		}
		if !strings.Contains(blob, "run-a") {
			t.Fatalf("reasons %q do not name attempt/expected run-a", got.Reasons)
		}
		if !strings.Contains(blob, "model") {
			t.Fatalf("reasons %q do not name field model", got.Reasons)
		}
		for _, token := range []string{"actual", "expected"} {
			if !strings.Contains(blob, token) {
				t.Fatalf("reasons %q do not name %s journal identity", got.Reasons, token)
			}
		}
		if !reasonContainsAny(blob, []string{"source_id", "sourceid"}) {
			t.Fatalf("reasons %q do not name source_id", got.Reasons)
		}
		if !reasonContainsAny(blob, []string{"run_id", "runid"}) {
			t.Fatalf("reasons %q do not name run_id", got.Reasons)
		}
		if !strings.Contains(blob, "path") {
			t.Fatalf("reasons %q do not name path", got.Reasons)
		}
		if !strings.Contains(blob, "producer") {
			t.Fatalf("reasons %q do not name producer", got.Reasons)
		}
		if !strings.Contains(blob, "run-b") || !strings.Contains(blob, "run-b/journal.jsonl") {
			t.Fatalf("reasons %q do not name actual run-b journal", got.Reasons)
		}
		if !strings.Contains(blob, "run-a/journal.jsonl") {
			t.Fatalf("reasons %q do not name expected run-a journal path", got.Reasons)
		}
		if !strings.Contains(blob, "journals") || !strings.Contains(blob, strings.ToLower(ProducerDispatcherV0_1_0)) {
			t.Fatalf("reasons %q do not name source_id/producer values", got.Reasons)
		}
	})
}

func testF1ConflictIndependentTies(t *testing.T) {
	// EvidenceConflictCode is the only legal producer Code on a valid
	// normalized AttemptConflict. A second Code is not a retained legal
	// schema value, so it is not used as an independent tie-break fixture.
	start := mustTime(t, "2026-01-01T00:00:00Z")
	id := NewAttemptID("run-a", "KC", start)
	journal := JournalIdentity{RunID: "run-a", SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	matching := []Reading{syntheticReading("run-a", "KC", start, 1, "features/study/tasks.yaml")}

	modelEvent := func(seq, line int, at time.Time) EventRef {
		return EventRef{
			Journal: journal, Type: EventTaskSpawnFinished,
			At: at, HasSeq: true, Seq: seq, Line: line,
		}
	}
	modelCite := func(seq, line int, at time.Time) FieldEvidence {
		return FieldEvidence{Source: EvidenceJournal, Event: modelEvent(seq, line, at)}
	}
	yamlRef := func(row int, source string, recorded time.Time) ReadingRef {
		return ReadingRef{
			SourceID:   source,
			Repository: "repo",
			Path:       "features/study/tasks.yaml",
			Revision:   "live",
			Row:        row,
			RecordedAt: recorded,
		}
	}
	yamlCite := func(row int, source string, recorded time.Time) FieldEvidence {
		return FieldEvidence{Source: EvidenceYAML, Reading: yamlRef(row, source, recorded)}
	}
	aCite := modelCite(1, 2, start.Add(time.Minute))
	aVal := modelConflictValue("modelA")
	bCite := modelCite(5, 6, start.Add(2*time.Minute))
	baseModel := AttemptConflict{
		Code: EvidenceConflictCode, ID: id, Field: "model",
		AValue: aVal, BValue: modelConflictValue("modelB"),
		A: aCite, B: bCite, Reason: "model evidence conflicts within run-a/KC",
	}
	baseRole := AttemptConflict{
		Code: EvidenceConflictCode, ID: id, Field: "role",
		AValue: roleConflictValue("bodies"), BValue: roleConflictValue("seals"),
		A: yamlCite(1, "live", start), B: yamlCite(2, "live", start),
		Reason: "role evidence conflicts within run-a/KC",
	}

	vary := func(base AttemptConflict, fn func(*AttemptConflict)) AttemptConflict {
		out := base
		fn(&out)
		return out
	}

	cases := []struct {
		name  string
		facts []AttemptConflict
	}{
		{
			name: "b-value-only",
			facts: []AttemptConflict{
				baseModel,
				vary(baseModel, func(c *AttemptConflict) { c.BValue = modelConflictValue("modelC") }),
			},
		},
		{
			name: "a-reading-row",
			facts: []AttemptConflict{
				baseRole,
				vary(baseRole, func(c *AttemptConflict) { c.A = yamlCite(4, "live", start) }),
			},
		},
		{
			name: "a-reading-source",
			facts: []AttemptConflict{
				baseRole,
				vary(baseRole, func(c *AttemptConflict) { c.A = yamlCite(1, "history", start) }),
			},
		},
		{
			name: "a-reading-time",
			facts: []AttemptConflict{
				baseRole,
				vary(baseRole, func(c *AttemptConflict) { c.A = yamlCite(1, "live", start.Add(time.Second)) }),
			},
		},
		{
			name: "b-event-time",
			facts: []AttemptConflict{
				baseModel,
				vary(baseModel, func(c *AttemptConflict) { c.B = modelCite(5, 6, start.Add(3*time.Minute)) }),
			},
		},
		{
			name: "b-source",
			facts: []AttemptConflict{
				baseModel,
				vary(baseModel, func(c *AttemptConflict) {
					c.B = FieldEvidence{Source: EvidenceYAML, Event: bCite.Event, Reading: yamlRef(2, "live", start)}
				}),
			},
		},
	}

	for _, path := range []struct {
		name     string
		readings []Reading
	}{
		{name: "empty-readings", readings: nil},
		{name: "matching-reading", readings: matching},
	} {
		t.Run(path.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					requireConflictJoinPermutations(t, tc.facts, path.readings)
				})
			}
		})
	}
}

func testF1ConflictReconcileAppend(t *testing.T) {
	// Bounded corpus: an initial same-ID multi-conflict block is sorted first,
	// then ordinary attempts with conflicting YAML roles append more conflicts
	// during reconciliation. The documented primary order is AttemptID, so the
	// least ordinary key must precede the initial Z block after the final sort.
	start := mustTime(t, "2026-01-01T00:00:00Z")
	const ordinaryN = 10
	attempts := make([]Attempt, ordinaryN)
	var readings []Reading
	for i := 0; i < ordinaryN; i++ {
		key := fmt.Sprintf("A%02d", i)
		attempts[i] = journalAttempt("run-a", key, start, 10*time.Minute, "stamp", OutcomeDone)
		bodies := syntheticReading("run-a", key, start, 1+2*i, "features/study/tasks.yaml")
		bodies.Snapshot.Role = RoleBodies
		seals := syntheticReading("run-a", key, start, 2+2*i, "features/study/tasks.yaml")
		seals.Snapshot.Role = RoleSeals
		readings = append(readings, bodies, seals)
	}

	journal := JournalIdentity{RunID: "run-a", SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	idZ := NewAttemptID("run-a", "Z", start)
	modelEvent := func(seq, line int) EventRef {
		return EventRef{
			Journal: journal, Type: EventTaskSpawnFinished,
			At: start.Add(time.Minute), HasSeq: true, Seq: seq, Line: line,
		}
	}
	aCite := FieldEvidence{Source: EvidenceJournal, Event: modelEvent(1, 2)}
	bCite := FieldEvidence{Source: EvidenceJournal, Event: modelEvent(5, 6)}
	aVal := modelConflictValue("modelA")
	zz := []AttemptConflict{
		{
			Code: EvidenceConflictCode, ID: idZ, Field: "model",
			AValue: aVal, BValue: modelConflictValue("modelB"),
			A: aCite, B: bCite, Reason: "model evidence conflicts within run-a/Z",
		},
		{
			Code: EvidenceConflictCode, ID: idZ, Field: "model",
			AValue: aVal, BValue: modelConflictValue("modelC"),
			A: aCite, B: bCite, Reason: "model evidence conflicts within run-a/Z",
		},
		{
			Code: EvidenceConflictCode, ID: idZ, Field: "model",
			AValue: aVal, BValue: modelConflictValue("modelD"),
			A: aCite, B: bCite, Reason: "model evidence conflicts within run-a/Z",
		},
	}

	type order struct {
		name     string
		zz       []AttemptConflict
		attempts []Attempt
		readings []Reading
	}
	reversedAttempts := append([]Attempt{}, attempts...)
	for i, j := 0, len(reversedAttempts)-1; i < j; i, j = i+1, j-1 {
		reversedAttempts[i], reversedAttempts[j] = reversedAttempts[j], reversedAttempts[i]
	}
	reversedReadings := append([]Reading{}, readings...)
	for i, j := 0, len(reversedReadings)-1; i < j; i, j = i+1, j-1 {
		reversedReadings[i], reversedReadings[j] = reversedReadings[j], reversedReadings[i]
	}
	orders := []order{
		{name: "initial-forward-readings-forward", zz: zz, attempts: attempts, readings: readings},
		{name: "initial-reversed-readings-reversed", zz: []AttemptConflict{zz[2], zz[1], zz[0]}, attempts: reversedAttempts, readings: reversedReadings},
	}

	wantConflicts := ordinaryN + len(zz)
	var encoded [][]byte
	for _, tc := range orders {
		t.Run(tc.name, func(t *testing.T) {
			set := attemptSetOf("run-a", tc.attempts...)
			set.Conflicts = append([]AttemptConflict{}, tc.zz...)
			sets := []AttemptSet{set}
			beforeSets := encodeJSON(t, sets)
			beforeReadings := encodeJSON(t, tc.readings)
			beforeZZ := encodeJSON(t, tc.zz)
			got, err := joinContract(t, sets, tc.readings, defaultSelection(), identityUniverse(set))
			if !errors.Is(err, ErrEvidenceConflict) {
				t.Fatalf("JoinEvidence err = %v, want ErrEvidenceConflict", err)
			}
			if !bytes.Equal(beforeSets, encodeJSON(t, sets)) {
				t.Fatal("JoinEvidence mutated caller attempt sets")
			}
			if !bytes.Equal(beforeReadings, encodeJSON(t, tc.readings)) {
				t.Fatal("JoinEvidence mutated caller readings")
			}
			if !bytes.Equal(beforeZZ, encodeJSON(t, tc.zz)) {
				t.Fatal("JoinEvidence mutated caller initial conflict facts")
			}
			if got.Attempts != ordinaryN+1 || got.UniqueRows != ordinaryN+1 {
				t.Fatalf("denominator attempts=%d unique=%d, want %d/%d", got.Attempts, got.UniqueRows, ordinaryN+1, ordinaryN+1)
			}
			if got.Recovered != 0 || len(got.Observations) != 0 {
				t.Fatalf("conflict corpus recovered a sample: recovered=%d", got.Recovered)
			}
			if len(got.Conflicts) != wantConflicts {
				t.Fatalf("retained %d conflicts, want %d", len(got.Conflicts), wantConflicts)
			}
			if got.Conflicts[0].ID.Key != "A00" {
				t.Errorf("final conflict order did not keep AttemptID primary: first key=%q, want A00", got.Conflicts[0].ID.Key)
			}
			zCount := 0
			roleCount := 0
			var zB []string
			for _, conflict := range got.Conflicts {
				if conflict.Code != EvidenceConflictCode {
					t.Fatalf("illegal producer Code %q", conflict.Code)
				}
				switch conflict.ID.Key {
				case "Z":
					zCount++
					zB = append(zB, string(conflict.BValue.Value))
					if conflict.Field != "model" {
						t.Fatalf("Z fact field = %q", conflict.Field)
					}
				default:
					roleCount++
					if conflict.Field != "role" {
						t.Fatalf("reconciled fact field = %q", conflict.Field)
					}
				}
			}
			if zCount != len(zz) || roleCount != ordinaryN {
				t.Fatalf("retained z=%d role=%d, want z=%d role=%d", zCount, roleCount, len(zz), ordinaryN)
			}
			wantZB := []string{`"modelB"`, `"modelC"`, `"modelD"`}
			if len(zB) != len(wantZB) {
				t.Errorf("Z-block BValue sequence %v, want %v", zB, wantZB)
			} else {
				for i := range wantZB {
					if zB[i] != wantZB[i] {
						t.Errorf("Z-block BValue sequence %v, want %v", zB, wantZB)
						break
					}
				}
			}
			encoded = append(encoded, encodeJSON(t, got))
		})
	}
	if len(encoded) != 2 || !bytes.Equal(encoded[0], encoded[1]) {
		t.Fatal("EvidenceJoin JSON depended on initial-conflict or reading order")
	}
}

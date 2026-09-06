package dispatched

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func gitRevision(pair string) string {
	return "git:" + strings.Repeat(pair, 20)
}

func roleReading(run, key string, start time.Time, row int, path, revision string, role Role) Reading {
	reading := syntheticReading(run, key, start, row, path)
	reading.Ref.Revision = revision
	reading.Snapshot.Role = role
	return reading
}

func requireNoConflict(t *testing.T, err error, got EvidenceJoin) {
	t.Helper()
	if errors.Is(err, ErrEvidenceConflict) {
		t.Fatalf("false role conflict: %v dispositions=%+v conflicts=%+v", err, got.Dispositions, got.Conflicts)
	}
	if err != nil {
		t.Fatalf("JoinEvidence: %v", err)
	}
	requireDisposition(t, got, DispositionConflictingEvidence, 0)
	if len(got.Conflicts) != 0 {
		t.Fatalf("portable conflicts = %+v", got.Conflicts)
	}
}

func recoveredRoleCitation(t *testing.T, got EvidenceJoin) ReadingRef {
	t.Helper()
	if got.Recovered != 1 || len(got.Observations) != 1 {
		t.Fatalf("recovered=%d observations=%d", got.Recovered, len(got.Observations))
	}
	obs := got.Observations[0]
	if obs.Cell.Role != RoleBodies {
		t.Fatalf("recovered role = %q, want bodies from the valid reading", obs.Cell.Role)
	}
	if obs.Attempt.Evidence.Role.Source != EvidenceYAML {
		t.Fatalf("role citation source = %q", obs.Attempt.Evidence.Role.Source)
	}
	return obs.Attempt.Evidence.Role.Reading
}

func requireRefuseInvalid(t *testing.T, art Artifact) {
	t.Helper()
	got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if got.Eligible {
		t.Fatal("invalid payload marked eligible")
	}
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("invalid payload refuse = %v, want ErrNotEligible and ErrSourceIncomplete", err)
	}
}

func requireEligibleCompleted(t *testing.T, art Artifact, completed int) {
	t.Helper()
	got, err := PredictionEligibility(art, eligibleTargets(), completed, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || len(got.Cells) != 1 || got.Cells[0].Completed != completed {
		t.Fatalf("eligibility = %+v", got)
	}
}

func mutateObservation(art Artifact, fn func(*RecoveredAttempt)) Artifact {
	obs := art.Evidence.Observations[0]
	fn(&obs)
	art.Evidence.Observations[0] = obs
	return art
}

func intervalAt(start time.Time, from, to time.Duration, phase Phase) Interval {
	return Interval{Phase: phase, Start: start.Add(from), End: start.Add(to), Evidence: []EventRef{}}
}

func testF1RoleAbsence(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-role", "F1-ROLE-ABSENT", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-role", att)
	universe := identityUniverse(set)

	t.Run("missing-and-valid", func(t *testing.T) {
		for _, citation := range []struct {
			name, validRev, absentRev string
		}{
			{name: "citation-valid-first", validRev: gitRevision("aa"), absentRev: gitRevision("bb")},
			{name: "citation-absent-first", validRev: gitRevision("bb"), absentRev: gitRevision("aa")},
		} {
			t.Run(citation.name, func(t *testing.T) {
				var encoded [][]byte
				for _, inputAbsentFirst := range []bool{false, true} {
					name := "input-valid-first"
					if inputAbsentFirst {
						name = "input-absent-first"
					}
					t.Run(name, func(t *testing.T) {
						valid := roleReading("run-role", "F1-ROLE-ABSENT", start, 1, "features/study/tasks.yaml", citation.validRev, RoleBodies)
						absent := roleReading("run-role", "F1-ROLE-ABSENT", start, 2, "features/study/tasks.yaml", citation.absentRev, "")
						absent.Snapshot.AuthoredModel = "bodies"
						readings := []Reading{valid, absent}
						if inputAbsentFirst {
							readings = []Reading{absent, valid}
						}
						got, err := joinContract(t, []AttemptSet{set}, readings, defaultSelection(), universe)
						requireNoConflict(t, err, got)
						cited := recoveredRoleCitation(t, got)
						if cited != valid.Ref {
							t.Fatalf("role cited %+v, want valid reading %+v", cited, valid.Ref)
						}
						requireDisposition(t, got, DispositionRecovered, 1)
						requireDisposition(t, got, DispositionDuplicateReading, 1)
						encoded = append(encoded, encodeJSON(t, got))
					})
				}
				if len(encoded) == 2 && !bytes.Equal(encoded[0], encoded[1]) {
					t.Fatal("missing/valid role join depended on input order")
				}
			})
		}
	})

	t.Run("invalid-with-valid-sibling", func(t *testing.T) {
		valid := roleReading("run-role", "F1-ROLE-ABSENT", start, 1, "features/study/tasks.yaml", "live", RoleBodies)
		invalid := valid
		invalid.Ref.Row = 2
		invalid.Snapshot.Role = Role("not-a-role")
		for _, name := range []string{"valid-first", "invalid-first"} {
			t.Run(name, func(t *testing.T) {
				readings := []Reading{valid, invalid}
				if name == "invalid-first" {
					readings = []Reading{invalid, valid}
				}
				got, err := joinContract(t, []AttemptSet{set}, readings, defaultSelection(), universe)
				requireNoConflict(t, err, got)
				if recoveredRoleCitation(t, got) != valid.Ref {
					t.Fatalf("valid sibling was not the recovered role citation: %+v", got.Observations[0])
				}
				requireDisposition(t, got, DispositionMalformed, 1)
				requireDisposition(t, got, DispositionRecovered, 1)
				requireDisposition(t, got, DispositionUnrecoverable, 0)
			})
		}
	})

	t.Run("invalid-heldout-wins", func(t *testing.T) {
		keep := journalAttempt("keep", "K", start, 10*time.Minute, "stamp", OutcomeDone)
		keepSet := attemptSetOf("keep", keep)
		heldID := JournalIdentity{RunID: "held", SourceID: "journals", Path: "held/journal.jsonl", Producer: ProducerDispatcherV0_1_0}
		keepReading := syntheticReading("keep", "K", start, 1, "features/study/tasks.yaml")
		held := syntheticReading("held", "K", start, 1, "features/study/tasks.yaml")
		held.Snapshot.Role = Role("not-a-role")
		got := mustJoin(t, []AttemptSet{keepSet}, []Reading{held, keepReading}, Selection{
			Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"},
		}, []JournalIdentity{keepSet.Journal, heldID})
		requireDisposition(t, got, DispositionHeldOut, 1)
		requireDisposition(t, got, DispositionMalformed, 0)
		if got.Recovered != 1 {
			t.Fatalf("held-out invalid role disturbed keep recovery: recovered=%d", got.Recovered)
		}
	})

	t.Run("all-absent", func(t *testing.T) {
		a := roleReading("run-role", "F1-ROLE-ABSENT", start, 1, "features/a/tasks.yaml", gitRevision("aa"), "")
		b := roleReading("run-role", "F1-ROLE-ABSENT", start, 2, "features/z/tasks.yaml", gitRevision("bb"), "")
		for _, tc := range []struct {
			name     string
			readings []Reading
		}{
			{name: "one", readings: []Reading{a}},
			{name: "two-a-b", readings: []Reading{a, b}},
			{name: "two-b-a", readings: []Reading{b, a}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := joinContract(t, []AttemptSet{set}, tc.readings, defaultSelection(), universe)
				if errors.Is(err, ErrEvidenceConflict) {
					t.Fatalf("all-absent role became a conflict: %v", err)
				}
				if err != nil {
					t.Fatalf("JoinEvidence: %v", err)
				}
				if got.Recovered != 0 || len(got.Observations) != 0 {
					t.Fatalf("all-absent role recovered a sample: %+v", got)
				}
				requireDisposition(t, got, DispositionUnrecoverable, len(tc.readings))
				requireDisposition(t, got, DispositionConflictingEvidence, 0)
				if len(got.LostAttempts) != 1 || got.LostAttempts[0] != att.ID {
					t.Fatalf("lost attempts = %+v", got.LostAttempts)
				}
				for _, examined := range got.Examined {
					if examined.Disposition != DispositionUnrecoverable {
						t.Fatalf("disposition = %s", examined.Disposition)
					}
					if !strings.Contains(strings.ToLower(examined.Reason), "role") {
						t.Fatalf("unrecoverable reason %q does not name missing role", examined.Reason)
					}
				}
			})
		}
	})
}

func testF1EnvelopeAssociation(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-env", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-env", att)
	valid := syntheticReading("run-env", "K", start, 1, "features/study/tasks.yaml")
	valid.Snapshot.Role = RoleBodies
	valid.Snapshot.Status = "Done"
	malformed := valid
	malformed.Kind = DocumentMalformed
	malformed.Err = errors.New("document malformed")
	malformed.Snapshot.Role = RoleSeals
	malformed.Snapshot.Status = "Blocked"
	if malformed.Ref != valid.Ref || malformed.Identity != valid.Identity || malformed.CompletedAt != valid.CompletedAt {
		t.Fatal("fixture envelopes drifted")
	}
	var encoded [][]byte
	for _, name := range []string{"malformed-first", "valid-first"} {
		t.Run(name, func(t *testing.T) {
			readings := []Reading{malformed, valid}
			if name == "valid-first" {
				readings = []Reading{valid, malformed}
			}
			got, err := joinContract(t, []AttemptSet{set}, readings, defaultSelection(), identityUniverse(set))
			if err != nil {
				t.Fatalf("JoinEvidence: %v", err)
			}
			cited := recoveredRoleCitation(t, got)
			if cited != valid.Ref || got.Observations[0].Cell.Role != RoleBodies {
				t.Fatalf("malformed envelope was reconciled as recovered: cell=%+v cited=%+v", got.Observations[0].Cell, cited)
			}
			requireDisposition(t, got, DispositionMalformed, 1)
			requireDisposition(t, got, DispositionRecovered, 1)
			if len(got.Examined) != 2 {
				t.Fatalf("examined = %d, want both envelopes", len(got.Examined))
			}
			encoded = append(encoded, encodeJSON(t, got))
		})
	}
	if len(encoded) != 2 || !bytes.Equal(encoded[0], encoded[1]) {
		t.Fatal("valid/malformed envelope join depended on input order")
	}
}

func testF3BuildAllowEmptyState(t *testing.T) {
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())},
		Selection{Cutoff: contractCutoff(), AllowEmpty: true},
		ReadBounds{},
	))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result == nil || result.Artifact.SourceManifest == nil {
		t.Fatal("AllowEmpty Build dropped the diagnostic artifact")
	}
	if result.Artifact.SourceManifest.State != SourceEmpty {
		t.Fatalf("AllowEmpty state = %q, want EMPTY", result.Artifact.SourceManifest.State)
	}
	got, eligErr := PredictionEligibility(result.Artifact, eligibleTargets(), 1, true)
	if got.Eligible {
		t.Fatal("EMPTY corpus marked eligible")
	}
	if !errors.Is(eligErr, ErrNotEligible) || !errors.Is(eligErr, ErrSourceIncomplete) {
		t.Fatalf("EMPTY eligibility = %v, want ErrNotEligible and ErrSourceIncomplete", eligErr)
	}
}

func testF3BuildEarlySourceReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   BuildOptions
		wantIs error
	}{
		{
			name: "negative-bounds",
			opts: amendedBuildOpts(
				[]SourceSpec{journalSpec("j", t.TempDir())},
				defaultSelection(),
				ReadBounds{MaxCommits: -1},
			),
			wantIs: ErrInvalidSourceSpec,
		},
		{
			name: "blank-source-id",
			opts: amendedBuildOpts(
				[]SourceSpec{{ID: "", Kind: SourceKindJournals, Repository: t.TempDir()}},
				defaultSelection(),
				ReadBounds{},
			),
			wantIs: ErrInvalidSourceSpec,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Build(context.Background(), tc.opts)
			if errors.Is(err, ErrNotImplemented) {
				t.Fatal(err)
			}
			requireSentinel(t, err, tc.wantIs)
			if result == nil || result.Artifact.SourceManifest == nil {
				t.Fatal("early source failure dropped the diagnostic artifact")
			}
			manifest := result.Artifact.SourceManifest
			if manifest.State != SourcePartial {
				t.Fatalf("state = %q, want PARTIAL", manifest.State)
			}
			if len(manifest.Reasons) == 0 || !sort.StringsAreSorted(manifest.Reasons) {
				t.Fatalf("early source failure reasons = %v", manifest.Reasons)
			}
			seen := map[string]bool{}
			for _, reason := range manifest.Reasons {
				if strings.TrimSpace(reason) == "" || seen[reason] {
					t.Fatalf("reason is blank or duplicated: %v", manifest.Reasons)
				}
				seen[reason] = true
			}
		})
	}
}

func testF4EligibleStructure(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		positive, err := PredictionEligibility(recoveredArtifact(2), eligibleTargets(), 2, true)
		if err != nil {
			t.Fatal(err)
		}
		if !positive.Eligible || positive.MinCompleted != 2 {
			t.Fatalf("positive recoveredArtifact(2) = %+v", positive)
		}
		if recoveredArtifact(2).Evidence.Observations[0].Attempt.CostUSD.Known {
			t.Fatal("positive fixture invented an optional cost measurement")
		}
	})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		mutate func(Artifact) Artifact
	}{
		{name: "remove-reading-refs", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) { obs.Readings = []ReadingRef{} })
		}},
		{name: "remove-role-citation", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) { obs.Attempt.Evidence.Role = FieldEvidence{} })
		}},
		{name: "remove-model-citation", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) { obs.Attempt.Evidence.Model = FieldEvidence{} })
		}},
		{name: "remove-terminal-citation", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) { obs.Attempt.Evidence.Terminal = FieldEvidence{} })
		}},
		{name: "remove-elapsed-citation", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) { obs.Attempt.Evidence.Elapsed = FieldEvidence{} })
		}},
		{name: "yaml-terminal-elapsed-mismatch", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) {
				a := obs.Readings[0]
				b := a
				b.Row = 99
				obs.Attempt.Evidence.Terminal = FieldEvidence{Source: EvidenceYAML, Reading: a}
				obs.Attempt.Evidence.Elapsed = FieldEvidence{Source: EvidenceYAML, Reading: b}
			})
		}},
		{name: "inject-conflict", mutate: func(art Artifact) Artifact {
			id := art.Evidence.Observations[0].Attempt.ID
			art.Evidence.Conflicts = []AttemptConflict{{
				Code: EvidenceConflictCode, ID: id, Field: "role",
				A: FieldEvidence{Source: EvidenceYAML, Reading: art.Evidence.Observations[0].Readings[0]},
				B: FieldEvidence{Source: EvidenceYAML},
			}}
			return art
		}},
		{name: "inject-ambiguity", mutate: func(art Artifact) Artifact {
			att := art.Evidence.Observations[0].Attempt
			art.Evidence.Ambiguous = []AmbiguousAttempt{{
				ID: att.ID, Starts: 2, Refs: []EventRef{att.Start},
			}}
			return art
		}},
		{name: "corrupt-recovered-count", mutate: func(art Artifact) Artifact {
			art.Evidence.Recovered = 0
			return art
		}},
		{name: "corrupt-attempts-count", mutate: func(art Artifact) Artifact {
			art.Evidence.Attempts = 99
			return art
		}},
		{name: "corrupt-unique-rows", mutate: func(art Artifact) Artifact {
			art.Evidence.UniqueRows = 0
			return art
		}},
		{name: "corrupt-disposition-count", mutate: func(art Artifact) Artifact {
			for i, row := range art.Evidence.Dispositions {
				if row.Disposition == DispositionRecovered {
					art.Evidence.Dispositions[i].Count = 0
				}
			}
			return art
		}},
		{name: "corrupt-yaml-only-terminal", mutate: func(art Artifact) Artifact {
			art.Evidence.RowsWithYAMLOnlyTerminal = 7
			return art
		}},
		{name: "inject-malformed-disposition", mutate: func(art Artifact) Artifact {
			art.Evidence.Examined = append(append([]Examined{}, art.Evidence.Examined...), Examined{
				Reading:     art.Evidence.Observations[0].Readings[0],
				Disposition: DispositionMalformed,
				Reason:      "injected",
			})
			return art
		}},
		{name: "invalid-wall-phase", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) {
				obs.Attempt.Wall.Intervals = []Interval{intervalAt(start, 0, time.Minute, PhaseUnclassified)}
			})
		}},
		{name: "reversed-wall-interval", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) {
				obs.Attempt.Wall.Intervals = []Interval{intervalAt(start, 5*time.Minute, 2*time.Minute, PhaseDevelopment)}
			})
		}},
		{name: "overlapping-wall-intervals", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) {
				obs.Attempt.Wall.Intervals = []Interval{
					intervalAt(start, 0, 6*time.Minute, PhaseDevelopment),
					intervalAt(start, 4*time.Minute, 8*time.Minute, PhaseDevelopment),
				}
			})
		}},
		{name: "noncanonical-wall-intervals", mutate: func(art Artifact) Artifact {
			return mutateObservation(art, func(obs *RecoveredAttempt) {
				obs.Attempt.Wall.Intervals = []Interval{
					intervalAt(start, 5*time.Minute, 8*time.Minute, PhaseDevelopment),
					intervalAt(start, 0, 2*time.Minute, PhaseDevelopment),
				}
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefuseInvalid(t, tc.mutate(recoveredArtifact(2)))
		})
	}
}

func testF4EligibleLegitimateLoss(t *testing.T) {
	art := recoveredArtifact(2)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lost := journalAttempt("run-a", "KZ", start, 10*time.Minute, "stamp", OutcomeDone)
	art.Evidence.LostAttempts = []AttemptID{lost.ID}
	art.Evidence.Attempts = 3
	art.Evidence.UniqueRows = 3
	got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible {
		t.Fatalf("complete artifact with a no-YAML lost attempt was refused: %+v", got)
	}
}

func testF1AttemptUniverse(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	keep := attemptSetOf("run-a", att)
	other := JournalIdentity{RunID: "run-b", SourceID: "journals", Path: "run-b/journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	reading := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")

	t.Run("missing-selected-set", func(t *testing.T) {
		got, err := joinContract(t, []AttemptSet{keep}, []Reading{reading}, defaultSelection(), []JournalIdentity{keep.Journal, other})
		if err != nil {
			t.Fatalf("missing selected set is valid diagnostic input: %v", err)
		}
		if got.Recovered != 1 {
			t.Fatalf("keep recovery lost: %+v", got)
		}
	})

	t.Run("repeated-set", func(t *testing.T) {
		empty := attemptSetOf("run-a")
		disjointA := attemptSetOf("run-a", journalAttempt("run-a", "A", start, 10*time.Minute, "stamp", OutcomeDone))
		disjointB := attemptSetOf("run-a", journalAttempt("run-a", "B", start, 10*time.Minute, "stamp", OutcomeDone))
		for _, tc := range []struct {
			name string
			sets []AttemptSet
		}{
			{name: "empty", sets: []AttemptSet{empty, empty}},
			{name: "disjoint", sets: []AttemptSet{disjointA, disjointB}},
			{name: "identical", sets: []AttemptSet{keep, keep}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := joinContract(t, tc.sets, nil, defaultSelection(), identityUniverse(keep))
				requireSentinel(t, err, ErrInvalidSelection)
			})
		}
	})

	t.Run("duplicate-ambiguous-ids", func(t *testing.T) {
		id := att.ID
		refs := []EventRef{att.Start, att.Start}
		refs[1].Seq, refs[1].Line = 9, 9
		amb := AmbiguousAttempt{ID: id, Starts: 2, Refs: refs}
		set := attemptSetOf("run-a")
		set.Ambiguous = []AmbiguousAttempt{amb, amb}
		_, err := joinContract(t, []AttemptSet{set}, nil, defaultSelection(), identityUniverse(set))
		requireSentinel(t, err, ErrEvidenceConflict)
	})

	t.Run("regular-vs-ambiguous", func(t *testing.T) {
		amb := AmbiguousAttempt{ID: att.ID, Starts: 2, Refs: []EventRef{att.Start}}
		t.Run("same-set", func(t *testing.T) {
			same := attemptSetOf("run-a", att)
			same.Ambiguous = []AmbiguousAttempt{amb}
			_, err := joinContract(t, []AttemptSet{same}, nil, defaultSelection(), identityUniverse(same))
			requireSentinel(t, err, ErrEvidenceConflict)
		})
		regular := attemptSetOf("run-a", att)
		onlyAmb := attemptSetOf("run-a")
		onlyAmb.Ambiguous = []AmbiguousAttempt{amb}
		for _, name := range []string{"regular-then-ambiguous", "ambiguous-then-regular"} {
			t.Run(name, func(t *testing.T) {
				sets := []AttemptSet{regular, onlyAmb}
				if name == "ambiguous-then-regular" {
					sets = []AttemptSet{onlyAmb, regular}
				}
				_, joinErr := joinContract(t, sets, nil, defaultSelection(), identityUniverse(regular))
				requireSentinel(t, joinErr, ErrInvalidSelection)
			})
		}
	})

	t.Run("multiple-conflict-facts-retained", func(t *testing.T) {
		set := attemptSetOf("run-a")
		set.Conflicts = []AttemptConflict{
			{Code: EvidenceConflictCode, ID: att.ID, Field: "terminal", Reason: "terminal", A: att.Evidence.Terminal, B: FieldEvidence{Source: EvidenceYAML}},
			{Code: EvidenceConflictCode, ID: att.ID, Field: "model", Reason: "model", A: att.Evidence.Model, B: FieldEvidence{Source: EvidenceJournal}},
		}
		got, err := joinContract(t, []AttemptSet{set}, nil, defaultSelection(), identityUniverse(set))
		if err != nil && !errors.Is(err, ErrEvidenceConflict) {
			t.Fatalf("distinct conflict facts: %v", err)
		}
		if len(got.Conflicts) != 2 {
			t.Fatalf("distinct conflict facts discarded: %+v", got.Conflicts)
		}
		fields := map[string]bool{}
		for _, conflict := range got.Conflicts {
			fields[conflict.Field] = true
		}
		if !fields["terminal"] || !fields["model"] {
			t.Fatalf("conflict fields = %v", fields)
		}
	})
}

func testF2JoinInputImmutable(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")

	t.Run("yaml-terminal-wall", func(t *testing.T) {
		att := journalAttempt("run-a", "K", start, contractCutoff().Sub(start), "stamp", OutcomeUnfinished)
		span := EventRef{
			Journal: att.Start.Journal, Type: EventTaskSpawnFinished,
			At: start.Add(time.Minute), HasSeq: true, Seq: 2, Line: 3,
		}
		att.Wall.Intervals = []Interval{{
			Phase: PhaseDevelopment, Start: start, End: start.Add(time.Hour),
			Evidence: []EventRef{span},
		}}
		att.Evidence.Terminal = FieldEvidence{}
		set := attemptSetOf("run-a", att)
		yaml := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
		yaml.Snapshot.Status = "Done"
		yaml.CompletedAt = Known(start.Add(10 * time.Minute))
		sets := []AttemptSet{set}
		readings := []Reading{yaml}
		beforeSets, beforeReadings := encodeJSON(t, sets), encodeJSON(t, readings)
		beforeNested := encodeJSON(t, sets[0].Attempts[0].Wall.Intervals[0].Evidence)
		got1 := mustJoin(t, sets, readings, defaultSelection(), identityUniverse(set))
		if !bytes.Equal(beforeSets, encodeJSON(t, sets)) || !bytes.Equal(beforeReadings, encodeJSON(t, readings)) {
			t.Fatal("JoinEvidence mutated YAML-terminal inputs")
		}
		if !bytes.Equal(beforeNested, encodeJSON(t, sets[0].Attempts[0].Wall.Intervals[0].Evidence)) {
			t.Fatal("JoinEvidence mutated nested wall citations")
		}
		got2 := mustJoin(t, sets, readings, defaultSelection(), identityUniverse(set))
		if !bytes.Equal(encodeJSON(t, got1), encodeJSON(t, got2)) {
			t.Fatal("repeated JoinEvidence outputs differed")
		}
		if got1.Observations[0].Attempt.Elapsed != 10*time.Minute {
			t.Fatalf("elapsed = %s", got1.Observations[0].Attempt.Elapsed)
		}
	})

	t.Run("ambiguous-refs", func(t *testing.T) {
		id := NewAttemptID("run-a", "K", start)
		journal := JournalIdentity{RunID: "run-a", SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0}
		low := EventRef{Journal: journal, Type: EventTaskStarted, At: start, HasSeq: true, Seq: 1, Line: 2, Hash: "aa"}
		high := EventRef{Journal: journal, Type: EventTaskStarted, At: start, HasSeq: true, Seq: 9, Line: 9, Hash: "zz"}
		set := attemptSetOf("run-a")
		set.Ambiguous = []AmbiguousAttempt{{ID: id, Starts: 2, Refs: []EventRef{high, low}}}
		before := encodeJSON(t, set.Ambiguous[0].Refs)
		beforeSets := encodeJSON(t, []AttemptSet{set})
		_, err := joinContract(t, []AttemptSet{set}, nil, defaultSelection(), identityUniverse(set))
		if err != nil && !errors.Is(err, ErrAmbiguousAttempt) {
			t.Fatalf("ambiguous control: %v", err)
		}
		if !bytes.Equal(before, encodeJSON(t, set.Ambiguous[0].Refs)) || !bytes.Equal(beforeSets, encodeJSON(t, []AttemptSet{set})) {
			t.Fatal("JoinEvidence sorted or rewrote caller-owned ambiguous refs")
		}
	})
}

func testF4ArtifactCutoffProof(t *testing.T) {
	cutoff := contractCutoff()
	future := cutoff.Add(time.Minute)

	for _, tc := range []struct {
		name     string
		artifact func() Artifact
		eligible bool
	}{
		{name: "reading-recorded-at-after-cutoff", artifact: func() Artifact {
			return withRecoveredRecordedAt(recoveredArtifact(2), 0, future)
		}},
		{name: "reading-recorded-at-exact-cutoff", artifact: func() Artifact {
			return withRecoveredRecordedAt(recoveredArtifact(2), 0, cutoff)
		}, eligible: true},
		{name: "yaml-terminal-after-cutoff", artifact: func() Artifact {
			return withYAMLOnlyTerminal(recoveredArtifact(2), 0, future)
		}},
		{name: "yaml-terminal-exact-cutoff", artifact: func() Artifact {
			return withYAMLOnlyTerminal(recoveredArtifact(2), 0, cutoff)
		}, eligible: true},
		{name: "recovered-audit-completed-at-after-cutoff", artifact: func() Artifact {
			return withExaminedCompletedAt(recoveredArtifact(2), 0, future)
		}},
		{name: "duplicate-audit-completed-at-after-cutoff", artifact: func() Artifact {
			return withDuplicateReadingCompletedAt(recoveredArtifact(2), future)
		}},
		{name: "unfinished-at-cutoff-two-completed", artifact: func() Artifact {
			return unfinishedObservationAtCutoff(recoveredArtifact(3), 2)
		}, eligible: true},
		{name: "unfinished-elapsed-shorter-than-cutoff", artifact: func() Artifact {
			return withUnfinishedElapsedDelta(recoveredArtifact(3), 2, -time.Minute)
		}},
		{name: "unfinished-elapsed-longer-than-cutoff", artifact: func() Artifact {
			return withUnfinishedElapsedDelta(recoveredArtifact(3), 2, time.Minute)
		}},
		{name: "after-cutoff-diagnostic-future-time", artifact: func() Artifact {
			return withAfterCutoffDiagnostic(recoveredArtifact(2), future)
		}, eligible: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			art := tc.artifact()
			if tc.eligible {
				got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
				if err != nil {
					t.Fatal(err)
				}
				if !got.Eligible || len(got.Cells) != 1 || got.Cells[0].Completed != 2 {
					t.Fatalf("eligibility = %+v", got)
				}
				return
			}
			requireRefuseInvalid(t, art)
		})
	}
}

func withRecoveredRecordedAt(art Artifact, i int, recordedAt time.Time) Artifact {
	obs := art.Evidence.Observations[i]
	ref := obs.Readings[0]
	ref.RecordedAt = recordedAt
	obs.Readings[0] = ref
	obs.Attempt.Evidence.Role.Reading = ref
	art.Evidence.Observations[i] = obs
	art.Evidence.Examined[i].Reading = ref
	return art
}

func withYAMLOnlyTerminal(art Artifact, i int, terminal time.Time) Artifact {
	obs := art.Evidence.Observations[i]
	citation := FieldEvidence{Source: EvidenceYAML, Reading: obs.Readings[0]}
	obs.Attempt.TerminalAt = terminal
	obs.Attempt.Elapsed = terminal.Sub(obs.Attempt.ID.StartedAt)
	obs.Attempt.Wall.Elapsed = obs.Attempt.Elapsed
	obs.Attempt.Evidence.Terminal = citation
	obs.Attempt.Evidence.Elapsed = citation
	art.Evidence.Observations[i] = obs
	art.Evidence.RowsWithYAMLOnlyTerminal = 1
	art.Evidence.Examined[i].CompletedAt = Known(terminal)
	return art
}

func withExaminedCompletedAt(art Artifact, i int, completedAt time.Time) Artifact {
	art.Evidence.Examined[i].CompletedAt = Known(completedAt)
	return art
}

func withDuplicateReadingCompletedAt(art Artifact, completedAt time.Time) Artifact {
	obs := art.Evidence.Observations[0]
	dup := obs.Readings[0]
	dup.Row = 99
	obs.Readings = append(append([]ReadingRef{}, obs.Readings...), dup)
	art.Evidence.Observations[0] = obs
	extra := art.Evidence.Examined[0]
	extra.Reading = dup
	extra.CompletedAt = Known(completedAt)
	extra.Disposition = DispositionDuplicateReading
	extra.Reason = "additional compatible reading of recovered attempt"
	art.Evidence.Examined = append(append([]Examined{}, art.Evidence.Examined...), extra)
	setDispositionCount(art, DispositionDuplicateReading, 1)
	return art
}

func unfinishedObservationAtCutoff(art Artifact, i int) Artifact {
	cutoff := art.SourceManifest.Cutoff
	obs := art.Evidence.Observations[i]
	obs.Attempt.Outcome = OutcomeUnfinished
	obs.Attempt.Evidence.Terminal = FieldEvidence{}
	obs.Attempt.Evidence.Elapsed = FieldEvidence{}
	obs.Attempt.Elapsed = cutoff.Sub(obs.Attempt.ID.StartedAt)
	obs.Attempt.Wall.Elapsed = obs.Attempt.Elapsed
	obs.Attempt.TerminalAt = time.Time{}
	art.Evidence.Observations[i] = obs
	art.Evidence.Examined[i].CompletedAt = Unknown[time.Time]()
	return art
}

func withUnfinishedElapsedDelta(art Artifact, i int, delta time.Duration) Artifact {
	art = unfinishedObservationAtCutoff(art, i)
	obs := art.Evidence.Observations[i]
	obs.Attempt.Elapsed += delta
	obs.Attempt.Wall.Elapsed = obs.Attempt.Elapsed
	art.Evidence.Observations[i] = obs
	return art
}

func withAfterCutoffDiagnostic(art Artifact, at time.Time) Artifact {
	ref := art.Evidence.Observations[0].Readings[0]
	ref.Row = 100
	ref.RecordedAt = at
	art.Evidence.Examined = append(append([]Examined{}, art.Evidence.Examined...), Examined{
		Identity: ReadingIdentity{
			RunID:     Known("run-z"),
			Key:       Known("KZ"),
			StartedAt: Known(at),
		},
		CompletedAt: Known(at),
		Reading:     ref,
		Disposition: DispositionAfterCutoff,
		Reason:      "reading contains evidence after the extraction cutoff",
	})
	setDispositionCount(art, DispositionAfterCutoff, 1)
	return art
}

func setDispositionCount(art Artifact, d Disposition, n int) {
	for i, row := range art.Evidence.Dispositions {
		if row.Disposition == d {
			art.Evidence.Dispositions[i].Count = n
			return
		}
	}
}

func recoveredStart() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func mapRecoveredYAML(art Artifact, i int, fn func(ReadingRef) ReadingRef) Artifact {
	obs := art.Evidence.Observations[i]
	id := obs.Attempt.ID
	refs := make([]ReadingRef, len(obs.Readings))
	for j, ref := range obs.Readings {
		refs[j] = fn(ref)
	}
	obs.Readings = refs
	obs.Attempt.Evidence.Role.Reading = fn(obs.Attempt.Evidence.Role.Reading)
	if obs.Attempt.Evidence.Terminal.Source == EvidenceYAML {
		obs.Attempt.Evidence.Terminal.Reading = fn(obs.Attempt.Evidence.Terminal.Reading)
	}
	if obs.Attempt.Evidence.Elapsed.Source == EvidenceYAML {
		obs.Attempt.Evidence.Elapsed.Reading = fn(obs.Attempt.Evidence.Elapsed.Reading)
	}
	art.Evidence.Observations[i] = obs
	examined := append([]Examined{}, art.Evidence.Examined...)
	for j, row := range examined {
		if row.Attempt != id {
			continue
		}
		if row.Disposition != DispositionRecovered && row.Disposition != DispositionDuplicateReading {
			continue
		}
		examined[j].Reading = fn(row.Reading)
	}
	art.Evidence.Examined = examined
	return art
}

func mapAllRecoveredYAML(art Artifact, fn func(ReadingRef) ReadingRef) Artifact {
	for i := range art.Evidence.Observations {
		art = mapRecoveredYAML(art, i, fn)
	}
	return art
}

func mapRecoveredJournals(art Artifact, i int, fn func(JournalIdentity) JournalIdentity) Artifact {
	obs := art.Evidence.Observations[i]
	obs.Attempt = mapAttemptJournals(obs.Attempt, fn)
	art.Evidence.Observations[i] = obs
	return art
}

func mapAllRecoveredJournals(art Artifact, fn func(JournalIdentity) JournalIdentity) Artifact {
	for i := range art.Evidence.Observations {
		art = mapRecoveredJournals(art, i, fn)
	}
	return art
}

// withLinkedRunIdentity relocates every recovered run ID and journal path on a
// hand-built artifact together. Models, outcomes, counts, n and threshold are
// left unchanged. Empty identities stay empty.
func withLinkedRunIdentity(art Artifact, runID, journalPath string) Artifact {
	remapJournal := func(j JournalIdentity) JournalIdentity {
		j.RunID = runID
		j.Path = journalPath
		return j
	}
	remapID := func(id AttemptID) AttemptID {
		if id.RunID == "" && id.Key == "" && id.StartedAt.IsZero() {
			return id
		}
		return NewAttemptID(runID, id.Key, id.StartedAt)
	}

	for i := range art.Evidence.Observations {
		obs := art.Evidence.Observations[i]
		obs.Attempt = mapAttemptJournals(obs.Attempt, remapJournal)
		obs.Attempt.ID = remapID(obs.Attempt.ID)
		art.Evidence.Observations[i] = obs
	}

	examined := append([]Examined{}, art.Evidence.Examined...)
	for i, row := range examined {
		if row.Identity.RunID.Known {
			row.Identity.RunID = Known(runID)
		}
		row.Attempt = remapID(row.Attempt)
		examined[i] = row
	}
	art.Evidence.Examined = examined

	if art.Evidence.LostAttempts != nil {
		lost := append([]AttemptID{}, art.Evidence.LostAttempts...)
		for i, id := range lost {
			lost[i] = remapID(id)
		}
		art.Evidence.LostAttempts = lost
	}
	if art.Evidence.ExcludedJournals != nil {
		excluded := append([]JournalIdentity{}, art.Evidence.ExcludedJournals...)
		for i, j := range excluded {
			excluded[i] = remapJournal(j)
		}
		art.Evidence.ExcludedJournals = excluded
	}
	if art.Evidence.Conflicts != nil {
		conflicts := append([]AttemptConflict{}, art.Evidence.Conflicts...)
		for i, c := range conflicts {
			c.ID = remapID(c.ID)
			c.A = mapFieldEvidenceJournal(c.A, remapJournal)
			c.B = mapFieldEvidenceJournal(c.B, remapJournal)
			conflicts[i] = c
		}
		art.Evidence.Conflicts = conflicts
	}
	if art.Evidence.Ambiguous != nil {
		ambiguous := append([]AmbiguousAttempt{}, art.Evidence.Ambiguous...)
		for i, a := range ambiguous {
			a.ID = remapID(a.ID)
			a.Refs = mapEventRefJournals(a.Refs, remapJournal)
			ambiguous[i] = a
		}
		art.Evidence.Ambiguous = ambiguous
	}
	return art
}

func requireLinkedRunIdentity(t *testing.T, art Artifact, runID, journalPath string) {
	t.Helper()
	if art.Evidence == nil {
		t.Fatal("relocated artifact dropped evidence")
	}
	if art.Evidence.Recovered != 2 || art.Evidence.Attempts != 2 || art.Evidence.UniqueRows != 2 || len(art.Evidence.Observations) != 2 {
		t.Fatalf("relocated counts recovered=%d attempts=%d unique=%d obs=%d", art.Evidence.Recovered, art.Evidence.Attempts, art.Evidence.UniqueRows, len(art.Evidence.Observations))
	}
	for i, obs := range art.Evidence.Observations {
		if obs.Attempt.ID.RunID != runID {
			t.Fatalf("observation %d attempt run %q, want %q", i, obs.Attempt.ID.RunID, runID)
		}
		if obs.Cell.Model != "stamp" || !obs.Attempt.Model.Known || obs.Attempt.Model.Value != "stamp" || obs.Attempt.Outcome != OutcomeDone {
			t.Fatalf("observation %d model/outcome changed: cell=%q model=%+v outcome=%s", i, obs.Cell.Model, obs.Attempt.Model, obs.Attempt.Outcome)
		}
		_ = mapAttemptJournals(obs.Attempt, func(j JournalIdentity) JournalIdentity {
			if j.RunID != runID || j.Path != journalPath {
				t.Fatalf("observation %d journal %+v, want run %q path %q", i, j, runID, journalPath)
			}
			if j.SourceID != "journals" || j.Producer != ProducerDispatcherV0_1_0 {
				t.Fatalf("observation %d journal source/producer changed: %+v", i, j)
			}
			return j
		})
	}
	for i, row := range art.Evidence.Examined {
		if row.Disposition != DispositionRecovered && row.Disposition != DispositionDuplicateReading {
			continue
		}
		if !row.Identity.RunID.Known || row.Identity.RunID.Value != runID {
			t.Fatalf("examined %d identity run %+v, want %q", i, row.Identity.RunID, runID)
		}
		if row.Attempt.RunID != runID {
			t.Fatalf("examined %d attempt run %q, want %q", i, row.Attempt.RunID, runID)
		}
	}
	for i, id := range art.Evidence.LostAttempts {
		if id.RunID != "" && id.RunID != runID {
			t.Fatalf("lost attempt %d run %q, want %q", i, id.RunID, runID)
		}
	}
	for i, j := range art.Evidence.ExcludedJournals {
		if j.RunID != runID || j.Path != journalPath {
			t.Fatalf("excluded journal %d %+v, want run %q path %q", i, j, runID, journalPath)
		}
	}
	for i, c := range art.Evidence.Conflicts {
		if c.ID.RunID != "" && c.ID.RunID != runID {
			t.Fatalf("conflict %d run %q, want %q", i, c.ID.RunID, runID)
		}
	}
	for i, a := range art.Evidence.Ambiguous {
		if a.ID.RunID != "" && a.ID.RunID != runID {
			t.Fatalf("ambiguous %d run %q, want %q", i, a.ID.RunID, runID)
		}
	}
}

func updateManifestSource(art Artifact, id string, fn func(*SourceReport)) Artifact {
	sources := append([]SourceReport{}, art.SourceManifest.Sources...)
	for i := range sources {
		if sources[i].ID == id {
			fn(&sources[i])
		}
	}
	art.SourceManifest.Sources = sources
	return art
}

func canonicalHistoryReport(id, repo, tip string, roots ...string) SourceReport {
	refs := []ResolvedRef{
		{Name: "HEAD", Commit: tip},
		{Name: "refs/heads/main", Commit: tip},
	}
	if refs[0].Name > refs[1].Name || refs[0].Name == refs[1].Name && refs[0].Commit > refs[1].Commit {
		refs[0], refs[1] = refs[1], refs[0]
	}
	return SourceReport{
		ID:           id,
		Kind:         SourceKindGitHistory,
		Repository:   repo,
		Roots:        append([]string{}, roots...),
		State:        SourceComplete,
		Reasons:      []string{},
		ResolvedRefs: refs,
		Counts:       SourceCounts{Commits: 1, Blobs: 1, Records: 1},
	}
}

func withSameRefDuplicateCompletion(art Artifact, recovered, duplicate Measured[time.Time]) Artifact {
	obs := art.Evidence.Observations[0]
	obs.Readings = append(append([]ReadingRef{}, obs.Readings...), obs.Readings[0])
	art.Evidence.Observations[0] = obs
	art.Evidence.Examined[0].CompletedAt = recovered
	extra := art.Evidence.Examined[0]
	extra.Disposition = DispositionDuplicateReading
	extra.Reason = "additional compatible reading of recovered attempt"
	extra.CompletedAt = duplicate
	art.Evidence.Examined = append(append([]Examined{}, art.Evidence.Examined...), extra)
	setDispositionCount(art, DispositionDuplicateReading, 1)
	return art
}

func spawnCitation(att Attempt, seq, line int) EventRef {
	ref := att.Start
	ref.Type = EventTaskSpawnFinished
	ref.Seq = seq
	ref.Line = line
	ref.At = att.ID.StartedAt.Add(time.Minute)
	return ref
}

func testF4EligibleAuditBinding(t *testing.T) {
	start := recoveredStart()
	terminal := start.Add(10 * time.Minute)
	pst := time.FixedZone("UTC-8", -8*3600)

	t.Run("journal-completed-at-does-not-infer-yaml-terminal", func(t *testing.T) {
		art := recoveredArtifact(2)
		if art.Evidence.RowsWithYAMLOnlyTerminal != 0 {
			t.Fatalf("positive fixture YAML-only terminal count = %d", art.Evidence.RowsWithYAMLOnlyTerminal)
		}
		if art.Evidence.Observations[0].Attempt.Evidence.Terminal.Source != EvidenceJournal {
			t.Fatalf("positive fixture terminal source = %q", art.Evidence.Observations[0].Attempt.Evidence.Terminal.Source)
		}
		if !art.Evidence.Examined[0].CompletedAt.Known {
			t.Fatal("positive fixture dropped independently retained CompletedAt")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("unfinished-known-completed-at-does-not-infer-terminal", func(t *testing.T) {
		art := unfinishedObservationAtCutoff(recoveredArtifact(3), 2)
		art.Evidence.Examined[2].CompletedAt = Known(art.SourceManifest.Cutoff)
		if art.Evidence.Observations[2].Attempt.Evidence.Terminal.Source != EvidenceNone {
			t.Fatalf("unfinished terminal source = %q", art.Evidence.Observations[2].Attempt.Evidence.Terminal.Source)
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("utc-equivalent-identity-start", func(t *testing.T) {
		art := recoveredArtifact(2)
		art.Evidence.Examined[0].Identity.StartedAt = Known(start.In(pst))
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("yaml-terminal-matching-known-plus-unknown-duplicate", func(t *testing.T) {
		art := withYAMLOnlyTerminal(recoveredArtifact(2), 0, terminal)
		art = withSameRefDuplicateCompletion(art, Known(terminal), Unknown[time.Time]())
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("yaml-terminal-unknown-recovered-plus-matching-duplicate", func(t *testing.T) {
		art := withYAMLOnlyTerminal(recoveredArtifact(2), 0, terminal)
		art = withSameRefDuplicateCompletion(art, Unknown[time.Time](), Known(terminal))
		requireEligibleCompleted(t, art, 2)
	})

	for _, tc := range []struct {
		name   string
		mutate func(Artifact) Artifact
	}{
		{name: "recovered-identity-absent", mutate: func(art Artifact) Artifact {
			art.Evidence.Examined[0].Identity = ReadingIdentity{}
			return art
		}},
		{name: "recovered-identity-wrong-run", mutate: func(art Artifact) Artifact {
			art.Evidence.Examined[0].Identity.RunID = Known("run-b")
			return art
		}},
		{name: "recovered-identity-wrong-key", mutate: func(art Artifact) Artifact {
			art.Evidence.Examined[0].Identity.Key = Known("KZ")
			return art
		}},
		{name: "recovered-identity-wrong-start", mutate: func(art Artifact) Artifact {
			art.Evidence.Examined[0].Identity.StartedAt = Known(start.Add(time.Second))
			return art
		}},
		{name: "recovered-identity-held-out", mutate: func(art Artifact) Artifact {
			art.SourceManifest.HoldoutRunIDs = []string{"held"}
			art.Evidence.Examined[0].Identity.RunID = Known("held")
			return art
		}},
		{name: "duplicate-identity-absent", mutate: func(art Artifact) Artifact {
			art = withDuplicateReadingCompletedAt(art, terminal)
			art.Evidence.Examined[len(art.Evidence.Examined)-1].Identity = ReadingIdentity{}
			return art
		}},
		{name: "duplicate-identity-wrong-run", mutate: func(art Artifact) Artifact {
			art = withDuplicateReadingCompletedAt(art, terminal)
			art.Evidence.Examined[len(art.Evidence.Examined)-1].Identity.RunID = Known("run-b")
			return art
		}},
		{name: "duplicate-identity-held-out", mutate: func(art Artifact) Artifact {
			art = withDuplicateReadingCompletedAt(art, terminal)
			art.SourceManifest.HoldoutRunIDs = []string{"held"}
			art.Evidence.Examined[len(art.Evidence.Examined)-1].Identity.RunID = Known("held")
			return art
		}},
		{name: "yaml-terminal-unknown-completion", mutate: func(art Artifact) Artifact {
			art = withYAMLOnlyTerminal(art, 0, terminal)
			art.Evidence.Examined[0].CompletedAt = Unknown[time.Time]()
			return art
		}},
		{name: "yaml-terminal-unequal-completion", mutate: func(art Artifact) Artifact {
			art = withYAMLOnlyTerminal(art, 0, terminal)
			art.Evidence.Examined[0].CompletedAt = Known(terminal.Add(time.Second))
			return art
		}},
		{name: "yaml-terminal-both-same-ref-unknown", mutate: func(art Artifact) Artifact {
			art = withYAMLOnlyTerminal(art, 0, terminal)
			return withSameRefDuplicateCompletion(art, Unknown[time.Time](), Unknown[time.Time]())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefuseInvalid(t, tc.mutate(recoveredArtifact(2)))
		})
	}
}

func testF4EligibleProvenance(t *testing.T) {
	tip := strings.Repeat("cd", 20)
	ancestor := strings.Repeat("ab", 20)

	t.Run("live-baseline", func(t *testing.T) {
		requireEligibleCompleted(t, recoveredArtifact(2), 2)
	})

	t.Run("history-ancestor-need-not-equal-tip", func(t *testing.T) {
		art := recoveredArtifact(2)
		art.SourceManifest.Sources = append(append([]SourceReport{}, art.SourceManifest.Sources...), canonicalHistoryReport("history", "repo", tip, "features"))
		art = mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
			ref.SourceID = "history"
			ref.Revision = "git:" + ancestor
			return ref
		})
		if ancestor == tip {
			t.Fatal("fixture ancestor SHA equals captured tip")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("legal-spaces-in-repository-and-path", func(t *testing.T) {
		art := updateManifestSource(recoveredArtifact(2), "live", func(src *SourceReport) {
			src.Repository = "repo with spaces"
		})
		art = mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
			ref.Repository = "repo with spaces"
			ref.Path = "features/study/my tasks.yaml"
			return ref
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("root-dot", func(t *testing.T) {
		art := updateManifestSource(recoveredArtifact(2), "live", func(src *SourceReport) {
			src.Roots = []string{"."}
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("colon-in-relative-path", func(t *testing.T) {
		const yamlPath = "features/archive:notes/tasks.yaml"
		art := mapAllRecoveredYAML(recoveredArtifact(2), func(ref ReadingRef) ReadingRef {
			ref.Path = yamlPath
			return ref
		})
		if art.Evidence.Recovered != 2 || len(art.Evidence.Observations) != 2 {
			t.Fatalf("colon citation dropped counts recovered=%d obs=%d", art.Evidence.Recovered, len(art.Evidence.Observations))
		}
		for i, obs := range art.Evidence.Observations {
			if obs.Readings[0].Path != yamlPath || obs.Attempt.Evidence.Role.Reading.Path != yamlPath {
				t.Fatalf("observation %d YAML path not relinked: %+v", i, obs.Readings)
			}
			if obs.Cell.Model != "stamp" || obs.Attempt.Outcome != OutcomeDone {
				t.Fatalf("observation %d model/outcome changed", i)
			}
		}
		requireEligibleCompleted(t, art, 2)
	})

	for _, tc := range []struct {
		name   string
		mutate func(Artifact) Artifact
	}{
		{name: "unknown-producer", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Producer = "evil-producer-9"
				return j
			})
		}},
		{name: "ghost-yaml-source", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.SourceID = "ghost-yaml"
				return ref
			})
		}},
		{name: "ghost-journal-source", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.SourceID = "ghost-journal"
				return j
			})
		}},
		{name: "yaml-cites-journal-source-kind", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.SourceID = "journals"
				ref.Repository = "/tmp/runs"
				return ref
			})
		}},
		{name: "journal-cites-live-source-kind", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.SourceID = "live"
				return j
			})
		}},
		{name: "live-revision-on-history-source", mutate: func(art Artifact) Artifact {
			art.SourceManifest.Sources = append(append([]SourceReport{}, art.SourceManifest.Sources...), canonicalHistoryReport("history", "repo", tip, "features"))
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.SourceID = "history"
				ref.Revision = "live"
				return ref
			})
		}},
		{name: "history-revision-on-live-source", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Revision = "git:" + ancestor
				return ref
			})
		}},
		{name: "wrong-yaml-repository", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Repository = "other-repo"
				return ref
			})
		}},
		{name: "wrong-live-source-repository", mutate: func(art Artifact) Artifact {
			return updateManifestSource(art, "live", func(src *SourceReport) {
				src.Repository = "other-repo"
			})
		}},
		{name: "yaml-path-escapes", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Path = "../secret.yaml"
				return ref
			})
		}},
		{name: "yaml-path-absolute", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Path = "/tmp/tasks.yaml"
				return ref
			})
		}},
		{name: "yaml-path-out-of-root", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Path = "dispatcher/tasks.yaml"
				return ref
			})
		}},
		{name: "journal-path-escapes", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Path = "../run-a/journal.jsonl"
				return j
			})
		}},
		{name: "journal-path-absolute", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Path = "/tmp/runs/run-a/journal.jsonl"
				return j
			})
		}},
		{name: "journal-path-not-direct-child", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Path = "journal.jsonl"
				return j
			})
		}},
		{name: "journal-path-nested", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Path = "run-a/nested/journal.jsonl"
				return j
			})
		}},
		{name: "journal-path-wrong-run", mutate: func(art Artifact) Artifact {
			return mapAllRecoveredJournals(art, func(j JournalIdentity) JournalIdentity {
				j.Path = "run-b/journal.jsonl"
				return j
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefuseInvalid(t, tc.mutate(recoveredArtifact(2)))
		})
	}

	t.Run("ordinary-direct-child-run", func(t *testing.T) {
		art := recoveredArtifact(2)
		requireLinkedRunIdentity(t, art, "run-a", "run-a/journal.jsonl")
		if path.Join("run-a", "journal.jsonl") != "run-a/journal.jsonl" {
			t.Fatal("ordinary fixture path is not a direct-child journal.jsonl")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("legal-internal-space-run-directory", func(t *testing.T) {
		const runID = "run a"
		journalPath := path.Join(runID, "journal.jsonl")
		if journalPath != "run a/journal.jsonl" {
			t.Fatalf("space run path = %q", journalPath)
		}
		art := withLinkedRunIdentity(recoveredArtifact(2), runID, journalPath)
		requireLinkedRunIdentity(t, art, runID, journalPath)
		requireEligibleCompleted(t, art, 2)
	})

	for _, tc := range []struct {
		name, runID, journalPath string
	}{
		{name: "nested-run-id", runID: "parent/run-a", journalPath: "parent/run-a/journal.jsonl"},
		{name: "dot-cleaned-run-id", runID: ".", journalPath: "journal.jsonl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if path.Join(tc.runID, "journal.jsonl") != tc.journalPath {
				t.Fatalf("path %q is not path.Join(%q, journal.jsonl)=%q", tc.journalPath, tc.runID, path.Join(tc.runID, "journal.jsonl"))
			}
			art := withLinkedRunIdentity(recoveredArtifact(2), tc.runID, tc.journalPath)
			requireLinkedRunIdentity(t, art, tc.runID, tc.journalPath)
			requireRefuseInvalid(t, art)
		})
	}

	// Host-independent drive-prefix rejection. These spellings are the
	// operator-confirmed gap under selected root ".". filepath.IsAbs on this
	// host is not the portable proof, and a colon that is not an ASCII-letter
	// drive prefix remains a valid relative citation (colon-in-relative-path).
	for _, tc := range []struct {
		name, yamlPath string
	}{
		{name: "yaml-drive-absolute-upper", yamlPath: "C:/outside/tasks.yaml"},
		{name: "yaml-drive-absolute-lower", yamlPath: "c:/outside/tasks.yaml"},
		{name: "yaml-drive-relative-upper", yamlPath: "C:outside/tasks.yaml"},
		{name: "yaml-drive-relative-lower", yamlPath: "c:outside/tasks.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			art := updateManifestSource(recoveredArtifact(2), "live", func(src *SourceReport) {
				src.Roots = []string{"."}
			})
			art = mapAllRecoveredYAML(art, func(ref ReadingRef) ReadingRef {
				ref.Path = tc.yamlPath
				return ref
			})
			if art.Evidence.Recovered != 2 || art.Evidence.Attempts != 2 || art.Evidence.UniqueRows != 2 {
				t.Fatalf("YAML relink dropped counts recovered=%d attempts=%d unique=%d", art.Evidence.Recovered, art.Evidence.Attempts, art.Evidence.UniqueRows)
			}
			for i, obs := range art.Evidence.Observations {
				if obs.Readings[0].Path != tc.yamlPath || obs.Attempt.Evidence.Role.Reading.Path != tc.yamlPath {
					t.Fatalf("observation %d YAML path not relinked: %+v", i, obs.Readings)
				}
				if obs.Cell.Model != "stamp" || obs.Attempt.Outcome != OutcomeDone {
					t.Fatalf("observation %d model/outcome changed", i)
				}
			}
			requireRefuseInvalid(t, art)
		})
	}

	for _, tc := range []struct {
		name, runID, journalPath string
	}{
		{name: "journal-drive-absolute-upper", runID: "C:", journalPath: "C:/journal.jsonl"},
		{name: "journal-drive-absolute-lower", runID: "c:", journalPath: "c:/journal.jsonl"},
		{name: "journal-drive-relative-upper", runID: "C:run", journalPath: "C:run/journal.jsonl"},
		{name: "journal-drive-relative-lower", runID: "c:run", journalPath: "c:run/journal.jsonl"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if path.Join(tc.runID, "journal.jsonl") != tc.journalPath {
				t.Fatalf("path %q is not path.Join(%q, journal.jsonl)=%q", tc.journalPath, tc.runID, path.Join(tc.runID, "journal.jsonl"))
			}
			art := withLinkedRunIdentity(recoveredArtifact(2), tc.runID, tc.journalPath)
			requireLinkedRunIdentity(t, art, tc.runID, tc.journalPath)
			requireRefuseInvalid(t, art)
		})
	}
}

func testF4EligibleLocalFixture(t *testing.T) {
	// Independent source facts: testdata journals/synthetic-utc-offset.jsonl
	// declares producer 0.1.0 and one done implementer-stamped F1-OFFSET
	// attempt (model stamp). testdata yaml/offset-equivalent.yaml supplies
	// role bodies and the matching run/key/start. Live plus history are two
	// readings of that one attempt, so completed bodies/stamp = 1.
	const wantCompleted = 1
	repo := initGitRepo(t)
	repo.writeTestdata("features/study/tasks.yaml", "yaml", "offset-equivalent.yaml")
	repo.commit("training yaml", "features/study/tasks.yaml")
	requireFixtureCommitBeforeCutoff(t, repo.path)
	runs := filepath.Join(t.TempDir(), "runs")
	writeJournalTree(t, runs, "run-offset", "synthetic-utc-offset.jsonl")
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{
			journalSpec("journals", runs),
			liveSpec("live", repo.path, "features"),
			historySpec("history", repo.path, "", "features"),
		},
		defaultSelection(),
		ReadBounds{},
	))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Artifact.SourceManifest == nil || result.Artifact.Evidence == nil {
		t.Fatal("local fixture Build dropped the artifact")
	}
	if result.Artifact.SourceManifest.State != SourceComplete {
		t.Fatalf("local fixture state = %q reasons=%v", result.Artifact.SourceManifest.State, result.Artifact.SourceManifest.Reasons)
	}
	got, eligErr := PredictionEligibility(result.Artifact, eligibleTargets(), wantCompleted, true)
	if eligErr != nil {
		t.Fatal(eligErr)
	}
	if !got.Eligible || len(got.Cells) != 1 || got.Cells[0].Completed != wantCompleted || got.Cells[0].Role != RoleBodies || got.Cells[0].Model != "stamp" {
		t.Fatalf("local fixture eligibility = %+v, recovered=%d", got, result.Artifact.Evidence.Recovered)
	}
	if result.Artifact.Evidence.Recovered != wantCompleted {
		t.Fatalf("recovered = %d, want %d from the authored F1-OFFSET attempt", result.Artifact.Evidence.Recovered, wantCompleted)
	}
	if _, statErr := os.Stat(filepath.Join(runs, "run-offset", "journal.jsonl")); statErr != nil {
		t.Fatalf("expected local journal missing: %v", statErr)
	}
}

func readingPermutations(readings []Reading) [][]Reading {
	switch len(readings) {
	case 2:
		return [][]Reading{
			{readings[0], readings[1]},
			{readings[1], readings[0]},
		}
	case 3:
		a, b, c := readings[0], readings[1], readings[2]
		return [][]Reading{
			{a, b, c}, {a, c, b}, {b, a, c}, {b, c, a}, {c, a, b}, {c, b, a},
		}
	default:
		return nil
	}
}

func testF1SameRefPermutation(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	terminal := start.Add(10 * time.Minute)
	base := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	base.Snapshot.Status = "Done"
	unknown := base
	unknown.CompletedAt = Unknown[time.Time]()
	known := base
	known.CompletedAt = Known(terminal)
	knownCopy := known

	t.Run("matched-two", func(t *testing.T) {
		att := journalAttempt("run-a", "K", start, contractCutoff().Sub(start), "stamp", OutcomeUnfinished)
		att.Evidence.Terminal = FieldEvidence{}
		att.Evidence.Elapsed = FieldEvidence{}
		att.TerminalAt = time.Time{}
		set := attemptSetOf("run-a", att)
		requireSameRefJoinPermutations(t, []AttemptSet{set}, []Reading{unknown, known}, 1, 1, true)
	})

	t.Run("matched-three", func(t *testing.T) {
		att := journalAttempt("run-a", "K", start, contractCutoff().Sub(start), "stamp", OutcomeUnfinished)
		att.Evidence.Terminal = FieldEvidence{}
		att.Evidence.Elapsed = FieldEvidence{}
		att.TerminalAt = time.Time{}
		set := attemptSetOf("run-a", att)
		requireSameRefJoinPermutations(t, []AttemptSet{set}, []Reading{unknown, known, knownCopy}, 1, 2, true)
	})

	t.Run("unrecovered-examined-ties", func(t *testing.T) {
		keep := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
		set := attemptSetOf("run-a", keep)
		keepReading := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
		future := contractCutoff().Add(time.Minute)
		shared := ReadingRef{
			SourceID:   "live",
			Repository: "repo",
			Path:       "features/late/tasks.yaml",
			Revision:   "live",
			Row:        1,
			RecordedAt: future,
		}
		a := syntheticReading("late-a", "LA", future, 1, "features/late/tasks.yaml")
		a.Ref = shared
		a.CompletedAt = Known(future)
		b := syntheticReading("late-b", "LB", future.Add(time.Second), 1, "features/late/tasks.yaml")
		b.Ref = shared
		b.CompletedAt = Known(future.Add(2 * time.Second))
		if a.Ref != b.Ref {
			t.Fatal("unrecovered fixtures drifted apart")
		}
		if a.Identity == b.Identity || a.CompletedAt == b.CompletedAt {
			t.Fatal("unrecovered fixtures must differ in retained identity and CompletedAt")
		}
		var encoded [][]byte
		for _, readings := range readingPermutations([]Reading{keepReading, a, b}) {
			got := mustJoin(t, []AttemptSet{set}, readings, defaultSelection(), identityUniverse(set))
			requireDisposition(t, got, DispositionRecovered, 1)
			requireDisposition(t, got, DispositionAfterCutoff, 2)
			encoded = append(encoded, encodeJSON(t, got))
		}
		if len(encoded) == 0 {
			t.Fatal("no unrecovered permutations")
		}
		for i := 1; i < len(encoded); i++ {
			if !bytes.Equal(encoded[0], encoded[i]) {
				t.Fatal("same-ref unrecovered Examined order depended on input order")
			}
		}
	})
}

func requireSameRefJoinPermutations(t *testing.T, sets []AttemptSet, readings []Reading, recovered, duplicates int, yamlTerminal bool) {
	t.Helper()
	universe := identityUniverse(sets...)
	var encoded [][]byte
	for _, order := range readingPermutations(readings) {
		got := mustJoin(t, sets, order, defaultSelection(), universe)
		if got.Recovered != recovered || len(got.Observations) != recovered {
			t.Fatalf("recovered=%d observations=%d, want %d", got.Recovered, len(got.Observations), recovered)
		}
		requireDisposition(t, got, DispositionRecovered, recovered)
		requireDisposition(t, got, DispositionDuplicateReading, duplicates)
		if yamlTerminal {
			if got.RowsWithYAMLOnlyTerminal != 1 {
				t.Fatalf("RowsWithYAMLOnlyTerminal = %d", got.RowsWithYAMLOnlyTerminal)
			}
			var want time.Time
			for _, reading := range readings {
				if reading.CompletedAt.Known {
					want = reading.CompletedAt.Value
					break
				}
			}
			obs := got.Observations[0]
			if obs.Attempt.Evidence.Terminal.Source != EvidenceYAML || want.IsZero() || !obs.Attempt.TerminalAt.Equal(want) {
				t.Fatalf("YAML terminal not supported: source=%q terminal_at=%s", obs.Attempt.Evidence.Terminal.Source, obs.Attempt.TerminalAt)
			}
		}
		encoded = append(encoded, encodeJSON(t, got))
	}
	if len(encoded) < 2 {
		t.Fatal("need at least two permutations")
	}
	for i := 1; i < len(encoded); i++ {
		if !bytes.Equal(encoded[0], encoded[i]) {
			t.Fatal("EvidenceJoin JSON depended on same-Ref input order")
		}
	}
}

func testF4EligibleNumericDomain(t *testing.T) {
	t.Run("unknown-optional-remain-unknown", func(t *testing.T) {
		art := recoveredArtifact(2)
		obs := art.Evidence.Observations[0].Attempt
		if obs.CostUSD.Known || obs.InputTokens.Known || obs.OutputTokens.Known {
			t.Fatal("positive fixture invented optional measurements")
		}
		if obs.Evidence.Cost.Source != EvidenceNone || obs.Evidence.InputTokens.Source != EvidenceNone || obs.Evidence.OutputTokens.Source != EvidenceNone {
			t.Fatal("unknown optional measurements must not require citations")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("known-zero", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			obs.Attempt.CostUSD = Known(0.0)
			obs.Attempt.InputTokens = Known(int64(0))
			obs.Attempt.OutputTokens = Known(int64(0))
			obs.Attempt.Corrections = 0
			obs.Attempt.Cascades = 0
			obs.Attempt.Reviews = 0
			obs.Attempt.Verifications = 0
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("known-positive-with-citations", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			cost := spawnCitation(obs.Attempt, 4, 5)
			tokens := spawnCitation(obs.Attempt, 5, 6)
			correction := spawnCitation(obs.Attempt, 6, 7)
			correction.Type = EventPanelIterate
			cascade := spawnCitation(obs.Attempt, 7, 8)
			cascade.Type = EventAgentFallback
			review := spawnCitation(obs.Attempt, 8, 9)
			review.Type = EventPanelStarted
			verify := spawnCitation(obs.Attempt, 9, 10)
			verify.Type = EventVerificationStarted
			obs.Attempt.CostUSD = Known(1.5)
			obs.Attempt.CostEvents = []EventRef{cost}
			obs.Attempt.Evidence.Cost = FieldEvidence{Source: EvidenceJournal, Event: cost}
			obs.Attempt.InputTokens = Known(int64(3))
			obs.Attempt.InputTokenEvents = []EventRef{tokens}
			obs.Attempt.Evidence.InputTokens = FieldEvidence{Source: EvidenceJournal, Event: tokens}
			obs.Attempt.OutputTokens = Known(int64(4))
			obs.Attempt.OutputTokenEvents = []EventRef{tokens}
			obs.Attempt.Evidence.OutputTokens = FieldEvidence{Source: EvidenceJournal, Event: tokens}
			obs.Attempt.Corrections = 2
			obs.Attempt.CorrectionEvents = []EventRef{correction}
			obs.Attempt.Evidence.Corrections = FieldEvidence{Source: EvidenceJournal, Event: correction}
			obs.Attempt.Cascades = 3
			obs.Attempt.CascadeEvents = []EventRef{cascade}
			obs.Attempt.Evidence.Cascades = FieldEvidence{Source: EvidenceJournal, Event: cascade}
			obs.Attempt.Reviews = 1
			obs.Attempt.ReviewEvents = []EventRef{review}
			obs.Attempt.Evidence.Reviews = FieldEvidence{Source: EvidenceJournal, Event: review}
			obs.Attempt.Verifications = 1
			obs.Attempt.VerificationEvents = []EventRef{verify}
			obs.Attempt.Evidence.Verifications = FieldEvidence{Source: EvidenceJournal, Event: verify}
			if obs.Attempt.Corrections == len(obs.Attempt.CorrectionEvents) || obs.Attempt.Cascades == len(obs.Attempt.CascadeEvents) {
				t.Fatal("positive control must not make counts equal event-list length")
			}
		})
		got := art.Evidence.Observations[0].Attempt
		if !got.CostUSD.Known || got.CostUSD.Value != 1.5 || got.Evidence.Cost.Event != got.CostEvents[0] {
			t.Fatalf("cost citation lost: %+v", got.Evidence.Cost)
		}
		requireEligibleCompleted(t, art, 2)
	})

	for _, tc := range []struct {
		name   string
		mutate func(*RecoveredAttempt)
	}{
		{name: "negative-cost", mutate: func(obs *RecoveredAttempt) { obs.Attempt.CostUSD = Known(-1.0) }},
		{name: "nan-cost", mutate: func(obs *RecoveredAttempt) { obs.Attempt.CostUSD = Known(math.NaN()) }},
		{name: "pos-inf-cost", mutate: func(obs *RecoveredAttempt) { obs.Attempt.CostUSD = Known(math.Inf(1)) }},
		{name: "neg-inf-cost", mutate: func(obs *RecoveredAttempt) { obs.Attempt.CostUSD = Known(math.Inf(-1)) }},
		{name: "negative-input-tokens", mutate: func(obs *RecoveredAttempt) { obs.Attempt.InputTokens = Known(int64(-1)) }},
		{name: "negative-output-tokens", mutate: func(obs *RecoveredAttempt) { obs.Attempt.OutputTokens = Known(int64(-1)) }},
		{name: "negative-corrections", mutate: func(obs *RecoveredAttempt) { obs.Attempt.Corrections = -1 }},
		{name: "negative-cascades", mutate: func(obs *RecoveredAttempt) { obs.Attempt.Cascades = -1 }},
		{name: "negative-reviews", mutate: func(obs *RecoveredAttempt) { obs.Attempt.Reviews = -1 }},
		{name: "negative-verifications", mutate: func(obs *RecoveredAttempt) { obs.Attempt.Verifications = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefuseInvalid(t, mutateObservation(recoveredArtifact(2), tc.mutate))
		})
	}
}

func conflictPermutations(facts []AttemptConflict) [][]AttemptConflict {
	switch len(facts) {
	case 2:
		return [][]AttemptConflict{
			{facts[0], facts[1]},
			{facts[1], facts[0]},
		}
	case 3:
		a, b, c := facts[0], facts[1], facts[2]
		return [][]AttemptConflict{
			{a, b, c}, {a, c, b}, {b, a, c}, {b, c, a}, {c, a, b}, {c, b, a},
		}
	default:
		return nil
	}
}

func modelConflictValue(name string) ConflictValue {
	return ConflictValue{Kind: "model", Value: json.RawMessage(`"` + name + `"`)}
}

func roleConflictValue(name string) ConflictValue {
	return ConflictValue{Kind: "role", Value: json.RawMessage(`"` + name + `"`)}
}

func testF1ConflictTotalOrder(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	id := NewAttemptID("run-a", "KC", start)
	journal := JournalIdentity{RunID: "run-a", SourceID: "journals", Path: "journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	matching := []Reading{syntheticReading("run-a", "KC", start, 1, "features/study/tasks.yaml")}

	modelEvent := func(seq, line int) EventRef {
		return EventRef{
			Journal: journal, Type: EventTaskSpawnFinished,
			At: start.Add(time.Minute), HasSeq: true, Seq: seq, Line: line,
		}
	}
	modelCite := func(seq, line int) FieldEvidence {
		return FieldEvidence{Source: EvidenceJournal, Event: modelEvent(seq, line)}
	}
	aCite := modelCite(1, 2)
	aVal := modelConflictValue("modelA")
	modelFact := func(bName string, bSeq, bLine int, errText string) AttemptConflict {
		return AttemptConflict{
			Code:   EvidenceConflictCode,
			ID:     id,
			Field:  "model",
			AValue: aVal,
			BValue: modelConflictValue(bName),
			A:      aCite,
			B:      modelCite(bSeq, bLine),
			Err:    errors.New(errText),
			Reason: "model evidence conflicts within run-a/KC",
		}
	}

	yamlRef := func(row int) ReadingRef {
		return ReadingRef{
			SourceID:   "live",
			Repository: "repo",
			Path:       "features/study/tasks.yaml",
			Revision:   "live",
			Row:        row,
			RecordedAt: start,
		}
	}
	yamlCite := func(row int) FieldEvidence {
		return FieldEvidence{Source: EvidenceYAML, Reading: yamlRef(row)}
	}
	roleFact := func(aRow, bRow int, reason, errText string) AttemptConflict {
		return AttemptConflict{
			Code:   EvidenceConflictCode,
			ID:     id,
			Field:  "role",
			AValue: roleConflictValue("bodies"),
			BValue: roleConflictValue("seals"),
			A:      yamlCite(aRow),
			B:      yamlCite(bRow),
			Err:    errors.New(errText),
			Reason: reason,
		}
	}

	twoB := []AttemptConflict{
		modelFact("modelB", 5, 6, "not serialized b"),
		modelFact("modelC", 9, 10, "not serialized c"),
	}
	threeB := []AttemptConflict{
		modelFact("modelB", 5, 6, "not serialized b"),
		modelFact("modelC", 9, 10, "not serialized c"),
		modelFact("modelD", 7, 8, "not serialized d"),
	}
	citationTwo := []AttemptConflict{
		roleFact(1, 2, "role evidence conflicts within run-a/KC", "not serialized row-2"),
		roleFact(1, 3, "role evidence conflicts within run-a/KC", "not serialized row-3"),
	}
	reasonTwo := []AttemptConflict{
		roleFact(1, 2, "role evidence conflicts within run-a/KC", "not serialized reason-a"),
		roleFact(1, 2, "later revision restates the same role conflict", "not serialized reason-b"),
	}
	citationReasonThree := []AttemptConflict{
		roleFact(1, 2, "role evidence conflicts within run-a/KC", "not serialized mix-a"),
		roleFact(1, 4, "role evidence conflicts within run-a/KC", "not serialized mix-b"),
		roleFact(8, 2, "later revision restates the same role conflict", "not serialized mix-c"),
	}
	fieldPrimary := []AttemptConflict{
		{
			Code: EvidenceConflictCode, ID: id, Field: "model",
			AValue: aVal, BValue: modelConflictValue("modelB"),
			A: aCite, B: modelCite(5, 6),
			Err: errors.New("not serialized model"), Reason: "model",
		},
		{
			Code: EvidenceConflictCode, ID: id, Field: "terminal",
			AValue: ConflictValue{Kind: "terminal", Value: json.RawMessage(`{"elapsed_ns":600000000000,"outcome":"done","terminal_at":"2026-01-01T00:10:00Z"}`)},
			BValue: ConflictValue{Kind: "terminal", Value: json.RawMessage(`{"elapsed_ns":720000000000,"outcome":"blocked","terminal_at":"2026-01-01T00:12:00Z"}`)},
			A:      FieldEvidence{Source: EvidenceJournal, Event: modelEvent(3, 4)},
			B:      FieldEvidence{Source: EvidenceYAML, Reading: yamlRef(2)},
			Err:    errors.New("not serialized terminal"), Reason: "terminal",
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
			t.Run("field-primary-order", func(t *testing.T) {
				requireConflictJoinPermutations(t, fieldPrimary, path.readings)
			})
			t.Run("b-event-value-two", func(t *testing.T) {
				requireConflictJoinPermutations(t, twoB, path.readings)
			})
			t.Run("b-event-value-three", func(t *testing.T) {
				requireConflictJoinPermutations(t, threeB, path.readings)
			})
			t.Run("complete-citation-ties", func(t *testing.T) {
				requireConflictJoinPermutations(t, citationTwo, path.readings)
			})
			t.Run("reason-ties", func(t *testing.T) {
				requireConflictJoinPermutations(t, reasonTwo, path.readings)
			})
			t.Run("citation-reason-three", func(t *testing.T) {
				requireConflictJoinPermutations(t, citationReasonThree, path.readings)
			})
		})
	}
}

func requireConflictJoinPermutations(t *testing.T, facts []AttemptConflict, readings []Reading) {
	t.Helper()
	if len(facts) < 2 {
		t.Fatal("need at least two hand-built conflict facts")
	}
	id := facts[0].ID
	for i, fact := range facts {
		if fact.ID != id {
			t.Fatalf("fact %d identity %+v, want one conflict-category identity %+v", i, fact.ID, id)
		}
	}
	seen := map[string]bool{}
	for _, fact := range facts {
		key := string(encodeJSON(t, fact))
		if seen[key] {
			t.Fatal("hand-built facts are byte-identical portable duplicates")
		}
		seen[key] = true
	}

	type candidateSnap struct{ a, b []byte }
	original := make([]candidateSnap, len(facts))
	beforeFacts := encodeJSON(t, facts)
	beforeReadings := encodeJSON(t, readings)
	for i, fact := range facts {
		original[i] = candidateSnap{
			a: append([]byte(nil), fact.AValue.Value...),
			b: append([]byte(nil), fact.BValue.Value...),
		}
	}

	orders := conflictPermutations(facts)
	if len(orders) < 2 {
		t.Fatal("need at least two permutations")
	}
	var encoded [][]byte
	for _, order := range orders {
		set := attemptSetOf("run-a")
		if len(set.Attempts) != 0 || len(set.Ambiguous) != 0 {
			t.Fatal("competing ordinary or ambiguous category in hand-built set")
		}
		set.Conflicts = append([]AttemptConflict{}, order...)
		sets := []AttemptSet{set}
		beforeSets := encodeJSON(t, sets)
		beforeConflicts := encodeJSON(t, set.Conflicts)
		got, err := joinContract(t, sets, readings, defaultSelection(), identityUniverse(set))
		if !errors.Is(err, ErrEvidenceConflict) {
			t.Fatalf("JoinEvidence err = %v, want ErrEvidenceConflict", err)
		}
		if !bytes.Equal(beforeSets, encodeJSON(t, sets)) {
			t.Fatal("JoinEvidence mutated caller attempt sets")
		}
		if !bytes.Equal(beforeConflicts, encodeJSON(t, set.Conflicts)) {
			t.Fatal("JoinEvidence mutated caller conflict slice")
		}
		if !bytes.Equal(beforeReadings, encodeJSON(t, readings)) {
			t.Fatal("JoinEvidence mutated caller readings")
		}
		if !bytes.Equal(beforeFacts, encodeJSON(t, facts)) {
			t.Fatal("JoinEvidence mutated caller conflict facts")
		}
		for i, fact := range facts {
			if !bytes.Equal(original[i].a, fact.AValue.Value) || !bytes.Equal(original[i].b, fact.BValue.Value) {
				t.Fatal("JoinEvidence mutated candidate bytes")
			}
		}
		if got.Attempts != 1 || got.UniqueRows != 1 {
			t.Fatalf("denominator attempts=%d unique=%d, want 1/1", got.Attempts, got.UniqueRows)
		}
		if got.Recovered != 0 || len(got.Observations) != 0 {
			t.Fatalf("conflict identity recovered a sample: recovered=%d observations=%d", got.Recovered, len(got.Observations))
		}
		if len(got.LostAttempts) != 1 || got.LostAttempts[0] != id {
			t.Fatalf("lost attempts = %+v, want [%+v]", got.LostAttempts, id)
		}
		if len(got.Conflicts) != len(facts) {
			t.Fatalf("retained %d conflicts, want %d: %+v", len(got.Conflicts), len(facts), got.Conflicts)
		}
		want := map[string]int{}
		for _, fact := range facts {
			want[string(encodeJSON(t, fact))]++
		}
		for _, conflict := range got.Conflicts {
			key := string(encodeJSON(t, conflict))
			if want[key] == 0 {
				t.Fatalf("output dropped or swapped a value/citation pairing: %s", encodeJSON(t, conflict))
			}
			want[key]--
		}
		if len(readings) == 0 {
			if len(got.Examined) != 0 {
				t.Fatalf("empty readings examined = %+v", got.Examined)
			}
			requireDisposition(t, got, DispositionConflictingEvidence, 0)
		} else {
			if len(got.Examined) != len(readings) {
				t.Fatalf("matching readings examined = %d, want %d", len(got.Examined), len(readings))
			}
			requireDisposition(t, got, DispositionConflictingEvidence, len(readings))
		}
		encoded = append(encoded, encodeJSON(t, got))
	}
	for i := 1; i < len(encoded); i++ {
		if !bytes.Equal(encoded[0], encoded[i]) {
			t.Fatal("EvidenceJoin JSON depended on conflict input order")
		}
	}
}

package dispatched

import (
	"bytes"
	"context"
	"errors"
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

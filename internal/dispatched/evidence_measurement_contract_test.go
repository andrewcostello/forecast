package dispatched

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func typedCitation(att Attempt, seq, line int, typ string) EventRef {
	ref := spawnCitation(att, seq, line)
	ref.Type = typ
	return ref
}

func journalCite(ref EventRef) FieldEvidence {
	return FieldEvidence{Source: EvidenceJournal, Event: ref}
}

// attachCompletePositiveMeasurements writes a faithful synthetic complete
// measurement block: each count equals its unique canonical list length, and
// each known total cites list[0] as EvidenceJournal. Values are not computed
// from original event payloads.
func attachCompletePositiveMeasurements(obs *RecoveredAttempt) {
	att := obs.Attempt
	cost := typedCitation(att, 4, 5, EventTaskSpawnFinished)
	tokens := typedCitation(att, 5, 6, EventTaskSpawnFinished)
	c1 := typedCitation(att, 6, 7, EventPanelIterate)
	c2 := typedCitation(att, 10, 11, EventVerificationIterate)
	k1 := typedCitation(att, 7, 8, EventAgentFallback)
	k2 := typedCitation(att, 11, 12, EventAgentFallback)
	k3 := typedCitation(att, 12, 13, EventAgentFallback)
	review := typedCitation(att, 8, 9, EventPanelStarted)
	verify := typedCitation(att, 9, 10, EventVerificationStarted)
	obs.Attempt.CostUSD = Known(1.5)
	obs.Attempt.CostEvents = []EventRef{cost}
	obs.Attempt.Evidence.Cost = journalCite(cost)
	obs.Attempt.InputTokens = Known(int64(3))
	obs.Attempt.InputTokenEvents = []EventRef{tokens}
	obs.Attempt.Evidence.InputTokens = journalCite(tokens)
	obs.Attempt.OutputTokens = Known(int64(4))
	obs.Attempt.OutputTokenEvents = []EventRef{tokens}
	obs.Attempt.Evidence.OutputTokens = journalCite(tokens)
	obs.Attempt.Corrections = 2
	obs.Attempt.CorrectionEvents = []EventRef{c1, c2}
	obs.Attempt.Evidence.Corrections = journalCite(c1)
	obs.Attempt.Cascades = 3
	obs.Attempt.CascadeEvents = []EventRef{k1, k2, k3}
	obs.Attempt.Evidence.Cascades = journalCite(k1)
	obs.Attempt.Reviews = 1
	obs.Attempt.ReviewEvents = []EventRef{review}
	obs.Attempt.Evidence.Reviews = journalCite(review)
	obs.Attempt.Verifications = 1
	obs.Attempt.VerificationEvents = []EventRef{verify}
	obs.Attempt.Evidence.Verifications = journalCite(verify)
	obs.Attempt.CostScope = CostScopeRecordedSpawns
}

func testF4EligibleMeasurement(t *testing.T) {
	t.Run("unknown-no-contributors", func(t *testing.T) {
		art := recoveredArtifact(2)
		obs := art.Evidence.Observations[0].Attempt
		if obs.CostUSD.Known || obs.InputTokens.Known || obs.OutputTokens.Known {
			t.Fatal("unknown control invented optional totals")
		}
		if len(obs.CostEvents) != 0 || len(obs.InputTokenEvents) != 0 || len(obs.OutputTokenEvents) != 0 {
			t.Fatal("unknown-no-contributors control grew contributor lists")
		}
		if obs.Evidence.Cost.Source != EvidenceNone || obs.Evidence.InputTokens.Source != EvidenceNone || obs.Evidence.OutputTokens.Source != EvidenceNone {
			t.Fatal("unknown totals must retain EvidenceNone")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("unknown-with-partial-contributor-lists", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			spawn := typedCitation(obs.Attempt, 4, 5, EventTaskSpawnFinished)
			obs.Attempt.CostUSD = Unknown[float64]()
			obs.Attempt.InputTokens = Unknown[int64]()
			obs.Attempt.OutputTokens = Unknown[int64]()
			obs.Attempt.CostEvents = []EventRef{spawn}
			obs.Attempt.InputTokenEvents = []EventRef{spawn}
			obs.Attempt.OutputTokenEvents = []EventRef{spawn}
			obs.Attempt.Evidence.Cost = FieldEvidence{}
			obs.Attempt.Evidence.InputTokens = FieldEvidence{}
			obs.Attempt.Evidence.OutputTokens = FieldEvidence{}
		})
		got := art.Evidence.Observations[0].Attempt
		if got.CostUSD.Known || got.InputTokens.Known || got.OutputTokens.Known {
			t.Fatal("partial-list control made unknown totals known")
		}
		if len(got.CostEvents) != 1 || got.Evidence.Cost.Source != EvidenceNone {
			t.Fatal("unknown totals must keep available contributor lists with EvidenceNone")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("complete-zero-with-spawn-lists", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			spawn := typedCitation(obs.Attempt, 4, 5, EventTaskSpawnFinished)
			obs.Attempt.CostUSD = Known(0.0)
			obs.Attempt.InputTokens = Known(int64(0))
			obs.Attempt.OutputTokens = Known(int64(0))
			obs.Attempt.CostEvents = []EventRef{spawn}
			obs.Attempt.InputTokenEvents = []EventRef{spawn}
			obs.Attempt.OutputTokenEvents = []EventRef{spawn}
			obs.Attempt.Evidence.Cost = journalCite(spawn)
			obs.Attempt.Evidence.InputTokens = journalCite(spawn)
			obs.Attempt.Evidence.OutputTokens = journalCite(spawn)
			obs.Attempt.Corrections = 0
			obs.Attempt.Cascades = 0
			obs.Attempt.Reviews = 0
			obs.Attempt.Verifications = 0
			obs.Attempt.CorrectionEvents = []EventRef{}
			obs.Attempt.CascadeEvents = []EventRef{}
			obs.Attempt.ReviewEvents = []EventRef{}
			obs.Attempt.VerificationEvents = []EventRef{}
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("complete-positive-matching-counts", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			if obs.Attempt.Corrections != 2 || obs.Attempt.Cascades != 3 {
				t.Fatal("positive control dropped Corrections=2 or Cascades=3")
			}
			if obs.Attempt.Corrections != len(obs.Attempt.CorrectionEvents) || obs.Attempt.Cascades != len(obs.Attempt.CascadeEvents) {
				t.Fatal("positive control counts must equal unique list lengths")
			}
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("same-spawn-across-cost-token-correction-lists", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			spawn := typedCitation(obs.Attempt, 4, 5, EventTaskSpawnFinished)
			obs.Attempt.CostUSD = Known(0.25)
			obs.Attempt.InputTokens = Known(int64(2))
			obs.Attempt.OutputTokens = Known(int64(3))
			obs.Attempt.CostEvents = []EventRef{spawn}
			obs.Attempt.InputTokenEvents = []EventRef{spawn}
			obs.Attempt.OutputTokenEvents = []EventRef{spawn}
			obs.Attempt.CorrectionEvents = []EventRef{spawn}
			obs.Attempt.Corrections = 1
			obs.Attempt.Evidence.Cost = journalCite(spawn)
			obs.Attempt.Evidence.InputTokens = journalCite(spawn)
			obs.Attempt.Evidence.OutputTokens = journalCite(spawn)
			obs.Attempt.Evidence.Corrections = journalCite(spawn)
		})
		got := art.Evidence.Observations[0].Attempt
		if got.CostEvents[0] != got.CorrectionEvents[0] || got.CostEvents[0] != got.InputTokenEvents[0] {
			t.Fatal("cross-list control lost the shared spawn identity")
		}
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("spawn-after-terminal-before-cutoff", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			later := obs.Attempt.TerminalAt.Add(time.Minute)
			if !later.Before(obs.Attempt.Cutoff) && !later.Equal(obs.Attempt.Cutoff) {
				t.Fatal("fixture later spawn is not at or before cutoff")
			}
			obs.Attempt.CostEvents[0].At = later
			obs.Attempt.Evidence.Cost.Event.At = later
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("spawn-at-exact-cutoff", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CostEvents[0].At = obs.Attempt.Cutoff
			obs.Attempt.Evidence.Cost.Event.At = obs.Attempt.Cutoff
		})
		requireEligibleCompleted(t, art, 2)
	})

	t.Run("local-build-eligibility-measurement", func(t *testing.T) {
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
		if result == nil || result.Artifact.Evidence == nil || len(result.Artifact.Evidence.Observations) == 0 {
			t.Fatal("local measurement Build dropped recovered observations")
		}
		att := result.Artifact.Evidence.Observations[0].Attempt
		if att.Corrections != len(att.CorrectionEvents) || att.Cascades != len(att.CascadeEvents) ||
			att.Reviews != len(att.ReviewEvents) || att.Verifications != len(att.VerificationEvents) {
			t.Fatalf("Build output counts disagree with lists: corrections=%d/%d cascades=%d/%d reviews=%d/%d verifications=%d/%d",
				att.Corrections, len(att.CorrectionEvents), att.Cascades, len(att.CascadeEvents),
				att.Reviews, len(att.ReviewEvents), att.Verifications, len(att.VerificationEvents))
		}
		if att.CostScope != CostScopeRecordedSpawns {
			t.Fatalf("Build cost scope = %q", att.CostScope)
		}
		if att.CostUSD.Known {
			if len(att.CostEvents) == 0 || att.Evidence.Cost.Source != EvidenceJournal || att.Evidence.Cost.Event != att.CostEvents[0] {
				t.Fatal("known Build cost lacks least-event EvidenceJournal citation")
			}
		}
		if _, statErr := os.Stat(filepath.Join(runs, "run-offset", "journal.jsonl")); statErr != nil {
			t.Fatalf("expected local journal missing: %v", statErr)
		}
		got, eligErr := PredictionEligibility(result.Artifact, eligibleTargets(), wantCompleted, true)
		if eligErr != nil {
			t.Fatal(eligErr)
		}
		if !got.Eligible || got.Cells[0].Completed != wantCompleted {
			t.Fatalf("local measurement eligibility = %+v", got)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*RecoveredAttempt)
	}{
		{name: "corrections-nonzero-without-list", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.Corrections = 99
			obs.Attempt.CorrectionEvents = nil
			obs.Attempt.Evidence.Corrections = FieldEvidence{}
		}},
		{name: "cascades-nonzero-without-list", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.Cascades = 99
			obs.Attempt.CascadeEvents = nil
			obs.Attempt.Evidence.Cascades = FieldEvidence{}
		}},
		{name: "reviews-nonzero-without-list", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.Reviews = 99
			obs.Attempt.ReviewEvents = nil
			obs.Attempt.Evidence.Reviews = FieldEvidence{}
		}},
		{name: "verifications-nonzero-without-list", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.Verifications = 99
			obs.Attempt.VerificationEvents = nil
			obs.Attempt.Evidence.Verifications = FieldEvidence{}
		}},
		{name: "corrections-count-exceeds-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Corrections = 3
		}},
		{name: "corrections-count-below-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Corrections = 1
		}},
		{name: "cascades-count-exceeds-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Cascades = 4
		}},
		{name: "missing-least-correction-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Evidence.Corrections = FieldEvidence{}
		}},
		{name: "non-least-correction-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Evidence.Corrections = journalCite(obs.Attempt.CorrectionEvents[1])
		}},
		{name: "mismatched-correction-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Evidence.Corrections = journalCite(obs.Attempt.Start)
		}},
		{name: "missing-least-cost-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Evidence.Cost = FieldEvidence{}
		}},
		{name: "non-least-cost-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			other := typedCitation(obs.Attempt, 20, 21, EventTaskSpawnFinished)
			obs.Attempt.CostEvents = []EventRef{obs.Attempt.CostEvents[0], other}
			obs.Attempt.Evidence.Cost = journalCite(other)
		}},
		{name: "mismatched-cost-citation", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Evidence.Cost = journalCite(obs.Attempt.Start)
		}},
		{name: "known-zero-cost-without-contributors", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.CostUSD = Known(0.0)
			obs.Attempt.CostEvents = nil
			obs.Attempt.Evidence.Cost = FieldEvidence{}
		}},
		{name: "known-positive-cost-without-contributors", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.CostUSD = Known(12345.0)
			obs.Attempt.CostEvents = nil
			obs.Attempt.Evidence.Cost = FieldEvidence{}
		}},
		{name: "known-zero-tokens-without-contributors", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.InputTokens = Known(int64(0))
			obs.Attempt.OutputTokens = Known(int64(0))
			obs.Attempt.InputTokenEvents = nil
			obs.Attempt.OutputTokenEvents = nil
			obs.Attempt.Evidence.InputTokens = FieldEvidence{}
			obs.Attempt.Evidence.OutputTokens = FieldEvidence{}
		}},
		{name: "known-positive-tokens-without-contributors", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.InputTokens = Known(int64(999999))
			obs.Attempt.OutputTokens = Known(int64(999999))
			obs.Attempt.InputTokenEvents = nil
			obs.Attempt.OutputTokenEvents = nil
			obs.Attempt.Evidence.InputTokens = FieldEvidence{}
			obs.Attempt.Evidence.OutputTokens = FieldEvidence{}
		}},
		{name: "duplicate-correction-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			dup := obs.Attempt.CorrectionEvents[0]
			obs.Attempt.CorrectionEvents = []EventRef{dup, dup}
			obs.Attempt.Corrections = 2
			obs.Attempt.Evidence.Corrections = journalCite(dup)
		}},
		{name: "unordered-correction-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			list := obs.Attempt.CorrectionEvents
			obs.Attempt.CorrectionEvents = []EventRef{list[1], list[0]}
			obs.Attempt.Evidence.Corrections = journalCite(list[1])
		}},
		{name: "duplicate-cost-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			spawn := obs.Attempt.CostEvents[0]
			obs.Attempt.CostEvents = []EventRef{spawn, spawn}
			obs.Attempt.Evidence.Cost = journalCite(spawn)
		}},
		{name: "unordered-cost-list", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			first := obs.Attempt.CostEvents[0]
			second := typedCitation(obs.Attempt, 20, 21, EventTaskSpawnFinished)
			obs.Attempt.CostEvents = []EventRef{second, first}
			obs.Attempt.Evidence.Cost = journalCite(second)
		}},
		{name: "wrong-type-cascade", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CascadeEvents[0].Type = EventPanelIterate
			obs.Attempt.Evidence.Cascades.Event.Type = EventPanelIterate
		}},
		{name: "wrong-type-review", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.ReviewEvents[0].Type = EventPanelIterate
			obs.Attempt.Evidence.Reviews.Event.Type = EventPanelIterate
		}},
		{name: "wrong-type-verification", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.VerificationEvents[0].Type = EventVerificationIterate
			obs.Attempt.Evidence.Verifications.Event.Type = EventVerificationIterate
		}},
		{name: "wrong-type-correction", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CorrectionEvents[0].Type = EventPanelStarted
			obs.Attempt.Evidence.Corrections.Event.Type = EventPanelStarted
		}},
		{name: "wrong-type-cost", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CostEvents[0].Type = EventTaskStarted
			obs.Attempt.Evidence.Cost.Event.Type = EventTaskStarted
		}},
		{name: "wrong-type-tokens", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.InputTokenEvents[0].Type = EventAgentFallback
			obs.Attempt.Evidence.InputTokens.Event.Type = EventAgentFallback
		}},
		{name: "nonpositive-line", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CostEvents[0].Line = 0
			obs.Attempt.Evidence.Cost.Event.Line = 0
		}},
		{name: "negative-line", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CorrectionEvents[0].Line = -1
			obs.Attempt.Evidence.Corrections.Event.Line = -1
		}},
		{name: "future-timestamp", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			future := obs.Attempt.Cutoff.Add(time.Minute)
			obs.Attempt.CostEvents[0].At = future
			obs.Attempt.Evidence.Cost.Event.At = future
		}},
		{name: "absent-timestamp", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CostEvents[0].At = time.Time{}
			obs.Attempt.Evidence.Cost.Event.At = time.Time{}
		}},
		{name: "wrong-cost-scope-known", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.CostScope = "total_process_cost"
		}},
		{name: "wrong-cost-scope-unknown", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.CostScope = "total_process_cost"
		}},
		{name: "empty-cost-scope", mutate: func(obs *RecoveredAttempt) {
			obs.Attempt.CostScope = ""
		}},
		{name: "zero-count-with-journal-evidence", mutate: func(obs *RecoveredAttempt) {
			attachCompletePositiveMeasurements(obs)
			obs.Attempt.Corrections = 0
			obs.Attempt.CorrectionEvents = []EventRef{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefuseInvalid(t, mutateObservation(recoveredArtifact(2), tc.mutate))
		})
	}

	// Physical identity is selected JournalIdentity plus positive Line, independent
	// of At/HasSeq/Seq/Type/Hash/PrevHash. Existing duplicate-* cases stay
	// byte-identical and are not rewritten. Distinct lines and cross-list reuse
	// remain legal; there is no global dedup or clock-within-terminal rule.
	t.Run("parent-1ns-two-cascade-probe", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 4, 5, EventAgentFallback)
			second := first
			second.At = first.At.Add(time.Nanosecond)
			obs.Attempt.Cascades = 2
			obs.Attempt.CascadeEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Cascades = journalCite(first)
			refs = obs.Attempt.CascadeEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "cascades", 5)
	})
	t.Run("corrections-same-line-allowed-type", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 6, 7, EventPanelIterate)
			second := first
			second.Type = EventVerificationIterate
			obs.Attempt.Corrections = 2
			obs.Attempt.CorrectionEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Corrections = journalCite(first)
			refs = obs.Attempt.CorrectionEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "corrections", 7)
	})
	t.Run("reviews-same-line-seq-metadata", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 8, 9, EventPanelStarted)
			second := first
			second.Seq = first.Seq + 1
			obs.Attempt.Reviews = 2
			obs.Attempt.ReviewEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Reviews = journalCite(first)
			refs = obs.Attempt.ReviewEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "reviews", 9)
	})
	t.Run("verifications-same-line-hash", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 9, 10, EventVerificationStarted)
			second := first
			second.Hash = "bbbb"
			obs.Attempt.Verifications = 2
			obs.Attempt.VerificationEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Verifications = journalCite(first)
			refs = obs.Attempt.VerificationEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "verifications", 10)
	})
	t.Run("cost-same-line-timestamp", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 4, 5, EventTaskSpawnFinished)
			second := first
			second.At = first.At.Add(time.Nanosecond)
			obs.Attempt.CostUSD = Known(1.5)
			obs.Attempt.CostEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Cost = journalCite(first)
			refs = obs.Attempt.CostEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "cost", 5)
	})
	t.Run("input-tokens-same-line-seq", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 5, 6, EventTaskSpawnFinished)
			second := first
			second.Seq = first.Seq + 1
			obs.Attempt.InputTokens = Known(int64(3))
			obs.Attempt.InputTokenEvents = []EventRef{first, second}
			obs.Attempt.Evidence.InputTokens = journalCite(first)
			refs = obs.Attempt.InputTokenEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "input_tokens", 6)
	})
	t.Run("output-tokens-same-line-prevhash", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 5, 6, EventTaskSpawnFinished)
			second := first
			second.PrevHash = "aaaa"
			obs.Attempt.OutputTokens = Known(int64(4))
			obs.Attempt.OutputTokenEvents = []EventRef{first, second}
			obs.Attempt.Evidence.OutputTokens = journalCite(first)
			refs = obs.Attempt.OutputTokenEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "output_tokens", 6)
	})
	t.Run("unknown-partial-cost-same-line", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 4, 5, EventTaskSpawnFinished)
			second := first
			second.At = first.At.Add(time.Nanosecond)
			obs.Attempt.CostUSD = Unknown[float64]()
			obs.Attempt.CostEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Cost = FieldEvidence{}
			refs = obs.Attempt.CostEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "cost", 5)
	})
	t.Run("nonadjacent-same-line-corrections", func(t *testing.T) {
		var refs []EventRef
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 6, 7, EventPanelIterate)
			middle := typedCitation(obs.Attempt, 7, 8, EventVerificationIterate)
			third := typedCitation(obs.Attempt, 8, 7, EventTaskSpawnFinished)
			obs.Attempt.Corrections = 3
			obs.Attempt.CorrectionEvents = []EventRef{first, middle, third}
			obs.Attempt.Evidence.Corrections = journalCite(first)
			refs = obs.Attempt.CorrectionEvents
		})
		requireRepeatedPhysicalLineRefused(t, art, refs, "corrections", 7)
	})
	t.Run("distinct-physical-lines-remain-eligible", func(t *testing.T) {
		art := mutateObservation(recoveredArtifact(2), func(obs *RecoveredAttempt) {
			first := typedCitation(obs.Attempt, 4, 5, EventAgentFallback)
			second := typedCitation(obs.Attempt, 5, 6, EventAgentFallback)
			second.At = first.At.Add(time.Nanosecond)
			obs.Attempt.Cascades = 2
			obs.Attempt.CascadeEvents = []EventRef{first, second}
			obs.Attempt.Evidence.Cascades = journalCite(first)
		})
		got := art.Evidence.Observations[0].Attempt.CascadeEvents
		if len(got) != 2 || got[0].Line == got[1].Line {
			t.Fatal("distinct-line control lost two different physical lines")
		}
		requireEligibleCompleted(t, art, 2)
	})
}

func requireStrictPhysicalDuplicateFixture(t *testing.T, refs []EventRef, line int) {
	t.Helper()
	if len(refs) < 2 {
		t.Fatal("physical-duplicate fixture needs at least two refs")
	}
	journal := refs[0].Journal
	matches := 0
	for i, ref := range refs {
		if ref.Journal != journal {
			t.Fatal("physical-duplicate fixture changed JournalIdentity; identity is selected journal plus line")
		}
		if ref.Line <= 0 {
			t.Fatal("physical-duplicate fixture used a nonpositive line")
		}
		if i > 0 {
			if refs[i-1] == ref {
				t.Fatal("fixture is byte-identical; existing duplicate-* tests already cover that shape")
			}
			if !eventRefLess(refs[i-1], ref) {
				t.Fatalf("fixture is not strictly comparator-ordered at index %d", i)
			}
		}
		if ref.Line == line {
			matches++
		}
	}
	if matches < 2 {
		t.Fatalf("fixture does not repeat physical line %d", line)
	}
}

func requireRepeatedPhysicalLineRefused(t *testing.T, art Artifact, refs []EventRef, field string, line int) {
	t.Helper()
	requireStrictPhysicalDuplicateFixture(t, refs, line)
	got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if got.Eligible {
		t.Fatal("invalid payload marked eligible")
	}
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("invalid payload refuse = %v, want ErrNotEligible and ErrSourceIncomplete", err)
	}
	blob := strings.ToLower(strings.Join(got.Reasons, "\n"))
	if !strings.Contains(blob, strings.ToLower(field)) {
		t.Fatalf("reasons %q do not name field %q", got.Reasons, field)
	}
	if !strings.Contains(blob, strconv.Itoa(line)) {
		t.Fatalf("reasons %q do not name physical line %d", got.Reasons, line)
	}
}

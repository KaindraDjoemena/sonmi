package agent

import (
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"sonmi/internal/api"
	"sonmi/internal/config"
	"sonmi/internal/db"
)

// CORRECTION LOOP
// 1. generate correction context for agent
// 2. if (1) passes, we prompt the agent,   otherwise enter failsafe mode: [enterFailsafeMode]
// 3. if (2) passes, we execute correction, otherwise enter failsafe mode: [enterFailsafeMode]
func StartCorrectionLoop(database db.Database, c api.DeviceController, cfg *config.Config, status *api.LoopStatus) {
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	for {
		<-ticker.C

		slog.Info("Executing Correction Loop...")
		status.MarkCorrectionAttempt()

		ctx, err := newCorrectionContext(database, cfg) // stale journals get handled here
		if err != nil {
			tryFallbackOrFailsafe(database, c, cfg, ctx, "Couldnt generate Correction Context for Agent")
			slog.Error(err.Error())
			continue
		}

		respBytes, err := promptAgent(ctx, cfg)
		if err != nil {
			tryFallbackOrFailsafe(database, c, cfg, ctx, "Couldnt get Agents Correction response")
			slog.Error(err.Error())
			continue
		}

		err = executeCorrections(respBytes, c, database, cfg, ctx)
		if err != nil {
			if isBudgetOnlyError(err) {
				// Budget exhausted is an expected daily condition, not a system failure.
				// Other corrections in the batch already applied; only watering was skipped.
				slog.Warn("Watering budget exhausted for today — skipping water correction")
			} else {
				tryFallbackOrFailsafe(database, c, cfg, ctx, "Couldnt execute Agents Correction response")
				slog.Error(err.Error())
				continue
			}
		}

		status.MarkCorrectionSuccess()

		// Don't stamp NOMINAL over a JOURNAL_DEGRADED state that newCorrectionContext
		// just inserted this same tick — that condition is still true regardless of
		// whether the correction itself executed successfully. newCorrectionContext
		// clears JOURNAL_DEGRADED itself once a fresh journal reappears.
		if !ctx.JournalDegraded {
			currentSysState, err := database.SelectCurrentSystemState()
			if errors.Is(err, sql.ErrNoRows) || (err == nil && currentSysState.State != db.StateNominal) {
				sysRow := db.SystemStateRow{
					State: db.StateNominal,
					Time:  time.Now().UTC(),
				}

				sysRow.Insert(database)
			}
		}
	}
}

// DAILY JOURNAL LOOP:
// 1. mark previous journals as stale
// 2. generate journal context for agent
// 3. if (2) passes, we prompt the agent,         otherwise fail silently and dont make a journal entry for the day
// 4. if (3) passes, we create the journal entry, otherwise fail silently and dont make a journal entry for the day
//
// Fires at a fixed wall-clock time (23:59) rather than a ticker started from
// process boot — a boot-relative 24h ticker drifts to whatever time-of-day the
// container last restarted, which can leave "today" without any journal entry
// whose valid_for_date matches today for most of the day (see JournalDegraded).
func StartJournalLoop(database db.Database, cfg *config.Config, status *api.LoopStatus) {
	for {
		<-time.After(durationUntilNext(23, 59))

		slog.Info("Executing Journal Loop...")
		status.MarkJournalAttempt()

		if err := generateJournal(database, cfg); err != nil {
			slog.Error("Journal generation failed, scheduling retry", "error", err)
			database.InsertRetryJob(time.Now().UTC().Add(30 * time.Minute))
		} else {
			status.MarkJournalSuccess()
		}
	}
}

// RETRY WORKER BACKGROUND WORKER
func StartRetryWorker(database db.Database, cfg *config.Config, status *api.LoopStatus) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C

		job, err := database.ConsumeReadyRetryJob()
		if err != nil {
			continue
		}

		slog.Info("Executing Retry Journal Loop...", "attempt", job.AttemptCount)
		status.MarkJournalAttempt()

		if err := generateJournal(database, cfg); err != nil {
			slog.Error("Retry failed again", "error", err)

			var nextRetry time.Time
			switch job.AttemptCount {
			case 0:
				nextRetry = time.Now().UTC().Add(60 * time.Minute)
			case 1:
				nextRetry = time.Now().UTC().Add(120 * time.Minute)
			case 2:
				nextRetry = time.Now().UTC().Add(180 * time.Minute)
			default:
				slog.Error("Journal retry exhausted. Skipping the day.")
				continue
			}

			database.RequeueRetryJob(*job, nextRetry)
		} else {
			slog.Info("Journal Retry succeeded!")
			status.MarkJournalSuccess()
		}
	}
}

// durationUntilNext returns how long to wait until the next occurrence of
// hour:minute in the local clock — today if that time hasn't passed yet,
// otherwise tomorrow. Used to keep the journal loop pinned to a fixed
// wall-clock time instead of drifting with container restarts.
func durationUntilNext(hour, minute int) time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return time.Until(next)
}

// isBudgetOnlyError reports whether every error in a (possibly joined) error
// value is a db.ErrBudgetExceeded. Used to distinguish "watering skipped today"
// (safe, expected) from a real execution failure that warrants FAILSAFE.
func isBudgetOnlyError(err error) bool {
	if err == nil {
		return false
	}
	type joinedErr interface {
		Unwrap() []error
	}
	if joined, ok := err.(joinedErr); ok {
		for _, e := range joined.Unwrap() {
			if e != nil && !errors.Is(e, db.ErrBudgetExceeded) {
				return false
			}
		}
		return true
	}
	return errors.Is(err, db.ErrBudgetExceeded)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func tryFallbackOrFailsafe(database db.Database, c api.DeviceController, cfg *config.Config, ctx *correctionContextWindow, rationale string) {
	if ctx != nil && ctx.SpecialInstructions != "" {
		sysRow := db.SystemStateRow{
			State: db.StateCorrectionDegraded,
			Time:  time.Now().UTC(),
		}

		sysRow.Insert(database)

		if err := executeCorrections([]byte(ctx.SpecialInstructions), c, database, cfg, ctx); err == nil {
			return
		}
	}
	enterFailsafeMode(database, c, cfg, rationale)
}

func enterFailsafeMode(database db.Database, c api.DeviceController, cfg *config.Config, rationale string) {
	slog.Error("Entering Failsafe Mode", "reason", rationale)

	sysRow := db.SystemStateRow{State: db.StateFailsafe, Time: time.Now().UTC()}
	sysRow.Insert(database)

	waterOK := true
	if telemetry, err := database.SelectPastNHourTelemetryRows(1); err == nil && len(telemetry) > 0 {
		waterOK = telemetry[0].SoilHumidity <= cfg.Ecosystem.IdealConditions.MaxSoilMoisturePercent
	}

	if waterOK {
		if err := database.DecrementWateringBudget(time.Now().UTC().Format(time.DateOnly), cfg.FailsafeDefaults.MaxWateringEventsPerDay); err == nil {
			c.ToggleWaterPump(cfg.FailsafeDefaults.WaterPumpDurationS, db.ModeFailsafe, rationale)
		}
	}

	c.ToggleGrowLight(cfg.FailsafeDefaults.GrowLightOn, db.ModeFailsafe, rationale)
	c.ToggleIntakeFan(cfg.FailsafeDefaults.IntakeFanOn, db.ModeFailsafe, rationale)
	c.ToggleExhaustFan(cfg.FailsafeDefaults.ExhaustFanOn, db.ModeFailsafe, rationale)
}

func generateJournal(database db.Database, cfg *config.Config) error {
	if err := database.MarkAllJournalsStale(); err != nil {
		return err
	}

	ctx, err := newJournalContext(database, cfg)
	if err != nil {
		return err
	}

	respBytes, err := promptAgent(ctx, cfg)
	if err != nil {
		return err
	}

	return executeJournal(respBytes, database)
}

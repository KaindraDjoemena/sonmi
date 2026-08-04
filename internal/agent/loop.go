package agent

import (
	"log/slog"
	"time"

	"sonmi/internal/api"
	"sonmi/internal/config"
	"sonmi/internal/db"
)

// HOURLY CORRECTION LOOP
// 1. generate correction context for agent
// 2. if (1) passes, we prompt the agent,   otherwise enter failsafe mode: [enterFailsafeMode]
// 3. if (2) passes, we execute correction, otherwise enter failsafe mode: [enterFailsafeMode]
func StartCorrectionLoop(database db.Database, c api.DeviceController, cfg *config.Config) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		<-ticker.C

		slog.Info("Executing Correction Loop...")

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
			tryFallbackOrFailsafe(database, c, cfg, ctx, "Couldnt execute Agents Correction response")
			slog.Error(err.Error())
			continue
		}

		currentSysState, err := database.SelectCurrentSystemState()
		if err == nil && currentSysState.State != db.StateNominal {
			sysRow := db.SystemStateRow{
				State: db.StateNominal,
				Time:  time.Now(),
			}

			sysRow.Insert(database)
		}
	}
}

// DAILY JOURNAL LOOP:
// 1. mark previous journals as stale
// 2. generate journal context for agent
// 3. if (2) passes, we prompt the agent,         otherwise fail silently and dont make a journal entry for the day
// 4. if (3) passes, we create the journal entry, otherwise fail silently and dont make a journal entry for the day
func StartJournalLoop(database db.Database, cfg *config.Config) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		<-ticker.C

		slog.Info("Executing Journal Loop...")

		if err := generateJournal(database, cfg); err != nil {
			slog.Error("Journal generation failed, scheduling retry", "error", err)
			database.InsertRetryJob(time.Now().Add(30 * time.Minute))
		}
	}
}

// RETRY WORKER BACKGROUND WORKER
func StartRetryWorker(database db.Database, cfg *config.Config) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C

		job, err := database.ConsumeReadyRetryJob()
		if err != nil {
			continue
		}

		slog.Info("Executing Retry Journal Loop...", "attempt", job.AttemptCount)

		if err := generateJournal(database, cfg); err != nil {
			slog.Error("Retry failed again", "error", err)

			var nextRetry time.Time
			switch job.AttemptCount {
			case 0:
				nextRetry = time.Now().Add(60 * time.Minute)
			case 1:
				nextRetry = time.Now().Add(120 * time.Minute)
			case 2:
				nextRetry = time.Now().Add(180 * time.Minute)
			default:
				slog.Error("Journal retry exhausted. Skipping the day.")
				continue
			}

			database.RequeueRetryJob(*job, nextRetry)
		} else {
			slog.Info("Journal Retry succeeded!")
		}
	}
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func tryFallbackOrFailsafe(database db.Database, c api.DeviceController, cfg *config.Config, ctx *correctionContextWindow, rationale string) {
	if ctx != nil && ctx.SpecialInstructions != "" {
		sysRow := db.SystemStateRow{
			State: db.StateCorrectionDegraded,
			Time:  time.Now(),
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

	sysRow := db.SystemStateRow{State: db.StateFailsafe, Time: time.Now()}
	sysRow.Insert(database)

	c.ToggleWaterPump(cfg.FailsafeDefaults.WaterPumpDurationS, db.ModeFailsafe, rationale)
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

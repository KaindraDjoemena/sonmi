package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"sonmi/internal/api"
	"sonmi/internal/config"
	"sonmi/internal/db"
)

// The schema the AI agent uses to describe a relay action for correction.
// as defined in [correctionContextWindow.getSchema]
type correctionResponse struct {
	Relay     string `json:"relay"`
	Value     bool   `json:"value"`
	Duration  uint   `json:"duration"` // used only for RelayWaterPump; seconds to run
	Rationale string `json:"rationale"`
}

type relayAction func(correction correctionResponse, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error

var actionMap = map[string]relayAction{
	string(db.RelayWaterPump): func(correction correctionResponse, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error {
		if !correction.Value {
			return c.ToggleWaterPump(0, db.ModeAgent, correction.Rationale)
		}
		if len(ctx.LatestTelemetryLog) > 0 {
			if ctx.LatestTelemetryLog[0].SoilHumidity > cfg.Ecosystem.IdealConditions.MaxSoilMoisturePercent {
				return nil
			}
		}
		if err := database.DecrementWateringBudget(time.Now().UTC().Format(time.DateOnly), cfg.FailsafeDefaults.MaxWateringEventsPerDay); err != nil {
			return err
		}
		return c.ToggleWaterPump(correction.Duration, db.ModeAgent, correction.Rationale)
	},
	string(db.RelayGrowLight): func(correction correctionResponse, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error {
		return c.ToggleGrowLight(correction.Value, db.ModeAgent, correction.Rationale)
	},
	string(db.RelayIntakeFan): func(correction correctionResponse, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error {
		return c.ToggleIntakeFan(correction.Value, db.ModeAgent, correction.Rationale)
	},
	string(db.RelayExhaustFan): func(correction correctionResponse, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error {
		return c.ToggleExhaustFan(correction.Value, db.ModeAgent, correction.Rationale)
	},
}

func executeCorrections(resp []byte, c api.DeviceController, database db.Database, cfg *config.Config, ctx *correctionContextWindow) error {
	var corrections []correctionResponse
	if err := json.Unmarshal(resp, &corrections); err != nil {
		return err
	}

	if len(corrections) == 0 {
		return nil
	}

	// NOTE .Relay is guarateed to be of type [db.Relay_t]
	var errs []error
	for _, correction := range corrections {
		action, exists := actionMap[correction.Relay]
		if !exists {
			continue
		}

		if err := action(correction, c, database, cfg, ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// The schema the AI agent uses to describe a journal.
// as defined in [journalContextWindow.getSchema]
type journalResponse struct {
	DayRecap         string `json:"day_recap"`
	PlanForTomorrow  string `json:"plan_for_tomorrow"`
	SafeDefaultsJSON string `json:"safe_defaults_json"`
	AgentMusings     string `json:"agent_musings"`
}

func executeJournal(resp []byte, database db.Database) error {
	var journalResponse journalResponse
	if err := json.Unmarshal(resp, &journalResponse); err != nil {
		return err
	}

	// Look up today's photo URL from the daily_photos table.
	// If not found yet (ticker hasn't fired, or S3 not configured), leave ImgUrl empty.
	imgUrl := ""
	photoRow, err := database.SelectDailyPhoto(time.Now().UTC().Format(time.DateOnly))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("Warning: failed to look up today's daily photo: %v", err)
	} else if err == nil {
		imgUrl = photoRow.ImgUrl
	}

	journalEntry := db.JournalEntryRow{
		DayRecap:         journalResponse.DayRecap,
		PlanForTomorrow:  journalResponse.PlanForTomorrow,
		SafeDefaultsJSON: journalResponse.SafeDefaultsJSON,
		AgentMusings:     journalResponse.AgentMusings,
		IsStale:          false,
		ValidForDate:     time.Now().UTC().Add(24 * time.Hour).Format(time.DateOnly),
		ImgUrl:           imgUrl,
		Time:             time.Now().UTC(),
	}

	if err := journalEntry.Insert(database); err != nil {
		return err
	}

	go api.FireRevalidateWebhook()

	return nil
}

package agent

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	"google.golang.org/genai"

	"sonmi/internal/config"
	"sonmi/internal/db"
)

type Task_t string

const (
	JOURNAL       Task_t = "JOURNAL"
	CORRECTION    Task_t = "CORRECTION"
	WAIT_DURATION        = 30 * time.Second
)

type compilablePrompt interface {
	getSchema() *genai.Schema
	compilePrompt() (string, error)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
type journalContextWindow struct {
	SysPrompt            string                  // templated sys prompt
	BotanicalProfile     string                  // plant profile
	IdealConditions      config.IdealConditions  // ideal conditions for the plant to thrive at each stage
	FullDayRelayEventLog []db.RelayEventRow      // past 24 hour relay events
	FullDaySysLog        []db.SystemStateRow     // past 24 hour system changes
	FullDayTelemetryLog  []db.SensorTelemetryRow // past 24 hour telemetry logs
	PastJournals         []db.JournalEntryRow    // t(-1) and t(-2) entries
	ImgYesterday         string                  // t(-1) plant image
	ImgToday             string                  // t(0) plant image
}

func newJournalContext(d db.Database, cfg *config.Config) (*journalContextWindow, error) {
	relayEventLogs, err := d.SelectPastNHourRelayEventRows(24)
	if err != nil {
		return nil, err
	}

	sysLogs, err := d.SelectPastNHourSystemRows(24)
	if err != nil {
		return nil, err
	}

	telemetryLogs, err := d.SelectPastNHourTelemetryRows(24)
	if err != nil {
		return nil, err
	}

	pastJournals, err := d.SelectPastNDayJournalEntryRows(2)
	if err != nil {
		return nil, err
	}

	today := time.Now().Format(time.DateOnly)
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)

	imgToday := ""
	if row, err := d.SelectDailyPhoto(today); err == nil {
		imgToday = row.ImgUrl
	}

	imgYesterday := ""
	if row, err := d.SelectDailyPhoto(yesterday); err == nil {
		imgYesterday = row.ImgUrl
	}

	return &journalContextWindow{
		SysPrompt:            cfg.Ecosystem.JournalSysPrompt,
		BotanicalProfile:     cfg.Ecosystem.BotanicalProfile,
		IdealConditions:      cfg.Ecosystem.IdealConditions,
		FullDayRelayEventLog: relayEventLogs,
		FullDaySysLog:        sysLogs,
		FullDayTelemetryLog:  telemetryLogs,
		PastJournals:         pastJournals,
		ImgYesterday:         imgYesterday,
		ImgToday:             imgToday,
	}, nil
}

func (j *journalContextWindow) getSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"day_recap": {
				Type:        genai.TypeString,
				Description: "A detailed recap of the plant's health and events over the past 24 hours.",
			},
			"plan_for_tomorrow": {
				Type:        genai.TypeString,
				Description: "The journal plan for the next 24 hours.",
			},
			"safe_defaults_json": {
				Type:        genai.TypeString,
				Description: "JSON string containing safe override defaults if the agent crashes.",
			},
			"agent_musings": {
				Type:        genai.TypeString,
				Description: "A creative, philosophical, or witty thought on the project, the hardware, or its existence as an AI botanist.",
			},
		},
	}
}

//go:embed sys_prompts/journal.tmpl
var journalPromptTemplate string

func (j *journalContextWindow) compilePrompt() (string, error) {
	tmpl, err := template.New("journal").Parse(journalPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("Error parsing journal template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, j); err != nil {
		return "", fmt.Errorf("Error executing journal template: %v", err)
	}

	return buf.String(), nil
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
type correctionContextWindow struct {
	SysPrompt              string                  // templated sys prompt
	BotanicalProfile       string                  // plant profile
	IdealConditions        config.IdealConditions  // ideal conditions for the plant to thrive at each stage
	TodaysStrategy         string                  // yesterdays proposed strategy
	SpecialInstructions    string                  // yesterdays special instructions
	LatestSysLog           db.SystemStateRow       // current system state
	LatestTelemetryLog     []db.SensorTelemetryRow // past hour telemetry logs
	LatestRelayLog         []db.RelayEventRow      // past 24 hour relay events
	WateringBudgetRemaining int                    // remaining waterings allowed today (0 = exhausted)
}

func newCorrectionContext(d db.Database, cfg *config.Config) (*correctionContextWindow, error) {
	sysLogs, err := d.SelectPastNHourSystemRows(1)
	if err != nil {
		return nil, err
	}
	var latestSysLog db.SystemStateRow

	if len(sysLogs) > 0 {
		latestSysLog = sysLogs[0]
	}

	pastJournals, err := d.SelectPastNDayJournalEntryRows(1)
	if err != nil {
		return nil, err
	}
	var todaysStrat, specialInstr string

	// stale journal handling
	if len(pastJournals) > 0 {
		if pastJournals[0].IsStale || pastJournals[0].ValidForDate != time.Now().Format(time.DateOnly) {

			// stale journal
			db.SystemStateRow{State: db.StateJournalDegraded, Time: time.Now()}.Insert(d)
		} else {

			// fresh journal
			todaysStrat = pastJournals[0].PlanForTomorrow
			specialInstr = pastJournals[0].SafeDefaultsJSON

			// if the system were previously degraded, log that we are nominal again
			if latestSysLog.State == db.StateJournalDegraded {
				db.SystemStateRow{State: db.StateNominal, Time: time.Now()}.Insert(d)
			}
		}
	}

	telemetryLogs, err := d.SelectPastNHourTelemetryRows(1)
	if err != nil {
		return nil, err
	}

	relayLogs, err := d.SelectPastNHourRelayEventRows(24)
	if err != nil {
		return nil, err
	}

	// Read remaining watering budget. sql.ErrNoRows means no watering has
	// happened yet today — treat as full budget available.
	wateringBudget := int(cfg.FailsafeDefaults.MaxWateringEventsPerDay)
	if remaining, err := d.SelectWateringBudget(time.Now().Format(time.DateOnly)); err == nil {
		wateringBudget = remaining
	}

	return &correctionContextWindow{
		SysPrompt:               cfg.Ecosystem.CorrectionSysPrompt,
		BotanicalProfile:        cfg.Ecosystem.BotanicalProfile,
		IdealConditions:         cfg.Ecosystem.IdealConditions,
		TodaysStrategy:          todaysStrat,
		SpecialInstructions:     specialInstr,
		LatestSysLog:            latestSysLog,
		LatestTelemetryLog:      telemetryLogs,
		LatestRelayLog:          relayLogs,
		WateringBudgetRemaining: wateringBudget,
	}, nil
}

func (a *correctionContextWindow) getSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeArray,
		Items: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"relay": {
					Type:        genai.TypeString,
					Description: "Must be exactly one of: WATER_PUMP, GROW_LIGHT, INTAKE_FAN, EXHAUST_FAN",
				},
				"value": {
					Type:        genai.TypeBoolean,
					Description: "true to turn the relay on, false to turn it off",
				},
				"duration": {
					Type:        genai.TypeInteger,
					Description: "Duration in seconds to run the relay. ONLY applicable when relay is WATER_PUMP (0 to turn off, >0 to run for duration)",
				},
				"rationale": {
					Type:        genai.TypeString,
					Description: "A short, 1-sentence scientific rationale for this correction",
				},
			},
		},
	}
}

//go:embed sys_prompts/correction.tmpl
var correctionPromptTemplate string

func (a *correctionContextWindow) compilePrompt() (string, error) {
	tmpl, err := template.New("correction").Parse(correctionPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("Error parsing Correction template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, a); err != nil {
		return "", fmt.Errorf("Error executing Correction template: %v", err)
	}

	return buf.String(), nil
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
func promptAgent(p compilablePrompt, cfg *config.Config, extraParts ...*genai.Part) ([]byte, error) {
	ctx := context.Background()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("'GEMINI_API_KEY' env variable is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("Error creating client: %v", err)
	}

	modelName := cfg.Agent.Model
	modelTemp := float32(cfg.Agent.Temp)
	log.Printf("Attempting to prompt %s\n", modelName)

	var resp *genai.GenerateContentResponse

	// Get Structured Ouput
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema:   p.getSchema(),
		Temperature:      &modelTemp,
	}

	// Compile Prompt
	prompt, err := p.compilePrompt()
	if err != nil {
		return nil, fmt.Errorf("Failed to compile prompt: %v", err)
	}

	parts := make([]*genai.Part, 0, len(extraParts)+1)
	parts = append(parts, extraParts...)
	parts = append(parts, genai.NewPartFromText(prompt))

	contents := []*genai.Content{genai.NewContentFromParts(parts, "user")}

	const maxRetries = 5
	for i := range maxRetries {
		resp, err = client.Models.GenerateContent(ctx, modelName, contents, config)
		if err == nil {
			break
		}

		log.Printf("Attempt %d failed: %v\n", i+1, err)
		if i < maxRetries-1 {
			log.Println("Waiting 30 seconds before retrying...")
			time.Sleep(WAIT_DURATION)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("Failed after %d attempts. Last error: %v", maxRetries, err)
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		part := resp.Candidates[0].Content.Parts[0]
		return []byte(part.Text), nil
	}

	return nil, fmt.Errorf("Gemini returned an empty response")
}

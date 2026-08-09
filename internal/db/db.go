package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// /////////////////////////////////////////////////////////////////////////////////////////// TYPES & CONSTS
type Table_t string

const (
	TableSensorTelemetries Table_t = "sensor_telemetries"
	TableRelayEvents       Table_t = "relay_events"
	TableSystemStates      Table_t = "system_states"
	TableJournalEntries    Table_t = "journal_entries"
	TableRetryQueue        Table_t = "retry_queue"
	TableWateringBudgets   Table_t = "watering_budgets"
	TableDailyPhotos       Table_t = "daily_photos"
)

type Sensor_t string
type Relay_t string
type State_t string
type Mode_t string

const (
	SensorTemp         Sensor_t = "TEMP"
	SensorAirHumidity  Sensor_t = "AIR_HUMIDITY"
	SensorSoilHumidity Sensor_t = "SOIL_HUMIDITY"

	RelayWaterPump  Relay_t = "WATER_PUMP"
	RelayGrowLight  Relay_t = "GROW_LIGHT"
	RelayIntakeFan  Relay_t = "INTAKE_FAN"
	RelayExhaustFan Relay_t = "EXHAUST_FAN"

	StateNominal            State_t = "NOMINAL"
	StateCorrectionDegraded State_t = "CORRECTION_DEGRADED"
	StateJournalDegraded    State_t = "JOURNAL_DEGRADED"
	StateFailsafe           State_t = "FAILSAFE"

	ModeAgent    Mode_t = "AGENT"
	ModeOverride Mode_t = "OVERRIDE"
	ModeFailsafe Mode_t = "FAILSAFE"
)

// ErrBudgetExceeded is returned by DecrementWateringBudget when the daily
// watering allowance is already exhausted. It is intentionally exported so
// callers in the agent loop can treat it as a non-fatal, expected condition
// rather than a system failure.
var ErrBudgetExceeded = errors.New("watering budget exceeded")

// /////////////////////////////////////////////////////////////////////////////////////////// DATABASE
type Database struct {
	path string
	conn *sql.DB
}

func NewDatabase(p string) (Database, error) {
	dbConn, err := sql.Open("sqlite", p+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return Database{}, err
	}

	newDB := Database{
		path: p,
		conn: dbConn,
	}
	log.Println("Connecting to Database...")

	if err := newDB.InitializeTables(); err != nil {
		return Database{}, err
	}

	return newDB, nil
}

func (d Database) CloseConn() error {
	err := d.conn.Close()
	if err != nil {
		return err
	}

	log.Println("Database Connection Closed")
	return nil
}

func (d Database) Vacuum(path string) error {
	_, err := d.conn.Exec(fmt.Sprintf("VACUUM INTO '%s'", path))
	return err
}

// /////////////////////////////////////////////////////////////////////////////////////////// QUERIES

// SENSOR TELEMETRY
type SensorTelemetryRow struct {
	Id           int
	Temp         float32
	AirHumidity  float32
	SoilHumidity float32
	Time         time.Time
}

func (r SensorTelemetryRow) Insert(d Database) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s (temp, air_humidity, soil_humidity, time) VALUES (?, ?, ?, ?)`, TableSensorTelemetries)
	if _, err := tx.ExecContext(ctx, query, r.Temp, r.AirHumidity, r.SoilHumidity, FormatTime(r.Time)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d Database) SelectNTelemetryRows(n uint) ([]SensorTelemetryRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`SELECT * FROM %s LIMIT %d`, TableSensorTelemetries, n))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var telemetryRows []SensorTelemetryRow
	var telemetryRow SensorTelemetryRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&telemetryRow.Id, &telemetryRow.Temp, &telemetryRow.AirHumidity, &telemetryRow.SoilHumidity, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		telemetryRows = append(telemetryRows, SensorTelemetryRow{
			Id:           telemetryRow.Id,
			Temp:         telemetryRow.Temp,
			AirHumidity:  telemetryRow.AirHumidity,
			SoilHumidity: telemetryRow.SoilHumidity,
			Time:         timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return telemetryRows, nil
}

func (d Database) SelectPastNHourTelemetryRows(n uint) ([]SensorTelemetryRow, error) {
	cutoff := time.Now().Add(-time.Duration(n) * time.Hour)
	cutoffStr := FormatTime(cutoff)

	query := fmt.Sprintf(`SELECT id, temp, air_humidity, soil_humidity, time FROM %s WHERE time >= ? ORDER BY time DESC`, TableSensorTelemetries)

	rows, err := d.conn.Query(query, cutoffStr)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var telemetryRows []SensorTelemetryRow
	var telemetryRow SensorTelemetryRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&telemetryRow.Id, &telemetryRow.Temp, &telemetryRow.AirHumidity, &telemetryRow.SoilHumidity, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		telemetryRows = append(telemetryRows, SensorTelemetryRow{
			Id:           telemetryRow.Id,
			Temp:         telemetryRow.Temp,
			AirHumidity:  telemetryRow.AirHumidity,
			SoilHumidity: telemetryRow.SoilHumidity,
			Time:         timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return telemetryRows, nil
}

// RELAY EVENT
type RelayEventRow struct {
	Id        int
	Relay     Relay_t
	Mode      Mode_t
	Value     bool
	Rationale string
	Time      time.Time
}

func (r RelayEventRow) Insert(d Database) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s (relay, mode, value, rationale, time) VALUES (?, ?, ?, ?, ?)`, TableRelayEvents)
	if _, err := tx.ExecContext(ctx, query, r.Relay, r.Mode, r.Value, r.Rationale, FormatTime(r.Time)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d Database) SelectNRelayEventRows(n uint) ([]RelayEventRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`SELECT * FROM %s LIMIT %d`, TableRelayEvents, n))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var relayEventRows []RelayEventRow
	var relayEventRow RelayEventRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&relayEventRow.Id, &relayEventRow.Relay, &relayEventRow.Mode, &relayEventRow.Value, &relayEventRow.Rationale, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		relayEventRows = append(relayEventRows, RelayEventRow{
			Id:        relayEventRow.Id,
			Relay:     relayEventRow.Relay,
			Mode:      relayEventRow.Mode,
			Value:     relayEventRow.Value,
			Rationale: relayEventRow.Rationale,
			Time:      timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return relayEventRows, nil
}

func (d Database) SelectAllRelayEventRows() ([]RelayEventRow, error) {
	rows, err := d.conn.Query(fmt.Sprintf(`SELECT * FROM %s`, TableRelayEvents))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var relayEventRows []RelayEventRow
	var relayEventRow RelayEventRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&relayEventRow.Id, &relayEventRow.Relay, &relayEventRow.Mode, &relayEventRow.Value, &relayEventRow.Rationale, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		relayEventRows = append(relayEventRows, RelayEventRow{
			Id:        relayEventRow.Id,
			Relay:     relayEventRow.Relay,
			Mode:      relayEventRow.Mode,
			Value:     relayEventRow.Value,
			Rationale: relayEventRow.Rationale,
			Time:      timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return relayEventRows, nil
}

func (d Database) SelectPastNHourRelayEventRows(n uint) ([]RelayEventRow, error) {
	cutoff := time.Now().Add(-time.Duration(n) * time.Hour)
	cutoffStr := FormatTime(cutoff)

	query := fmt.Sprintf(`SELECT id, relay, mode, value, rationale, time FROM %s WHERE time >= ? ORDER BY time DESC`, TableRelayEvents)

	rows, err := d.conn.Query(query, cutoffStr)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var relayEventRows []RelayEventRow
	var relayEventRow RelayEventRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&relayEventRow.Id, &relayEventRow.Relay, &relayEventRow.Mode, &relayEventRow.Value, &relayEventRow.Rationale, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		relayEventRows = append(relayEventRows, RelayEventRow{
			Id:        relayEventRow.Id,
			Relay:     relayEventRow.Relay,
			Mode:      relayEventRow.Mode,
			Value:     relayEventRow.Value,
			Rationale: relayEventRow.Rationale,
			Time:      timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return relayEventRows, nil
}

func (d Database) GetLastKnownRelayState(relay Relay_t) bool {
	query := fmt.Sprintf(`SELECT value FROM %s WHERE relay = ? ORDER BY time DESC LIMIT 1`, TableRelayEvents)

	var value bool
	if err := d.conn.QueryRow(query, string(relay)).Scan(&value); err != nil {
		return false
	}

	return value
}

// SYSTEM STATE
type SystemStateRow struct {
	Id    int
	State State_t
	Time  time.Time
}

func (r SystemStateRow) Insert(d Database) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s (state, time) VALUES (?, ?)`, TableSystemStates)
	if _, err := tx.ExecContext(ctx, query, r.State, FormatTime(r.Time)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d Database) SelectPastNHourSystemRows(n uint) ([]SystemStateRow, error) {
	cutoff := time.Now().Add(-time.Duration(n) * time.Hour)
	cutoffStr := FormatTime(cutoff)

	query := fmt.Sprintf(`SELECT id, state, time FROM %s WHERE time >= ? ORDER BY time DESC`, TableSystemStates)

	rows, err := d.conn.Query(query, cutoffStr)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var systemStateRows []SystemStateRow
	var systemStateRow SystemStateRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&systemStateRow.Id, &systemStateRow.State, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		systemStateRows = append(systemStateRows, SystemStateRow{
			Id:    systemStateRow.Id,
			State: systemStateRow.State,
			Time:  timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return systemStateRows, nil
}

func (d Database) SelectCurrentSystemState() (SystemStateRow, error) {
	query := fmt.Sprintf(`SELECT id, state, time FROM %s ORDER BY time DESC LIMIT 1`, TableSystemStates)

	var systemStateRow SystemStateRow
	var tempTime string

	err := d.conn.QueryRow(query).Scan(&systemStateRow.Id, &systemStateRow.State, &tempTime)
	if err != nil {
		return SystemStateRow{}, err
	}

	timeObj, err := ParseTime(tempTime)
	if err != nil {
		return SystemStateRow{}, err
	}

	systemStateRow.Time = timeObj
	return systemStateRow, nil
}

// JOURNAL ENTRY
type JournalEntryRow struct {
	Id               int
	DayRecap         string
	PlanForTomorrow  string
	SafeDefaultsJSON string
	AgentMusings     string
	IsStale          bool
	ValidForDate     string
	ImgUrl           string
	Time             time.Time
}

func (r JournalEntryRow) Insert(d Database) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s (day_recap, plan_for_tomorrow, safe_defaults_json, agent_musings, is_stale, valid_for_date, img_url, time) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, TableJournalEntries)
	if _, err := tx.ExecContext(ctx, query, r.DayRecap, r.PlanForTomorrow, r.SafeDefaultsJSON, r.AgentMusings, r.IsStale, r.ValidForDate, r.ImgUrl, FormatTime(r.Time)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d Database) SelectPastNDayJournalEntryRows(n uint) ([]JournalEntryRow, error) {
	cutoff := time.Now().Add(-time.Duration(24*n) * time.Hour)
	cutoffStr := FormatTime(cutoff)

	query := fmt.Sprintf(`SELECT id, day_recap, plan_for_tomorrow, safe_defaults_json, agent_musings, is_stale, valid_for_date, img_url, time from %s WHERE time >= ? ORDER BY time DESC`, TableJournalEntries)

	rows, err := d.conn.Query(query, cutoffStr)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var journalEntryRows []JournalEntryRow
	var journalEntryRow JournalEntryRow
	var tempTime string

	for rows.Next() {
		err := rows.Scan(&journalEntryRow.Id, &journalEntryRow.DayRecap, &journalEntryRow.PlanForTomorrow, &journalEntryRow.SafeDefaultsJSON, &journalEntryRow.AgentMusings, &journalEntryRow.IsStale, &journalEntryRow.ValidForDate, &journalEntryRow.ImgUrl, &tempTime)
		if err != nil {
			return nil, err
		}

		timeObj, err := ParseTime(tempTime)
		if err != nil {
			return nil, err
		}

		journalEntryRows = append(journalEntryRows, JournalEntryRow{
			Id:               journalEntryRow.Id,
			DayRecap:         journalEntryRow.DayRecap,
			PlanForTomorrow:  journalEntryRow.PlanForTomorrow,
			SafeDefaultsJSON: journalEntryRow.SafeDefaultsJSON,
			AgentMusings:     journalEntryRow.AgentMusings,
			IsStale:          journalEntryRow.IsStale,
			ValidForDate:     journalEntryRow.ValidForDate,
			ImgUrl:           journalEntryRow.ImgUrl,
			Time:             timeObj,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return journalEntryRows, nil
}

func (d Database) MarkAllJournalsStale() error {
	query := fmt.Sprintf(`UPDATE %s SET is_stale = 1`, TableJournalEntries)

	_, err := d.conn.Exec(query)

	return err
}

// RETRY JOB
type RetryJobRow struct {
	Id           int
	AttemptCount uint
	NextRetry    time.Time
	Time         time.Time
}

func (d Database) InsertRetryJob(nextRetry time.Time) error {
	query := fmt.Sprintf(`INSERT INTO %s (next_retry) VALUES (?)`, TableRetryQueue)

	_, err := d.conn.Exec(query, FormatTime(nextRetry))

	return err
}

func (d Database) ConsumeReadyRetryJob() (*RetryJobRow, error) {
	selectQuery := fmt.Sprintf(`SELECT id, attempt_count, next_retry FROM %s WHERE next_retry <= ? ORDER BY next_retry ASC LIMIT 1`, TableRetryQueue)

	row := d.conn.QueryRow(selectQuery, FormatTime(time.Now()))

	var job RetryJobRow
	job.Time = time.Now()
	err := row.Scan(&job.Id, &job.AttemptCount, &job.NextRetry)
	if err != nil {
		return nil, err
	}

	deleteQuery := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, TableRetryQueue)

	if _, err := d.conn.Exec(deleteQuery, job.Id); err != nil {
		return nil, err
	}

	return &job, nil
}

func (d Database) RequeueRetryJob(job RetryJobRow, nextRetry time.Time) error {
	query := fmt.Sprintf(`INSERT INTO %s (attempt_count, next_retry) VALUES (?, ?)`, TableRetryQueue)

	_, err := d.conn.Exec(query, job.AttemptCount+1, FormatTime(nextRetry))

	return err
}

// WATERING BUDGETS
type WateringBudgetRow struct {
	Date   time.Time
	Budget uint
	Time   time.Time
}

func (d Database) DecrementWateringBudget(date string, defaultBudget uint) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	insert_q := fmt.Sprintf(`INSERT OR IGNORE INTO %s (date, budget) VALUES (?, ?)`, TableWateringBudgets)
	if _, err := tx.ExecContext(ctx, insert_q, date, defaultBudget); err != nil {
		return err
	}

	var currentBudget int
	select_q := fmt.Sprintf(`SELECT budget FROM %s WHERE date = ?`, TableWateringBudgets)
	if err := tx.QueryRowContext(ctx, select_q, date).Scan(&currentBudget); err != nil {
		return err
	}

	if currentBudget <= 0 {
		return fmt.Errorf("%w for today (%s)", ErrBudgetExceeded, date)
	}

	update_q := fmt.Sprintf(`UPDATE %s SET budget = budget - 1 WHERE date = ?`, TableWateringBudgets)
	if _, err := tx.ExecContext(ctx, update_q, date); err != nil {
		return err
	}

	return tx.Commit()
}

// SelectWateringBudget returns the remaining watering count for the given date.
// Returns (0, sql.ErrNoRows) if no row exists yet — meaning the budget has not
// been touched today and the full default allowance is available.
func (d Database) SelectWateringBudget(date string) (int, error) {
	query := fmt.Sprintf(`SELECT budget FROM %s WHERE date = ?`, TableWateringBudgets)
	var budget int
	err := d.conn.QueryRowContext(context.Background(), query, date).Scan(&budget)
	return budget, err
}

// DAILY PHOTOS
type DailyPhotoRow struct {
	Id     int
	Date   string
	ImgUrl string
	Time   time.Time
}

func (r DailyPhotoRow) Insert(d Database) error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (date, img_url, time) VALUES (?, ?, ?)`, TableDailyPhotos)
	if _, err := tx.ExecContext(ctx, query, r.Date, r.ImgUrl, FormatTime(r.Time)); err != nil {
		return err
	}

	return tx.Commit()
}

func (d Database) SelectDailyPhoto(date string) (DailyPhotoRow, error) {
	query := fmt.Sprintf(`SELECT id, date, img_url, time FROM %s WHERE date = ?`, TableDailyPhotos)

	var row DailyPhotoRow
	var tempTime string

	err := d.conn.QueryRow(query, date).Scan(&row.Id, &row.Date, &row.ImgUrl, &tempTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyPhotoRow{}, sql.ErrNoRows
		}
		return DailyPhotoRow{}, err
	}

	timeObj, err := ParseTime(tempTime)
	if err != nil {
		return DailyPhotoRow{}, err
	}

	row.Time = timeObj
	return row, nil
}

// /////////////////////////////////////////////////////////////////////////////////////////// TABLE SCHEMA
func (d Database) InitializeTables() error {
	ctx := context.Background()

	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	sensorTelemetries_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			temp          FLOAT,
			air_humidity  FLOAT,
			soil_humidity FLOAT,
			time          TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableSensorTelemetries)
	if _, err := tx.ExecContext(ctx, sensorTelemetries_q); err != nil {
		return err
	}

	relayEvents_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			relay       TEXT NOT NULL CHECK(relay IN ('WATER_PUMP', 'GROW_LIGHT', 'INTAKE_FAN', 'EXHAUST_FAN')),
			mode        TEXT NOT NULL CHECK(mode IN ('AGENT', 'OVERRIDE', 'FAILSAFE')),
			value       BOOL,
			rationale   TEXT NOT NULL,
			time        TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableRelayEvents)
	if _, err := tx.ExecContext(ctx, relayEvents_q); err != nil {
		return err
	}

	systemStates_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			state   TEXT NOT NULL CHECK(state IN ('NOMINAL', 'CORRECTION_DEGRADED', 'JOURNAL_DEGRADED', 'FAILSAFE')),
			time    TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableSystemStates)
	if _, err := tx.ExecContext(ctx, systemStates_q); err != nil {
		return err
	}

	journalEntries_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			day_recap          TEXT NOT NULL,
			plan_for_tomorrow  TEXT NOT NULL,
			safe_defaults_json TEXT,
			agent_musings      TEXT,
			is_stale           BOOLEAN DEFAULT 0,
			valid_for_date     TEXT,
			img_url            TEXT,
			time               TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableJournalEntries)
	if _, err := tx.ExecContext(ctx, journalEntries_q); err != nil {
		return err
	}

	retryJobs_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			attempt_count INTEGER DEFAULT 0,
			next_retry    TEXT NOT NULL,
			time          TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableRetryQueue)
	if _, err := tx.ExecContext(ctx, retryJobs_q); err != nil {
		return err
	}

	wateringBudgets_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			date   TEXT PRIMARY KEY NOT NULL,
			budget INTEGER NOT NULL,
			time   TEXT DEFAULT CURRENT_TIMESTAMP
		);`,
		TableWateringBudgets)
	if _, err := tx.ExecContext(ctx, wateringBudgets_q); err != nil {
		return err
	}

	dailyPhotos_q := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS
		%s (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			date    TEXT NOT NULL UNIQUE,
			img_url TEXT NOT NULL,
			time    TEXT NOT NULL
		);`,
		TableDailyPhotos)
	if _, err := tx.ExecContext(ctx, dailyPhotos_q); err != nil {
		return err
	}

	return tx.Commit()
}

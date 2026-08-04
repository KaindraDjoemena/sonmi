package agent

import (
	"os"
	"testing"

	"sonmi/internal/config"
	"sonmi/internal/db"
)

func TestPrompt(t *testing.T) {
	dbConn, err := db.NewDatabase("../../dummy_db.db")
	if err != nil {
		t.Log("Failed to connect to DB:", err)
		os.Exit(1)
	}
	defer dbConn.CloseConn()

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Log(err)
	}

	correctionCtx, err := newCorrectionContext(dbConn, cfg)
	if err != nil {
		t.Log(err)
	}

	respBytes, err := promptAgent(correctionCtx, cfg)
	if err != nil {
		t.Log(err)
	}

	t.Logf("%s\n", respBytes)
}

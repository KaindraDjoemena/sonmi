package tui

import (
	"context"
	"errors"
	"image"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	cssh "charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/logging"

	tea "github.com/charmbracelet/bubbletea"

	"sonmi/internal/api"
	"sonmi/internal/db"
)

// teaMiddleware creates a wish.Middleware that runs a github.com/charmbracelet/bubbletea
// program per SSH session. We avoid charm.land/wish/v2/bubbletea because it
// requires charm.land/bubbletea/v2 (a different module with an incompatible
// tea.Model interface), while the rest of the app uses github.com/charmbracelet/bubbletea.
//
// Only one session is allowed at a time (enforced via CAS) — simultaneous
// sessions would race over relay control and the single-consumer TUI channels.
func teaMiddleware(framePipe chan image.Image, tuiTelemetryPipe chan api.Telemetry, tuiRelayStatePipe chan api.RelayState, tuiGatewayHealthPipe chan api.GatewayHealth, ctrl api.DeviceController, dbConn db.Database, loopStatus *api.LoopStatus) wish.Middleware {
	var activeSessions atomic.Int32
	return func(next cssh.Handler) cssh.Handler {
		return func(sess cssh.Session) {
			if !activeSessions.CompareAndSwap(0, 1) {
				wish.Fatalln(sess, "another session is already active")
				return
			}
			defer activeSessions.Store(0)

			pty, _, hasPTY := sess.Pty()
			if !hasPTY {
				wish.Fatalln(sess, "no active terminal, skipping")
				return
			}

			m := InitialModel(framePipe, tuiTelemetryPipe, tuiRelayStatePipe, tuiGatewayHealthPipe, ctrl, dbConn, loopStatus)
			p := tea.NewProgram(m,
				tea.WithAltScreen(),
				tea.WithInput(sess),
				tea.WithOutput(sess),
			)

			// Forward terminal resize events as tea.WindowSizeMsg.
			ctx, cancel := context.WithCancel(sess.Context())
			defer cancel()
			_, windowChanges, _ := sess.Pty()
			go func() {
				_ = pty // suppress unused warning — pty is used above for hasPTY check
				for {
					select {
					case <-ctx.Done():
						p.Quit()
						return
					case w, ok := <-windowChanges:
						if !ok {
							return
						}
						p.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
					}
				}
			}()

			if _, err := p.Run(); err != nil {
				slog.Error("SSH bubbletea session exited with error", "err", err)
			}
			p.Kill()
			next(sess)
		}
	}
}

func StartSSHServer(addr string, hostKeyPath string, framePipe chan image.Image, tuiTelemetryPipe chan api.Telemetry, tuiRelayStatePipe chan api.RelayState, tuiGatewayHealthPipe chan api.GatewayHealth, ctrl api.DeviceController, dbConn db.Database, loopStatus *api.LoopStatus) {
	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithAuthorizedKeys(".ssh/authorized_keys"),
		wish.WithMiddleware(
			teaMiddleware(framePipe, tuiTelemetryPipe, tuiRelayStatePipe, tuiGatewayHealthPipe, ctrl, dbConn, loopStatus),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatalf("Could not start SSH server: %s\n", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting SSH server on %s", addr)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, cssh.ErrServerClosed) {
			log.Fatalf("Could not start SSH server: %s\n", err)
		}
	}()

	<-done
	log.Print("Stopping SSH server\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, cssh.ErrServerClosed) {
		log.Fatalf("Could not stop SSH server: %s\n", err)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/signal"
	"strings"
	"syscall"

	mcp "github.com/Back-to-code/go-mcp"
	"github.com/gofiber/fiber/v3"
	"github.com/mjarkk/firefox-ui-testing-mcp/src"
)

func main() {
	src.SetupLogging()
	cfg := src.LoadConfig()
	if len(cfg.GlobalAllowedDomains) > 0 {
		src.Logf(src.ServerLogID, "global allowed domains (including subdomains): %s", strings.Join(cfg.GlobalAllowedDomains, ", "))
	} else {
		src.Logf(src.ServerLogID, "no global domain restriction, sessions may allow any domain")
	}
	src.Logf(src.ServerLogID, "max sessions: %d, ignore https errors: %v, headful: %v", cfg.MaxSessions, cfg.IgnoreHTTPSErrors, cfg.Headful)
	shots := src.NewScreenshotStore(src.ScreenshotTTL)

	manager, err := src.NewManager(cfg, shots)
	if err != nil {
		src.Fatalf("%v", err)
	}

	server := mcp.NewServer("firefox-ui-testing-mcp")
	src.RegisterTools(server, manager)

	app := fiber.New(fiber.Config{AppName: "firefox-ui-testing-mcp"})

	app.All("/mcp", func(c fiber.Ctx) error {
		logInitialize(c.Body(), c.IP())
		res := server.Handle(c.Method(), c.Body(), c.Context())
		c.Status(res.Status)
		c.Set(fiber.HeaderContentType, res.ContentType)
		return c.Send(res.Payload)
	})

	app.Get("/screenshots/:file", func(c fiber.Ctx) error {
		data, ok := shots.Get(strings.TrimSuffix(c.Params("file"), ".png"))
		if !ok {
			src.Logf(src.ServerLogID, "screenshot %s requested but it does not exist or expired", c.Params("file"))
			return c.Status(fiber.StatusNotFound).SendString("screenshot not found or expired")
		}
		c.Set(fiber.HeaderContentType, "image/png")
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Send(data)
	})

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("firefox-ui-testing-mcp is running, the mcp endpoint is /mcp")
	})

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		_ = app.Shutdown()
	}()

	src.Logf(src.ServerLogID, "listening on :%s, mcp endpoint %s/mcp", cfg.Port, cfg.PublicURL)
	if err := app.Listen(":"+cfg.Port, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		src.Logf(src.ServerLogID, "%v", err)
	}
	src.Logf(src.ServerLogID, "shutting down, closing %d open sessions", manager.Count())
	manager.Shutdown()
}

// logInitialize logs which mcp client connected.
func logInitialize(body []byte, ip string) {
	if !bytes.Contains(body, []byte(`"initialize"`)) {
		return
	}
	var req struct {
		Method string `json:"method"`
		Params struct {
			ClientInfo struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &req) != nil || req.Method != "initialize" {
		return
	}
	src.Logf(src.ServerLogID, "mcp client %s %s connected from %s", req.Params.ClientInfo.Name, req.Params.ClientInfo.Version, ip)
}

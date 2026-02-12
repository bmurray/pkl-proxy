package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "install":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pkl-proxy install <github-path>")
			os.Exit(1)
		}
		if err := cmdInstall(os.Args[2]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "uninstall":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pkl-proxy uninstall <github-path>")
			os.Exit(1)
		}
		if err := cmdUninstall(os.Args[2]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "settings":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pkl-proxy settings <install|uninstall>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "install":
			if err := cmdSettingsInstall(); err != nil {
				fmt.Println("Error:", err)
				os.Exit(1)
			}
		case "uninstall":
			if err := cmdSettingsUninstall(); err != nil {
				fmt.Println("Error:", err)
				os.Exit(1)
			}
		default:
			fmt.Println("Usage: pkl-proxy settings <install|uninstall>")
			os.Exit(1)
		}
	case "daemon":
		if err := cmdDaemon(); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: pkl-proxy run <cmd> [args...]")
			os.Exit(1)
		}
		if err := cmdRun(os.Args[2:]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	default:
		// Treat as implicit "run" for backwards compatibility
		if err := cmdRun(os.Args[1:]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	}
}

func usage() {
	fmt.Println("Usage: pkl-proxy <command> [args...]")
	fmt.Println("Commands:")
	fmt.Println("  install <path>      Add a GitHub path to proxy rewrites")
	fmt.Println("  uninstall <path>    Remove a GitHub path from proxy rewrites")
	fmt.Println("  settings install    Add pkl-proxy rewrites to ~/.pkl/settings.pkl")
	fmt.Println("  settings uninstall  Remove pkl-proxy rewrites from ~/.pkl/settings.pkl")
	fmt.Println("  daemon              Start proxy in daemon mode")
	fmt.Println("  run <cmd> [args]    Start proxy and run a command")
	os.Exit(1)
}

// startProxy sets up config, auth, and starts the HTTP proxy server.
// Returns the server and the resolved listen address for the env var.
func startProxy() (*http.Server, string, error) {
	configDir, err := findConfigDir()
	if err != nil {
		return nil, "", err
	}

	config, err := loadConfig(configDir)
	if err != nil {
		return nil, "", err
	}

	privateKeyPath := config.PrivateKey
	if !filepath.IsAbs(privateKeyPath) {
		privateKeyPath = filepath.Join(configDir, privateKeyPath)
	}
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, "", fmt.Errorf("reading private key file: %w", err)
	}

	tokenSource, err := buildTokenSource(config, privateKey)
	if err != nil {
		return nil, "", err
	}

	han := NewGithubPrivateReleaseProxy(tokenSource)

	svr := &http.Server{
		Addr:    config.ListenAddress,
		Handler: han,
	}

	listenAddr := config.ListenAddress
	if strings.HasPrefix(listenAddr, ":") {
		listenAddr = "localhost" + listenAddr
	}

	go func() {
		fmt.Printf("Starting local HTTP server on %s...\n", config.ListenAddress)
		if err := svr.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Error starting HTTP server:", err)
		}
	}()

	return svr, listenAddr, nil
}

func cmdDaemon() error {
	svr, _, err := startProxy()
	if err != nil {
		return err
	}

	// Reap orphaned children if running as PID 1 (e.g. in Docker)
	if os.Getpid() == 1 {
		go reapChildren()
	}

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	fmt.Printf("\nReceived %s, shutting down...\n", s)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return svr.Shutdown(ctx)
}

func cmdRun(args []string) error {
	_, listenAddr, err := startProxy()
	if err != nil {
		return err
	}

	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Env = append(os.Environ(), "PKL_PROXY_LISTEN_ADDRESS="+listenAddr)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("executing command: %w", err)
	}
	return nil
}

func findConfigDir() (string, error) {
	// Check XDG-compliant config dir first (e.g. ~/.config/pkl-proxy on Linux)
	if xdgDir, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(xdgDir, "pkl-proxy")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}

	// Fall back to ~/.pkl-proxy
	if homeDir, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(homeDir, ".pkl-proxy")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}

	return "", fmt.Errorf("config directory not found: create one at $XDG_CONFIG_HOME/pkl-proxy or ~/.pkl-proxy")
}

package pocketic

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type serverProc struct {
	cmd      *exec.Cmd
	portFile string
}

// Start launches the pocket-ic server binary (from POCKET_IC_BIN or the given
// path), discovers its port via --port-file, verifies the major version, and
// returns a ready Client. Call Close to stop the server.
func Start(binPath string) (*Client, error) {
	if binPath == "" {
		binPath = os.Getenv("POCKET_IC_BIN")
	}
	if binPath == "" {
		return nil, fmt.Errorf("no pocket-ic binary: set POCKET_IC_BIN or pass a path")
	}
	if err := checkVersion(binPath); err != nil {
		return nil, err
	}

	pf, err := os.CreateTemp("", "pocket-ic-*.port")
	if err != nil {
		return nil, err
	}
	pf.Close()
	portFile := pf.Name()
	os.Remove(portFile) // server (re)creates it

	cmd := exec.Command(binPath, "--port-file", portFile, "--ttl", "600")
	if os.Getenv("POCKET_IC_MUTE_SERVER") == "" {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pocket-ic: %w", err)
	}

	port, err := waitPort(portFile, 30*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	return &Client{
		base:   fmt.Sprintf("http://127.0.0.1:%d", port),
		http:   newHTTPClient(),
		poll:   50 * time.Millisecond,
		server: &serverProc{cmd: cmd, portFile: portFile},
	}, nil
}

func (c *Client) Close() error {
	if c.server == nil {
		return nil
	}
	os.Remove(c.server.portFile)
	if c.server.cmd.Process != nil {
		_ = c.server.cmd.Process.Kill()
	}
	_ = c.server.cmd.Wait()
	return nil
}

func checkVersion(binPath string) error {
	out, err := exec.Command(binPath, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("pocket-ic --version: %w", err)
	}
	// "pocket-ic-server 13.0.0"
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return fmt.Errorf("unexpected version string: %q", string(out))
	}
	major := strings.SplitN(fields[len(fields)-1], ".", 2)[0]
	if maj, _ := strconv.Atoi(major); maj != serverMajorVersion {
		return fmt.Errorf("pocket-ic server major version %s, want %d (%q)", major, serverMajorVersion, string(out))
	}
	return nil
}

func waitPort(portFile string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(portFile)
		if err == nil {
			s := strings.TrimSpace(string(b))
			// server writes the port followed by a newline; require the newline
			// so we never read a half-written value.
			if strings.HasSuffix(string(b), "\n") && s != "" {
				if port, err := strconv.Atoi(s); err == nil {
					return port, nil
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0, fmt.Errorf("pocket-ic did not write port file %s within %s", portFile, timeout)
}

package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Manager is the runtime-plane boundary. Production callers must use this interface,
// never invoke Docker directly from HTTP handlers.
type Manager interface {
	Provision(context.Context, string, string) (string, error)
	Destroy(context.Context, string) error
}

type DockerManager struct{}

func (DockerManager) Provision(ctx context.Context, sessionID, scenarioID string) (string, error) {
	if scenarioID == "broken-nginx-upstream" {
		return provisionBrokenNginx(ctx, sessionID)
	}
	name := "kernelquest-" + sessionID
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--rm", "--name", name,
		"--label", "kernelquest.managed=true", "--label", "kernelquest.session="+sessionID,
		"--network", "none", "--read-only", "--pids-limit", "128", "--memory", "256m", "--cpus", "0.5",
		"alpine:3.20", "sleep", "3600")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("provision bounded runtime: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return sessionID, nil
}

func (DockerManager) Destroy(ctx context.Context, runtimeID string) error {
	if runtimeID == "" {
		return nil
	}
	ids, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label=kernelquest.session="+runtimeID).Output()
	if err != nil {
		return fmt.Errorf("list runtime containers: %w", err)
	}
	for _, id := range strings.Fields(string(ids)) {
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", id).CombinedOutput(); err != nil && !strings.Contains(string(out), "No such container") {
			return fmt.Errorf("destroy runtime container: %w", err)
		}
	}
	networks, err := exec.CommandContext(ctx, "docker", "network", "ls", "-q", "--filter", "label=kernelquest.session="+runtimeID).Output()
	if err != nil {
		return fmt.Errorf("list runtime networks: %w", err)
	}
	for _, id := range strings.Fields(string(networks)) {
		if _, err := exec.CommandContext(ctx, "docker", "network", "rm", id).CombinedOutput(); err != nil {
			return fmt.Errorf("destroy runtime network: %w", err)
		}
	}
	return nil
}

func provisionBrokenNginx(ctx context.Context, sessionID string) (string, error) {
	network := "kq-" + sessionID
	labels := []string{"--label", "kernelquest.managed=true", "--label", "kernelquest.session=" + sessionID}
	if out, err := exec.CommandContext(ctx, "docker", append([]string{"network", "create", "--internal"}, append(labels, network)...)...).CombinedOutput(); err != nil {
		return "", fmt.Errorf("create incident network: %w: %s", err, strings.TrimSpace(string(out)))
	}
	cleanup := func() { _ = DockerManager{}.Destroy(context.Background(), sessionID) }
	apiName := network + "-api"
	apiArgs := append([]string{"run", "-d", "--rm", "--name", apiName}, labels...)
	apiArgs = append(apiArgs, "--network", network, "--network-alias", "api", "--read-only", "--pids-limit", "64", "--memory", "96m", "--cpus", "0.25", "hashicorp/http-echo:1.0", "-listen=:8080", "-text=checkout-api healthy")
	if out, err := exec.CommandContext(ctx, "docker", apiArgs...).CombinedOutput(); err != nil {
		cleanup()
		return "", fmt.Errorf("start incident api: %w: %s", err, strings.TrimSpace(string(out)))
	}
	nginxArgs := append([]string{"run", "-d", "--rm", "--name", network + "-nginx"}, labels...)
	// Nginx is writable because the challenge asks the learner to repair its configuration.
	nginxArgs = append(nginxArgs, "--network", network, "--pids-limit", "96", "--memory", "128m", "--cpus", "0.35", "kernelquest/nginx-incident:dev")
	if out, err := exec.CommandContext(ctx, "docker", nginxArgs...).CombinedOutput(); err != nil {
		cleanup()
		return "", fmt.Errorf("start incident nginx: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return sessionID, nil
}

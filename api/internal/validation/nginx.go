package validation

import (
	"context"
	"fmt"
	"os/exec"
)

// NginxHealthy independently checks the learner-repaired service from inside
// the incident network. It does not trust client-side claims.
func NginxHealthy(ctx context.Context, sessionID string) error {
	name := "kq-" + sessionID + "-nginx"
	cmd := exec.CommandContext(ctx, "docker", "exec", name, "curl", "-fsS", "http://localhost/health")
	if out, err := cmd.CombinedOutput(); err != nil { return fmt.Errorf("nginx health validation failed: %w: %s", err, out) }
	return nil
}

package voice

import (
	"fmt"
	"os/exec"
)

// Announce sends a voice announcement using kokoroSay.sh
func Announce(project, message string) error {
	cmd := exec.Command("kokoroSay.sh", "projectsay", project, message)

	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to announce: %w", err)
	}

	return nil
}

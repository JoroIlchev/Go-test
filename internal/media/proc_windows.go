package media

import (
	"context"
	"os/exec"
	"syscall"
)

// command runs a tool without flashing a console window when marquee runs as a service.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

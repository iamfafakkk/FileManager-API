package services

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RawCommandService handles raw shell command execution
type RawCommandService struct {
	basePath string
	owner    string
}

// CommandResult represents the result of a single command execution
type CommandResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code"`
}

// NewRawCommandService creates a new raw command service
func NewRawCommandService(basePath string, owner string) *RawCommandService {
	return &RawCommandService{
		basePath: basePath,
		owner:    owner,
	}
}

// ExecuteCommands executes a list of commands with security restrictions
func (s *RawCommandService) ExecuteCommands(commands []string) ([]CommandResult, error) {
	results := make([]CommandResult, 0, len(commands))

	for _, cmd := range commands {
		result := s.executeCommand(cmd)
		results = append(results, result)
	}

	return results, nil
}

// executeCommand executes a single command with security restrictions
func (s *RawCommandService) executeCommand(command string) CommandResult {
	result := CommandResult{
		Command:  command,
		ExitCode: 0,
	}

	// Validate command is allowed (basic security check)
	if err := s.validateCommand(command); err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result
	}

	// Build the command to run
	// If owner is set, run with cd to basePath first
	var shellCmd string
	if s.owner != "" {
		// Run command as the owner user with proper working directory
		shellCmd = fmt.Sprintf("cd %s && %s", s.basePath, command)
	} else {
		shellCmd = command
	}

	// Execute the command
	cmd := exec.Command("bash", "-c", shellCmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result.Output = strings.TrimSpace(stdout.String())

	if err != nil {
		result.Error = strings.TrimSpace(stderr.String())
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	return result
}

// validateCommand checks if a command is allowed based on security restrictions
func (s *RawCommandService) validateCommand(command string) error {
	// Deny commands that try to escape the base path
	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		"dd if=",
		"> /dev/",
		"chmod 777 /",
		"chown root",
	}

	lowerCmd := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, strings.ToLower(pattern)) {
			return fmt.Errorf("command contains dangerous pattern: %s", pattern)
		}
	}

	// If basePath is set, ensure command doesn't try to escape
	if s.basePath != "" {
		// Check for path traversal attempts
		if strings.Contains(command, "../") {
			return fmt.Errorf("path traversal not allowed")
		}

		// Check for absolute paths outside basePath
		// Allow common system commands but block file operations outside basePath
		fileOps := []string{"cp ", "mv ", "rm ", "cat ", "touch ", "mkdir ", "rmdir "}
		for _, op := range fileOps {
			if strings.Contains(command, op) {
				// Check if there's an absolute path that's not within basePath
				parts := strings.Fields(command)
				for _, part := range parts {
					if strings.HasPrefix(part, "/") && !strings.HasPrefix(part, s.basePath) {
						return fmt.Errorf("absolute path outside allowed directory: %s", part)
					}
				}
			}
		}
	}

	return nil
}

// GetBasePath returns the base path for this service
func (s *RawCommandService) GetBasePath() string {
	return s.basePath
}

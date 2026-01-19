package utils

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
)

// ResolveUser resolves a username to UID and GID
// It first attempts to use the os/user CGO lookup.
// If that fails, it falls back to executing the 'id' command.
func ResolveUser(username string) (int, int, error) {
	if username == "" {
		return -1, -1, fmt.Errorf("empty username")
	}

	// Strategy 1: os/user Lookup
	u, err := user.Lookup(username)
	if err == nil {
		uid, err1 := strconv.Atoi(u.Uid)
		gid, err2 := strconv.Atoi(u.Gid)
		if err1 == nil && err2 == nil {
			return uid, gid, nil
		}
	}

	// Strategy 2: Command line 'id' fallback
	// Useful in static binaries or non-cgo builds on Linux where NSS is not available
	uidCmd := exec.Command("id", "-u", username)
	outUid, errUid := uidCmd.Output()

	gidCmd := exec.Command("id", "-g", username)
	outGid, errGid := gidCmd.Output()

	if errUid == nil && errGid == nil {
		uidStr := strings.TrimSpace(string(outUid))
		gidStr := strings.TrimSpace(string(outGid))

		uid, err1 := strconv.Atoi(uidStr)
		gid, err2 := strconv.Atoi(gidStr)

		if err1 == nil && err2 == nil {
			return uid, gid, nil
		}
	}

	return -1, -1, fmt.Errorf("failed to resolve user %s: %v", username, err)
}

// SudoChown changes ownership of a file/folder using chown command.
// Uses format: chown user:user path
// This works when the application runs as root.
func SudoChown(path, owner string) error {
	if owner == "" {
		return nil
	}
	// Format: chown owner:owner path
	cmd := exec.Command("chown", owner+":"+owner, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chown failed for %s: %v, output: %s", path, err, string(output))
	}
	return nil
}

// SudoChownRecursive changes ownership of a directory recursively using chown -R command.
// Uses format: chown -R user:user path
func SudoChownRecursive(path, owner string) error {
	if owner == "" {
		return nil
	}
	// Format: chown -R owner:owner path
	cmd := exec.Command("chown", "-R", owner+":"+owner, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chown -R failed for %s: %v, output: %s", path, err, string(output))
	}
	return nil
}
